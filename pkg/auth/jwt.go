package auth

import (
	"context"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
	StateHash   []byte     `json:"state_hash,omitempty"`
	RootTokenID *uuid.UUID `json:"root_token_id,omitempty"`
	Scopes      []string   `json:"scopes,omitempty"`
	ClientID    string     `json:"client_id,omitempty"`
}

type (
	ScopeContextKey struct{}
)

// CodeInsufficientScope OAuth token scope insufficient
const CodeInsufficientScope = 40089

// ValidateScopes checks if all requested scopes are a subset of the allowed scopes.
// Returns true if all requested scopes are valid, false otherwise.
func ValidateScopes(requestedScopes, allowedScopes []string) bool {
	allowed := make(map[string]struct{}, len(allowedScopes))
	for _, scope := range allowedScopes {
		allowed[scope] = struct{}{}
	}
	for _, scope := range requestedScopes {
		if _, ok := allowed[scope]; !ok {
			return false
		}
	}
	return true
}

func GetScopesFromContext(ctx context.Context) (bool, []string) {
	scopes, ok := ctx.Value(ScopeContextKey{}).([]string)
	if !ok {
		return false, nil
	}
	return true, scopes
}

func CheckScope(c context.Context, requiredScopes ...string) error {
	hasScopes, tokenScopes := GetScopesFromContext(c)
	if !hasScopes {
		return nil
	}

	// Build a set of token scopes including implicit read permissions from write scopes
	scopeSet := make(map[string]struct{}, len(tokenScopes)*2)
	for _, scope := range tokenScopes {
		scopeSet[scope] = struct{}{}
		// If scope is "xxx.Write", also grant "xxx.Read"
		if resource, ok := extractWriteResource(scope); ok {
			scopeSet[resource+".Read"] = struct{}{}
		}
	}

	// Check if all required scopes are present
	for _, required := range requiredScopes {
		if _, ok := scopeSet[required]; !ok {
			return errors.New(CodeInsufficientScope, "Insufficient scope", "Insufficient scope: "+required)
		}
	}

	return nil
}

// extractWriteResource extracts the resource name from a write scope.
// For example, "File.Write" returns ("File", true), "File.Read" returns ("", false).
func extractWriteResource(scope string) (string, bool) {
	const writeSuffix = ".Write"
	if len(scope) > len(writeSuffix) && scope[len(scope)-len(writeSuffix):] == writeSuffix {
		return scope[:len(scope)-len(writeSuffix)], true
	}
	return "", false
}
