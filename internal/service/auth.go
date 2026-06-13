package service

import (
	"errors"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	jwtSecret       []byte
	ErrNoSigningKey = errors.New("JWT signing key not configured")
)

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

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw))
}

func GenerateJWT(userID int64) (string, error) {
	if len(jwtSecret) == 0 {
		return "", ErrNoSigningKey
	}
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(jwtSecret)
}

func ParseJWT(tokenStr string) (int64, error) {
	if len(jwtSecret) == 0 {
		return 0, ErrNoSigningKey
	}
	tok, err := jwt.Parse(tokenStr,
		func(t *jwt.Token) (interface{}, error) { return jwtSecret, nil },
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
