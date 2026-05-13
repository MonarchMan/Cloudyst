package biz

import (
	"bytes"
	"common/auth"
	"common/cache"
	"common/constants"
	"common/hashid"
	"common/serializer"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"user/ent"
	"user/internal/biz/setting"
	"user/internal/conf"
	"user/internal/data"

	"github.com/gin-gonic/gin"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
)

type TokenAuth interface {
	// Issue issues a new pair of credentials for the given users.
	Issue(ctx context.Context, args *IssueTokenArgs) (*Token, error)
	// VerifyAndRetrieveUser verifies the given token and inject the users into current context.
	// Returns if upper caller should continue process other session provider.
	VerifyAndRetrieveUser(c *gin.Context) (bool, error)
	// Refresh refreshes the given refresh token and returns a new pair of credentials.
	Refresh(ctx context.Context, refreshToken string) (*Token, error)
	// Claims parses the given token string and returns the claims.
	Claims(ctx context.Context, tokenStr string) (*auth.Claims, error)
}

type IssueTokenArgs struct {
	User               *ent.User
	RootTokenID        *uuid.UUID
	ClientID           string
	Scopes             []string
	RefreshTTLOverride time.Duration
}

// Token stores token pair for authentication
type Token struct {
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token"`
	AccessExpires  time.Time `json:"access_expires"`
	RefreshExpires time.Time `json:"refresh_expires"`

	UID int `json:"-"`
}

type (
	TokenType         string
	TokenIDContextKey struct{}
	ScopeContextKey   struct{}
)

var (
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrUserNotFound        = errors.New("user not found")
)

// NewTokenAuth creates a new token based auth provider.
func NewTokenAuth(idEncoder hashid.Encoder, s setting.Provider, config *conf.Bootstrap, userClient data.UserClient,
	oAuthClient data.OAuthClientClient, l log.Logger, kv cache.Driver) TokenAuth {
	return &tokenAuth{
		idEncoder:   idEncoder,
		s:           s,
		secret:      []byte(config.Server.Sys.Secret),
		userClient:  userClient,
		l:           log.NewHelper(l, log.WithMessageKey("biz-auth")),
		kv:          kv,
		oAuthClient: oAuthClient,
	}
}

type tokenAuth struct {
	l           *log.Helper
	idEncoder   hashid.Encoder
	s           setting.Provider
	secret      []byte
	userClient  data.UserClient
	kv          cache.Driver
	oAuthClient data.OAuthClientClient
}

func (t *tokenAuth) Claims(ctx context.Context, tokenStr string) (*auth.Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &auth.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return t.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*auth.Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

func (t *tokenAuth) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	token, err := jwt.ParseWithClaims(refreshToken, &auth.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return t.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	claims, ok := token.Claims.(*auth.Claims)
	if !ok || claims.TokenType != constants.TokenTypeRefresh {
		return nil, constants.ErrInvalidRefreshToken
	}

	uid, err := t.idEncoder.Decode(claims.Subject, hashid.UserID)
	if err != nil {
		return nil, constants.ErrUserNotFound
	}

	expectedUser, err := t.userClient.GetActiveByID(ctx, uid)
	if err != nil {
		return nil, constants.ErrUserNotFound
	}

	// Check if users changed password or revoked session
	expectedHash := t.hashUserState(ctx, expectedUser)
	if !bytes.Equal(claims.StateHash, expectedHash[:]) {
		return nil, constants.ErrInvalidRefreshToken
	}

	// Check if root token is revoked
	if claims.RootTokenID == nil {
		return nil, constants.ErrInvalidRefreshToken
	}

	_, ok = t.kv.Get(fmt.Sprintf("%s%s", constants.RevokeTokenPrefix, claims.RootTokenID.String()))
	if ok {
		return nil, constants.ErrInvalidRefreshToken
	}

	// If token issued for an OAuth client, check if the client is still valid
	refreshTTLOverride := time.Duration(0)
	if claims.ClientID != "" {
		client, err := t.oAuthClient.GetByGUIDWithGrants(ctx, claims.ClientID, expectedUser.ID)
		if err != nil || len(client.Edges.Grants) == 0 {
			return nil, ErrInvalidRefreshToken
		}

		// Consented scopes must be a subset of the client's scopes
		if !auth.ValidateScopes(claims.Scopes, client.Edges.Grants[0].Scopes) {
			return nil, ErrInvalidRefreshToken
		}

		// Update last used at for the grant
		if err := t.oAuthClient.UpdateGrantLastUsedAt(ctx, expectedUser.ID, client.ID); err != nil {
			return nil, ErrInvalidRefreshToken
		}

		if client.Props != nil {
			refreshTTLOverride = time.Duration(client.Props.RefreshTokenTTL) * time.Second
		}
	}

	return t.Issue(ctx, &IssueTokenArgs{
		User:               expectedUser,
		RootTokenID:        claims.RootTokenID,
		Scopes:             claims.Scopes,
		ClientID:           claims.ClientID,
		RefreshTTLOverride: refreshTTLOverride,
	})
}

func (t *tokenAuth) VerifyAndRetrieveUser(c *gin.Context) (bool, error) {
	headerVal := c.GetHeader(constants.AuthorizationHeader)
	if strings.HasPrefix(headerVal, constants.TokenHeaderPrefixCr) {
		// This is an HMAC auth header, skip JWT verification
		return false, nil
	}

	tokenString := strings.TrimPrefix(headerVal, constants.TokenHeaderPrefix)
	if tokenString == "" {
		return true, nil
	}

	token, err := jwt.ParseWithClaims(tokenString, &auth.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return t.secret, nil
	})

	if err != nil {
		t.l.Warn("Failed to parse jwt token: %s", err)
		return false, nil
	}

	claims, ok := token.Claims.(*auth.Claims)
	if !ok || claims.TokenType != constants.TokenTypeAccess {
		return false, serializer.NewError(serializer.CodeCredentialInvalid, "Invalid token type", nil)
	}

	uid, err := t.idEncoder.Decode(claims.Subject, hashid.UserID)
	if err != nil {
		return false, serializer.NewError(serializer.CodeNotFound, "User not found", err)
	}

	context.WithValue(c, data.UserIDCtx{}, uid)

	if claims.ClientID != "" {
		context.WithValue(c, ScopeContextKey{}, claims.Scopes)
	}
	return false, nil
}

func (t *tokenAuth) Issue(ctx context.Context, args *IssueTokenArgs) (*Token, error) {
	u := args.User
	rootTokenID := args.RootTokenID

	uidEncoded := hashid.EncodeUserID(t.idEncoder, args.User.ID)
	tokenSettings := t.s.TokenAuth(ctx)
	issueDate := time.Now()
	accessTokenExpired := time.Now().Add(tokenSettings.AccessTokenTTL)
	refreshTokenExpired := time.Now().Add(tokenSettings.RefreshTokenTTL)
	if args.RefreshTTLOverride > 0 {
		refreshTokenExpired = time.Now().Add(args.RefreshTTLOverride)
	}
	if rootTokenID == nil {
		newRootTokenID := uuid.Must(uuid.NewV4())
		rootTokenID = &newRootTokenID
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		TokenType: constants.TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uidEncoded,
			NotBefore: jwt.NewNumericDate(issueDate),
			ExpiresAt: jwt.NewNumericDate(accessTokenExpired),
		},
		Scopes: args.Scopes,
	}).SignedString(t.secret)
	if err != nil {
		return nil, fmt.Errorf("faield to sign access token: %w", err)
	}

	userHash := t.hashUserState(ctx, u)
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		TokenType:   constants.TokenTypeRefresh,
		RootTokenID: rootTokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uidEncoded,
			NotBefore: jwt.NewNumericDate(issueDate),
			ExpiresAt: jwt.NewNumericDate(refreshTokenExpired),
		},
		Scopes:    args.Scopes,
		ClientID:  args.ClientID,
		StateHash: userHash[:],
	}).SignedString(t.secret)
	if err != nil {
		return nil, fmt.Errorf("faield to sign refresh token: %w", err)
	}

	return &Token{
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		AccessExpires:  accessTokenExpired,
		RefreshExpires: refreshTokenExpired,
		UID:            u.ID,
	}, nil
}

// hashUserState returns a hash string for users state for critical fields, it is used
// to detect refresh token revocation after users changed password.
func (t *tokenAuth) hashUserState(ctx context.Context, u *ent.User) [32]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s", u.Email, u.Password, t.s.SiteBasic(ctx).ID)))
}
