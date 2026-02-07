package database

import (
	"time"

	"github.com/PageWrightCloud/pagewright/gateway/internal/types"
)

// CreatePasswordResetToken creates a new password reset token
func (db *DB) CreatePasswordResetToken(userID, token string, expiresAt time.Time) (*types.PasswordResetToken, error) {
	query := `
		INSERT INTO password_reset_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token, expires_at, used, created_at
	`

	var resetToken types.PasswordResetToken
	err := db.QueryRow(query, userID, token, expiresAt).Scan(
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
	err := db.QueryRow(query, token).Scan(
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
		WHERE expires_at < NOW()
	`

	_, err := db.Exec(query)
	return err
}

// UpdateUserPassword updates a user's password hash
func (db *DB) UpdateUserPassword(userID, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $1
		WHERE id = $2
	`

	_, err := db.Exec(query, passwordHash, userID)
	return err
}
