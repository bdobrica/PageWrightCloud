package database

import (
	"database/sql"
	"fmt"

	"github.com/PageWrightCloud/pagewright/gateway/internal/types"
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
