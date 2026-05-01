package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var JWTKey = []byte("my_super_secret_key_for_noumena")

type CustomClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken creates an access token (15m) and a refresh token (30d)
func GenerateTokens(userID string) (string, string, error) {
	accessExp := time.Now().Add(15 * time.Minute)
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	accessStr, err := accessToken.SignedString(JWTKey)
	if err != nil {
		return "", "", err
	}

	refreshExp := time.Now().Add(30 * 24 * time.Hour)
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(refreshExp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	refreshStr, err := refreshToken.SignedString(JWTKey)
	if err != nil {
		return "", "", err
	}

	return accessStr, refreshStr, nil
}

// ValidateToken parses and validates a JWT
func ValidateToken(tokenStr string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(t *jwt.Token) (interface{}, error) {
		return JWTKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
