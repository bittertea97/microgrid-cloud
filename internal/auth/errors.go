package auth

import "errors"

var (
	ErrUnauthorized = errors.New("auth: unauthorized")
	ErrForbidden    = errors.New("auth: forbidden")
	ErrInvalidToken = errors.New("auth: invalid token")
)
