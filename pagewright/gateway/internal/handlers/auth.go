package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

type ResetSender interface {
	SendReset(context.Context, string, string) error
}

type AuthHandler struct {
	db           *database.DB
	jwtManager   *auth.JWTManager
	oauthManager *auth.OAuthManager
	resetSender  ResetSender
}

func (h *AuthHandler) SetResetSender(sender ResetSender) { h.resetSender = sender }

func NewAuthHandler(db *database.DB, jwtManager *auth.JWTManager, oauthManager *auth.OAuthManager) *AuthHandler {
	return &AuthHandler{
		db:           db,
		jwtManager:   jwtManager,
		oauthManager: oauthManager,
	}
}

// Register handles user registration with email/password
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req types.RegisterRequest
	if err := boundedJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check if user already exists
	existingUser, err := h.db.WithContext(r.Context()).GetUserByEmail(req.Email)
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
	user, err := h.db.WithContext(r.Context()).CreateUser(req.Email, passwordHash, nil, nil)
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
	w.Header().Set("Cache-Control", "no-store")
	var req types.LoginRequest
	if err := boundedJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Get user
	user, err := h.db.WithContext(r.Context()).GetUserByEmail(req.Email)
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

// GoogleLogin is deliberately unavailable in the text-only MVP.
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	mvpUnavailable(w, "Google sign-in")
}

// GoogleCallback must not exchange codes or create accounts in the MVP.
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	mvpUnavailable(w, "Google sign-in")
}

// ForgotPassword initiates password reset flow
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req types.ForgotPasswordRequest
	if err := boundedJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" {
		respondError(w, http.StatusBadRequest, "email is required")
		return
	}

	if h.resetSender == nil {
		respondError(w, http.StatusServiceUnavailable, "password reset email is not configured; contact the operator")
		return
	}
	// Account-keyed throttling applies even to nonexistent accounts, across gateway
	// instances. The public router additionally limits auth requests by source IP.
	key := sha256.Sum256([]byte(req.Email))
	if err := h.db.TakePilotRate(r.Context(), "password-reset:"+hex.EncodeToString(key[:]), 1); err != nil {
		if errors.Is(err, database.ErrPilotLimit) {
			w.Header().Set("Retry-After", "60")
			respondError(w, 429, "wait a minute before requesting another reset")
		} else {
			respondError(w, 503, "password reset temporarily unavailable")
		}
		return
	}
	const message = "If the email exists, a password reset link will be sent"
	// Keep existing/nonexistent/ineligible accounts and delivery failures identical.
	user, err := h.db.WithContext(r.Context()).GetUserByEmail(req.Email)
	if err != nil {
		log.Print("Password reset account lookup failed")
	}
	if err == nil && user != nil && user.PasswordHash != "" {
		var bytes [32]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			respondError(w, 503, "password reset temporarily unavailable")
			return
		}
		token := hex.EncodeToString(bytes[:])
		record, err := h.db.WithContext(r.Context()).CreatePasswordResetToken(user.ID, token, time.Now().Add(time.Hour))
		if err != nil {
			log.Print("Password reset token persistence failed")
		} else if err := h.resetSender.SendReset(r.Context(), user.Email, token); err != nil {
			log.Print("Password reset delivery failed; verify SMTP configuration")
			// Invalidate even when delivery failed because the browser disconnected.
			cleanup, cancel := outcomeContext(r)
			defer cancel()
			if err := h.db.WithContext(cleanup).MarkPasswordResetTokenUsed(record.ID); err != nil {
				log.Print("Password reset delivery-failure invalidation failed")
			}
		}
	}
	respondJSON(w, map[string]string{"message": message})
}

// ResetPassword completes the password reset flow
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req types.ResetPasswordRequest
	if err := boundedJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Token == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "token and password are required")
		return
	}

	if err := auth.ValidatePassword(req.Password); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if raw, err := hex.DecodeString(req.Token); err != nil || len(raw) != 32 {
		respondError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}

	// Hash new password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.db.ConsumePasswordReset(r.Context(), req.Token, passwordHash); err != nil {
		if errors.Is(err, database.ErrResetToken) {
			respondError(w, 400, "invalid or expired token")
		} else {
			respondError(w, 503, "password reset temporarily unavailable")
		}
		return
	}
	respondJSON(w, map[string]string{
		"message": "Password successfully reset",
	})
}

// UpdatePassword handles password change for authenticated users
func (h *AuthHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, _ := middleware.GetUserFromContext(r)

	var req types.UpdatePasswordRequest
	if err := boundedJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get user from database
	dbUser, err := h.db.WithContext(r.Context()).GetUserByID(user.UserID)
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
	if err := h.db.WithContext(r.Context()).UpdateUserPassword(dbUser.ID, newPasswordHash); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	respondJSON(w, map[string]string{
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
