package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

var ErrResetToken = errors.New("invalid or expired reset token")

func resetDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreatePasswordResetToken creates a new password reset token
func (db *DB) CreatePasswordResetToken(userID, token string, expiresAt time.Time) (*types.PasswordResetToken, error) {
	query := `
		INSERT INTO password_reset_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token, expires_at, used, created_at
	`

	var resetToken types.PasswordResetToken
	err := db.QueryRow(query, userID, resetDigest(token), expiresAt.UTC()).Scan(
		&resetToken.ID,
		&resetToken.UserID,
		&resetToken.Token,
		&resetToken.ExpiresAt,
		&resetToken.Used,
		&resetToken.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &resetToken, nil
}

// GetPasswordResetToken retrieves a reset token by token string
func (db *DB) GetPasswordResetToken(token string) (*types.PasswordResetToken, error) {
	query := `
		SELECT id, user_id, token, expires_at, used, created_at
		FROM password_reset_tokens
		WHERE token = $1
	`

	var resetToken types.PasswordResetToken
	err := db.QueryRow(query, resetDigest(token)).Scan(
		&resetToken.ID,
		&resetToken.UserID,
		&resetToken.Token,
		&resetToken.ExpiresAt,
		&resetToken.Used,
		&resetToken.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &resetToken, nil
}

// MarkPasswordResetTokenUsed marks a reset token as used
func (db *DB) MarkPasswordResetTokenUsed(tokenID string) error {
	query := `
		UPDATE password_reset_tokens
		SET used = true
		WHERE id = $1
	`

	_, err := db.Exec(query, tokenID)
	return err
}

// DeleteExpiredPasswordResetTokens removes expired tokens (cleanup job)
func (db *DB) DeleteExpiredPasswordResetTokens() error {
	query := `
		DELETE FROM password_reset_tokens
		WHERE expires_at < (NOW() AT TIME ZONE 'UTC')
	`

	_, err := db.Exec(query)
	return err
}

func (db *DB) PrunePasswordResetTokens(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < (NOW() AT TIME ZONE 'UTC')`)
	return err
}

// UpdateUserPassword updates a user's password hash
func (db *DB) UpdateUserPassword(userID, passwordHash string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET password_hash=$1 WHERE id=$2`, passwordHash, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE password_reset_tokens SET used=true WHERE user_id=$1 AND NOT used`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// Lock the account before token consumption. Concurrent uses of the same or
// sibling tokens cannot both succeed; password and invalidation commit together.
func (db *DB) ConsumePasswordReset(ctx context.Context, token, passwordHash string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID string
	if err := tx.GetContext(ctx, &userID, `SELECT user_id FROM password_reset_tokens WHERE token=$1`, resetDigest(token)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrResetToken
		}
		return err
	}
	var locked string
	if err := tx.GetContext(ctx, &locked, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE password_reset_tokens SET used=true WHERE token=$1 AND NOT used AND expires_at > (clock_timestamp() AT TIME ZONE 'UTC')`, resetDigest(token))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrResetToken
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=$1 WHERE id=$2`, passwordHash, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE password_reset_tokens SET used=true WHERE user_id=$1 AND NOT used`, userID); err != nil {
		return err
	}
	return tx.Commit()
}
