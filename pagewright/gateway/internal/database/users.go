package database

import (
	"database/sql"
	"fmt"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
)

// User operations

func (db *DB) CreateUser(email, passwordHash string, oauthProvider, oauthID *string) (*types.User, error) {
	user := &types.User{
		ID:            uuid.New().String(),
		Email:         email,
		PasswordHash:  passwordHash,
		OAuthProvider: oauthProvider,
		OAuthID:       oauthID,
	}

	query := `
		INSERT INTO users (id, email, password_hash, oauth_provider, oauth_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`

	err := db.QueryRow(query, user.ID, user.Email, user.PasswordHash, user.OAuthProvider, user.OAuthID).Scan(&user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

func (db *DB) GetUserByEmail(email string) (*types.User, error) {
	user := &types.User{}
	query := `SELECT id, email, password_hash, oauth_provider, oauth_id, created_at FROM users WHERE email = $1`

	err := db.Get(user, query, email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return user, nil
}

func (db *DB) GetUserByOAuth(provider, oauthID string) (*types.User, error) {
	user := &types.User{}
	query := `SELECT id, email, password_hash, oauth_provider, oauth_id, created_at FROM users WHERE oauth_provider = $1 AND oauth_id = $2`

	err := db.Get(user, query, provider, oauthID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by oauth: %w", err)
	}

	return user, nil
}

func (db *DB) GetUserByID(id string) (*types.User, error) {
	user := &types.User{}
	query := `SELECT id, email, password_hash, oauth_provider, oauth_id, created_at FROM users WHERE id = $1`

	err := db.Get(user, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return user, nil
}

func (db *DB) ListUsers() ([]*types.User, error) {
	var users []*types.User
	query := `SELECT id, email, password_hash, oauth_provider, oauth_id, created_at FROM users ORDER BY created_at DESC`

	err := db.Select(&users, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	return users, nil
}

func (db *DB) DeleteUser(id string) error {
	query := `DELETE FROM users WHERE id = $1`

	result, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}
