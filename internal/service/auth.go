// Package service implements authentication and ledger operations.
package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	jwtSecret []byte
	// ErrNoSigningKey reports that authentication has no configured signing key.
	ErrNoSigningKey = errors.New("JWT signing key not configured")

	// accessTTL is how long an access JWT is valid. Short by design — a leaked
	// access token expires quickly, and longer-lived access is provided via
	// rotating refresh tokens. Configurable via SetAccessTTL.
	accessTTL = 15 * time.Minute
)

// SetAccessTTL overrides the access-token lifetime (e.g. from config). A
// non-positive duration is ignored, keeping the safe default.
func SetAccessTTL(d time.Duration) {
	if d > 0 {
		accessTTL = d
	}
}

// AccessTTL reports the configured access-token lifetime (for expires_in).
func AccessTTL() time.Duration { return accessTTL }

// NewRefreshToken returns a fresh opaque refresh token and its SHA-256 hash.
// Only the hash is ever persisted; the raw token is returned to the client
// once and never stored.
func NewRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken returns the SHA-256 hex digest used to look a token up by hash.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// InitAuth must be called once at startup with a non-empty secret.
// main fails fast when the key is missing so a default secret can
// never reach production.
func InitAuth(secret string) error {
	if secret == "" {
		return ErrNoSigningKey
	}
	jwtSecret = []byte(secret)
	return nil
}

// HashPassword returns a bcrypt hash of the password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword compares a bcrypt hash with a password.
func CheckPassword(hash, pw string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw))
}

// GenerateJWT signs an access token for the user with the configured lifetime.
func GenerateJWT(userID int64) (string, error) {
	if len(jwtSecret) == 0 {
		return "", ErrNoSigningKey
	}
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(accessTTL).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(jwtSecret)
}

// ParseJWT validates an HS256 access token and returns its user ID.
func ParseJWT(tokenStr string) (int64, error) {
	if len(jwtSecret) == 0 {
		return 0, ErrNoSigningKey
	}
	tok, err := jwt.Parse(tokenStr,
		func(_ *jwt.Token) (interface{}, error) { return jwtSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !tok.Valid {
		if err == nil {
			err = jwt.ErrTokenUnverifiable
		}
		return 0, err
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0, jwt.ErrTokenInvalidClaims
	}
	id, ok := claims["user_id"].(float64)
	if !ok || id <= 0 {
		return 0, jwt.ErrTokenInvalidClaims
	}
	return int64(id), nil
}
