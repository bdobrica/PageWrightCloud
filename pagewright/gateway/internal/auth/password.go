package auth

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
	"unicode/utf8"
)

// Keep the existing MVP minimum and enforce bcrypt's byte ceiling consistently.
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return errors.New("password must contain at least 8 characters and at most 72 UTF-8 bytes")
	}
	return nil
}

// HashPassword generates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPasswordHash compares a password with a hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
