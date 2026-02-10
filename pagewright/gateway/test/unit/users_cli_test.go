package handlers_test

import (
	"os"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
)

// TestCreateUserCLI tests the user creation functionality
func TestCreateUserCLI(t *testing.T) {
	// Setup test database
	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test database not configured")
	}
	defer db.Close()

	email := "testuser@example.com"
	password := "testpassword123"

	// Hash the password
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// Create the user
	user, err := db.CreateUser(email, hashedPassword, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if user.ID == "" {
		t.Error("User ID should not be empty")
	}
	if user.Email != email {
		t.Errorf("Expected email %s, got %s", email, user.Email)
	}
	if user.PasswordHash == "" {
		t.Error("Password hash should not be empty")
	}
	if user.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Verify the user can be retrieved
	retrievedUser, err := db.GetUserByEmail(email)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}
	if retrievedUser.ID != user.ID {
		t.Errorf("Expected user ID %s, got %s", user.ID, retrievedUser.ID)
	}
	if retrievedUser.Email != email {
		t.Errorf("Expected email %s, got %s", email, retrievedUser.Email)
	}

	// Verify password hash is correct
	if !auth.CheckPasswordHash(password, retrievedUser.PasswordHash) {
		t.Error("Password hash verification failed")
	}
}

// TestCreateDuplicateUser tests that creating a duplicate user fails
func TestCreateDuplicateUser(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test database not configured")
	}
	defer db.Close()

	email := "duplicate@example.com"
	password := "password123"

	// Create first user
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	_, err = db.CreateUser(email, hashedPassword, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create first user: %v", err)
	}

	// Try to create duplicate user
	_, err = db.CreateUser(email, hashedPassword, nil, nil)
	if err == nil {
		t.Error("Creating duplicate user should fail")
	}
}

// TestListUsers tests listing all users
func TestListUsers(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test database not configured")
	}
	defer db.Close()

	// Initially should have no users
	users, err := db.ListUsers()
	if err != nil {
		t.Fatalf("Failed to list users: %v", err)
	}
	initialCount := len(users)

	// Create test users
	testUsers := []struct {
		email    string
		password string
	}{
		{"user1@example.com", "pass1"},
		{"user2@example.com", "pass2"},
		{"user3@example.com", "pass3"},
	}

	for _, tu := range testUsers {
		hashedPassword, err := auth.HashPassword(tu.password)
		if err != nil {
			t.Fatalf("Failed to hash password: %v", err)
		}
		_, err = db.CreateUser(tu.email, hashedPassword, nil, nil)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
	}

	// List all users
	users, err = db.ListUsers()
	if err != nil {
		t.Fatalf("Failed to list users: %v", err)
	}
	if len(users) != initialCount+3 {
		t.Errorf("Expected %d users, got %d", initialCount+3, len(users))
	}

	// Verify users are in the list
	emails := make(map[string]bool)
	for _, user := range users {
		emails[user.Email] = true
	}
	for _, tu := range testUsers {
		if !emails[tu.email] {
			t.Errorf("User %s should be in the list", tu.email)
		}
	}
}

// TestDeleteUser tests user deletion
func TestDeleteUser(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test database not configured")
	}
	defer db.Close()

	email := "todelete@example.com"
	password := "password123"

	// Create user
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	user, err := db.CreateUser(email, hashedPassword, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify user exists
	retrievedUser, err := db.GetUserByEmail(email)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}
	if retrievedUser == nil {
		t.Error("User should exist")
	}

	// Delete user
	err = db.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	// Verify user is deleted
	retrievedUser, err = db.GetUserByEmail(email)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}
	if retrievedUser != nil {
		t.Error("User should be deleted")
	}
}

// TestDeleteNonExistentUser tests deleting a user that doesn't exist
func TestDeleteNonExistentUser(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		t.Skip("Test database not configured")
	}
	defer db.Close()

	// Try to delete a non-existent user
	err := db.DeleteUser("non-existent-id")
	if err == nil {
		t.Error("Deleting non-existent user should fail")
	}
}

// setupTestDB creates a test database connection
func setupTestDB(t *testing.T) *database.DB {
	// Use a test database URL from environment or default to in-memory SQLite
	dbURL := getTestDatabaseURL()

	db, err := database.NewDB(dbURL)
	if err != nil {
		t.Logf("Failed to connect to test database: %v", err)
		return nil
	}

	// Run migrations
	err = runTestMigrations(db)
	if err != nil {
		t.Logf("Failed to run test migrations: %v", err)
		return nil
	}

	return db
}

// getTestDatabaseURL returns the test database URL from environment or a default
func getTestDatabaseURL() string {
	// For PostgreSQL testing, you would set PAGEWRIGHT_TEST_DATABASE_URL
	// For now, we'll use the actual database URL but with a test database suffix
	// In a real scenario, you'd want to use a separate test database
	dbURL := getEnvOrDefault("PAGEWRIGHT_TEST_DATABASE_URL", "postgres://pagewright:pagewright@localhost:5432/pagewright_test?sslmode=disable")
	return dbURL
}

// runTestMigrations runs database migrations for testing
func runTestMigrations(db *database.DB) error {
	// Create users table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255),
			oauth_provider VARCHAR(50),
			oauth_id VARCHAR(255),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			CONSTRAINT unique_oauth UNIQUE (oauth_provider, oauth_id),
			CONSTRAINT email_or_oauth CHECK (
				(password_hash IS NOT NULL) OR 
				(oauth_provider IS NOT NULL AND oauth_id IS NOT NULL)
			)
		)
	`)
	if err != nil {
		return err
	}

	// Create indexes
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_oauth ON users(oauth_provider, oauth_id)`)
	return err
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
