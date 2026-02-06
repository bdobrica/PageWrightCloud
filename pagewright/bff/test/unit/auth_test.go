package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PageWrightCloud/pagewright/bff/internal/auth"
	"github.com/PageWrightCloud/pagewright/bff/internal/types"
)

// MockDB is a mock database for testing
type MockDB struct {
	CreateUserFunc     func(email, passwordHash, oauthProvider, oauthID string) (*types.User, error)
	GetUserByEmailFunc func(email string) (*types.User, error)
}

func (m *MockDB) CreateUser(email, passwordHash, oauthProvider, oauthID string) (*types.User, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(email, passwordHash, oauthProvider, oauthID)
	}
	return nil, nil
}

func (m *MockDB) GetUserByEmail(email string) (*types.User, error) {
	if m.GetUserByEmailFunc != nil {
		return m.GetUserByEmailFunc(email)
	}
	return nil, nil
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name           string
		payload        types.RegisterRequest
		mockUser       *types.User
		mockErr        error
		expectedStatus int
	}{
		{
			name: "successful registration",
			payload: types.RegisterRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
			mockUser: &types.User{
				ID:    "user-id",
				Email: "test@example.com",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing email",
			payload: types.RegisterRequest{
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing password",
			payload: types.RegisterRequest{
				Email: "test@example.com",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock database
			mockDB := &MockDB{
				CreateUserFunc: func(email, passwordHash, oauthProvider, oauthID string) (*types.User, error) {
					return tt.mockUser, tt.mockErr
				},
			}

			// Create handler with mocks
			jwtManager := auth.NewJWTManager("test-secret", "15m")
			handler := &AuthHandler{
				db:         mockDB,
				jwtManager: jwtManager,
			}

			// Create request
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.Register(w, req)

			// Assert status code
			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Additional assertions for successful case
			if tt.expectedStatus == http.StatusOK {
				var resp types.AuthResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if resp.Token == "" {
					t.Error("expected token in response")
				}

				if resp.User.Email != tt.payload.Email {
					t.Errorf("expected email %s, got %s", tt.payload.Email, resp.User.Email)
				}
			}
		})
	}
}

func TestLogin(t *testing.T) {
	// Hash password for testing
	passwordHash, _ := auth.HashPassword("password123")

	tests := []struct {
		name           string
		payload        types.LoginRequest
		mockUser       *types.User
		expectedStatus int
	}{
		{
			name: "successful login",
			payload: types.LoginRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
			mockUser: &types.User{
				ID:           "user-id",
				Email:        "test@example.com",
				PasswordHash: &passwordHash,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid credentials",
			payload: types.LoginRequest{
				Email:    "test@example.com",
				Password: "wrongpassword",
			},
			mockUser: &types.User{
				ID:           "user-id",
				Email:        "test@example.com",
				PasswordHash: &passwordHash,
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "user not found",
			payload: types.LoginRequest{
				Email:    "notfound@example.com",
				Password: "password123",
			},
			mockUser:       nil,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock database
			mockDB := &MockDB{
				GetUserByEmailFunc: func(email string) (*types.User, error) {
					return tt.mockUser, nil
				},
			}

			// Create handler with mocks
			jwtManager := auth.NewJWTManager("test-secret", "15m")
			handler := &AuthHandler{
				db:         mockDB,
				jwtManager: jwtManager,
			}

			// Create request
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.Login(w, req)

			// Assert status code
			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
