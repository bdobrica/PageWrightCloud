package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

// mockAuthHandler is a test version of AuthHandler with mock database
type mockAuthHandler struct {
	createUserFunc     func(email, passwordHash string, oauthProvider, oauthID *string) (*types.User, error)
	getUserByEmailFunc func(email string) (*types.User, error)
	jwtManager         *auth.JWTManager
	oauthManager       *auth.OAuthManager
}

func (h *mockAuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req types.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Check if user already exists
	existingUser, err := h.getUserByEmailFunc(req.Email)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check existing user")
		return
	}
	if existingUser != nil {
		respondError(w, http.StatusConflict, "user already exists")
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Create user
	user, err := h.createUserFunc(req.Email, passwordHash, nil, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Generate JWT token
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, types.AuthResponse{
		Token: token,
		User:  *user,
	})
}

func (h *mockAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req types.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Get user by email
	user, err := h.getUserByEmailFunc(req.Email)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	if user == nil {
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Check password
	if !auth.CheckPasswordHash(req.Password, user.PasswordHash) {
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Generate JWT token
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, types.AuthResponse{
		Token: token,
		User:  *user,
	})
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
			// Create handler with mocks
			jwtManager := auth.NewJWTManager("test-secret", 15*time.Minute)
			handler := &mockAuthHandler{
				createUserFunc: func(email, passwordHash string, oauthProvider, oauthID *string) (*types.User, error) {
					return tt.mockUser, tt.mockErr
				},
				getUserByEmailFunc: func(email string) (*types.User, error) {
					return nil, nil // User doesn't exist for registration
				},
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
				PasswordHash: passwordHash,
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
				PasswordHash: passwordHash,
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
			// Create handler with mocks
			jwtManager := auth.NewJWTManager("test-secret", 15*time.Minute)
			handler := &mockAuthHandler{
				createUserFunc: func(email, passwordHash string, oauthProvider, oauthID *string) (*types.User, error) {
					return nil, nil // Not used in login
				},
				getUserByEmailFunc: func(email string) (*types.User, error) {
					return tt.mockUser, nil
				},
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
