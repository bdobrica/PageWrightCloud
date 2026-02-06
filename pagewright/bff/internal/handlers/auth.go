package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/PageWrightCloud/pagewright/bff/internal/auth"
	"github.com/PageWrightCloud/pagewright/bff/internal/database"
	"github.com/PageWrightCloud/pagewright/bff/internal/types"
	"github.com/google/uuid"
)

type AuthHandler struct {
	db           *database.DB
	jwtManager   *auth.JWTManager
	oauthManager *auth.OAuthManager
}

func NewAuthHandler(db *database.DB, jwtManager *auth.JWTManager, oauthManager *auth.OAuthManager) *AuthHandler {
	return &AuthHandler{
		db:           db,
		jwtManager:   jwtManager,
		oauthManager: oauthManager,
	}
}

// Register handles user registration with email/password
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
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
	existingUser, err := h.db.GetUserByEmail(req.Email)
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
	user, err := h.db.CreateUser(req.Email, passwordHash, nil, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Generate token
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, types.AuthResponse{
		Token:     token,
		ExpiresIn: h.jwtManager.GetExpirationSeconds(),
		User:      *user,
	})
}

// Login handles user login with email/password
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
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

	// Get user
	user, err := h.db.GetUserByEmail(req.Email)
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

	// Generate token
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, types.AuthResponse{
		Token:     token,
		ExpiresIn: h.jwtManager.GetExpirationSeconds(),
		User:      *user,
	})
}

// GoogleLogin initiates Google OAuth flow
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate state token for CSRF protection
	state := uuid.New().String()

	// Store state in session/cookie (simplified for PoC)
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	// Redirect to Google
	url := h.oauthManager.GetAuthURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// GoogleCallback handles the OAuth callback from Google
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing state cookie")
		return
	}

	state := r.URL.Query().Get("state")
	if state != stateCookie.Value {
		respondError(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	// Get code
	code := r.URL.Query().Get("code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "missing code parameter")
		return
	}

	// Exchange code for token
	token, err := h.oauthManager.Exchange(r.Context(), code)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to exchange code")
		return
	}

	// Get user info from Google
	googleUser, err := h.oauthManager.GetUserInfo(r.Context(), token)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get user info")
		return
	}

	// Check if user exists
	user, err := h.db.GetUserByOAuth("google", googleUser.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check user")
		return
	}

	// Create user if doesn't exist
	if user == nil {
		provider := "google"
		user, err = h.db.CreateUser(googleUser.Email, "", &provider, &googleUser.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	// Generate JWT
	jwtToken, err := h.jwtManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Return token (in real app, redirect to frontend with token)
	respondJSON(w, types.AuthResponse{
		Token:     jwtToken,
		ExpiresIn: h.jwtManager.GetExpirationSeconds(),
		User:      *user,
	})
}

func respondError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
	})
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
