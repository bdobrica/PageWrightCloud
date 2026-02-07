//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var (
	testDB         *database.DB
	testJWTManager *auth.JWTManager
	testRouter     *mux.Router
)

func TestMain(m *testing.M) {
	// Setup test database
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://pagewright:pagewright@localhost:5432/pagewright_test?sslmode=disable"
	}

	dbConn, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to test database: %v\n", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	testDB = database.NewDB(dbConn)

	// Setup test JWT manager
	testJWTManager = auth.NewJWTManager("test-secret", "15m")

	// Setup test router
	testRouter = setupTestRouter()

	// Run migrations
	if err := runTestMigrations(dbConn.DB); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	cleanupTestData(dbConn.DB)

	os.Exit(code)
}

func setupTestRouter() *mux.Router {
	r := mux.NewRouter()

	authHandler := handlers.NewAuthHandler(testDB, testJWTManager, nil)

	// Public routes
	r.HandleFunc("/auth/register", authHandler.Register).Methods("POST")
	r.HandleFunc("/auth/login", authHandler.Login).Methods("POST")

	return r
}

func runTestMigrations(db *sqlx.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255),
			oauth_provider VARCHAR(50),
			oauth_id VARCHAR(255),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			UNIQUE(oauth_provider, oauth_id)
		)`,
		`CREATE TABLE IF NOT EXISTS sites (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			fqdn VARCHAR(255) NOT NULL UNIQUE,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			template_id VARCHAR(100) NOT NULL,
			live_version_id VARCHAR(100),
			preview_version_id VARCHAR(100),
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return err
		}
	}

	return nil
}

func cleanupTestData(db *sqlx.DB) {
	db.Exec("DROP TABLE IF EXISTS sites CASCADE")
	db.Exec("DROP TABLE IF EXISTS users CASCADE")
}

func TestAuthFlow(t *testing.T) {
	// Clear test data
	testDB.DB.Exec("DELETE FROM users WHERE email LIKE 'test-%'")

	t.Run("Register new user", func(t *testing.T) {
		payload := types.RegisterRequest{
			Email:    "test-integration@example.com",
			Password: "password123",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp types.AuthResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Token == "" {
			t.Error("expected token in response")
		}

		if resp.User.Email != payload.Email {
			t.Errorf("expected email %s, got %s", payload.Email, resp.User.Email)
		}
	})

	t.Run("Login with correct credentials", func(t *testing.T) {
		payload := types.LoginRequest{
			Email:    "test-integration@example.com",
			Password: "password123",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp types.AuthResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Token == "" {
			t.Error("expected token in response")
		}
	})

	t.Run("Login with wrong password", func(t *testing.T) {
		payload := types.LoginRequest{
			Email:    "test-integration@example.com",
			Password: "wrongpassword",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("Duplicate registration", func(t *testing.T) {
		payload := types.RegisterRequest{
			Email:    "test-integration@example.com",
			Password: "password123",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Error("expected error for duplicate registration")
		}
	})
}

func TestJWTValidation(t *testing.T) {
	// Create test user
	user, err := testDB.CreateUser("test-jwt@example.com", "hash", "", "")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	t.Run("Valid JWT token", func(t *testing.T) {
		token, err := testJWTManager.GenerateToken(user.ID, user.Email)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		claims, err := testJWTManager.ValidateToken(token)
		if err != nil {
			t.Errorf("expected valid token, got error: %v", err)
		}

		if claims.UserID != user.ID {
			t.Errorf("expected user_id %s, got %s", user.ID, claims.UserID)
		}
	})

	t.Run("Invalid JWT token", func(t *testing.T) {
		invalidToken := "invalid.jwt.token"

		_, err := testJWTManager.ValidateToken(invalidToken)
		if err == nil {
			t.Error("expected error for invalid token")
		}
	})

	t.Run("Expired JWT token", func(t *testing.T) {
		// Create manager with very short expiration
		shortJWT := auth.NewJWTManager("test-secret", "1ns")
		token, _ := shortJWT.GenerateToken(user.ID, user.Email)

		// Wait a bit to ensure expiration
		// time.Sleep(10 * time.Millisecond)

		_, err := testJWTManager.ValidateToken(token)
		if err == nil {
			t.Error("expected error for expired token")
		}
	})
}
