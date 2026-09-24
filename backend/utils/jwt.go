package utils

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 7 * 24 * time.Hour

	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Claims struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"`
	jwt.RegisteredClaims
}

func secretKey() []byte {
	return []byte(os.Getenv("JWT_SECRET_KEY"))
}

func generateToken(userId, tokenType string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID: userId,
		Type:   tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey())
}

func GenerateAccessToken(userId string) (string, error) {
	return generateToken(userId, TokenTypeAccess, AccessTokenTTL)
}

func GenerateRefreshToken(userId string) (string, error) {
	return generateToken(userId, TokenTypeRefresh, RefreshTokenTTL)
}

func GenerateTokenPair(userId string) (accessToken string, refreshToken string, err error) {
	accessToken, err = GenerateAccessToken(userId)
	if err != nil {
		return "", "", err
	}

	refreshToken, err = GenerateRefreshToken(userId)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func VerifyToken(tokenString, expectedType string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) { return secretKey(), nil })
	if err != nil || !token.Valid {
		return nil, err
	}

	if claims.Type != expectedType {
		return nil, errors.New("unexpected token type")
	}

	return claims, nil
}
