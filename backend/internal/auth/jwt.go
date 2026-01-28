package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims used by this service.
type Claims struct {
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// ParseJWT validates a JWT and returns claims.
func ParseJWT(tokenString string, secret []byte) (*Claims, error) {
	if tokenString == "" {
		return nil, errors.New("auth: empty token")
	}
	if len(secret) == 0 {
		return nil, errors.New("auth: empty secret")
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	claims := &Claims{}
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("auth: invalid signing method")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("auth: invalid token")
	}
	if claims.TenantID == "" {
		return nil, errors.New("auth: missing tenant_id")
	}
	if _, ok := NormalizeRole(claims.Role); !ok {
		return nil, errors.New("auth: invalid role")
	}
	if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
		return nil, errors.New("auth: token expired")
	}
	return claims, nil
}
