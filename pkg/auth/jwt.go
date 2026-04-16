package auth

import (
	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
	StateHash   []byte     `json:"state_hash,omitempty"`
	RootTokenID *uuid.UUID `json:"root_token_id,omitempty"`
}
