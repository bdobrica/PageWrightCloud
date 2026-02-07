package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/PageWrightCloud/pagewright/gateway/internal/types"
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

// ForgotPassword initiates password reset flow
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req types.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" {
		respondError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Get user by email
	user, err := h.db.GetUserByEmail(req.Email)
	if err != nil || user == nil {
		// Don't reveal if user exists - always return success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "If the email exists, a password reset link will be sent",
		})
		return
	}

	// Only allow password reset for non-OAuth users
	if user.PasswordHash == "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "If the email exists, a password reset link will be sent",
		})
		return
	}

	// Generate reset token (UUID)
	token := uuid.New().String()

	// Create reset token in database (expires in 1 hour)
	expiresAt := time.Now().Add(1 * time.Hour)
	_, err = h.db.CreatePasswordResetToken(user.ID, token, expiresAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create reset token")
		return
	}

	// TODO: Send email with reset link
	// In production: send email to user.Email with link: https://frontend.com/reset-password?token={token}
	// For now, just return success (in dev, you can log the token)
	log.Printf("Password reset token for %s: %s", user.Email, token)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "If the email exists, a password reset link will be sent",
	})
}

// ResetPassword completes the password reset flow
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req types.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Token == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "token and password are required")
		return
	}

	if len(req.Password) < 8 {
		respondError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Get reset token
	resetToken, err := h.db.GetPasswordResetToken(req.Token)
	if err != nil || resetToken == nil {
		respondError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}

	// Check if token is expired
	if time.Now().After(resetToken.ExpiresAt) {
		respondError(w, http.StatusBadRequest, "token has expired")
		return
	}

	// Check if token was already used
	if resetToken.Used {
		respondError(w, http.StatusBadRequest, "token has already been used")
		return
	}

	// Hash new password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Update user password
	if err := h.db.UpdateUserPassword(resetToken.UserID, passwordHash); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	// Mark token as used
	if err := h.db.MarkPasswordResetTokenUsed(resetToken.ID); err != nil {
		log.Printf("Warning: failed to mark token as used: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Password successfully reset",
	})
}

// UpdatePassword handles password change for authenticated users
func (h *AuthHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)

	var req types.UpdatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	if len(req.NewPassword) < 8 {
		respondError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	// Get user from database
	dbUser, err := h.db.GetUserByID(user.UserID)
	if err != nil || dbUser == nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	// Check if user has password (not OAuth user)
	if dbUser.PasswordHash == "" {
		respondError(w, http.StatusBadRequest, "OAuth users cannot change password")
		return
	}

	// Verify current password
	if !auth.CheckPasswordHash(req.CurrentPassword, dbUser.PasswordHash) {
		respondError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Hash new password
	newPasswordHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Update password
	if err := h.db.UpdateUserPassword(dbUser.ID, newPasswordHash); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Password successfully updated",
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
