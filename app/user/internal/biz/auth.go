package biz

import (
	pbsession "api/api/user/session/v1"
	"bytes"
	"common/auth"
	"common/cache"
	"common/constants"
	"common/hashid"
	"common/serializer"
	"context"
	"crypto/sha256"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TokenAuth interface {
	// Issue issues a new pair of credentials for the given users.
	Issue(ctx context.Context, u *ent.User, rootTokenID *uuid.UUID) (*pbsession.Token, error)
	// VerifyAndRetrieveUser verifies the given token and inject the users into current context.
	// Returns if upper caller should continue process other session provider.
	VerifyAndRetrieveUser(c *gin.Context) (bool, error)
	// Refresh refreshes the given refresh token and returns a new pair of credentials.
	Refresh(ctx context.Context, refreshToken string) (*pbsession.Token, error)
	// Claims parses the given token string and returns the claims.
	Claims(ctx context.Context, tokenStr string) (*auth.Claims, error)
}

type (
	TokenType         string
	TokenIDContextKey struct{}
)

// NewTokenAuth creates a new token based auth provider.
func NewTokenAuth(idEncoder hashid.Encoder, s setting.Provider, config *conf.Bootstrap, userClient data.UserClient,
	l log.Logger, kv cache.Driver) TokenAuth {
	return &tokenAuth{
		idEncoder:  idEncoder,
		s:          s,
		secret:     []byte(config.Server.Sys.Secret),
		userClient: userClient,
		l:          log.NewHelper(l, log.WithMessageKey("biz-auth")),
		kv:         kv,
	}
}

type tokenAuth struct {
	l          *log.Helper
	idEncoder  hashid.Encoder
	s          setting.Provider
	secret     []byte
	userClient data.UserClient
	kv         cache.Driver
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

func (t *tokenAuth) Refresh(ctx context.Context, refreshToken string) (*pbsession.Token, error) {
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

	return t.Issue(ctx, expectedUser, claims.RootTokenID)
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
	return false, nil
}

func (t *tokenAuth) Issue(ctx context.Context, u *ent.User, rootTokenID *uuid.UUID) (*pbsession.Token, error) {
	uidEncoded := hashid.EncodeUserID(t.idEncoder, u.ID)
	tokenSettings := t.s.TokenAuth(ctx)
	issueDate := time.Now()
	accessTokenExpired := time.Now().Add(tokenSettings.AccessTokenTTL)
	refreshTokenExpired := time.Now().Add(tokenSettings.RefreshTokenTTL)
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
		StateHash: userHash[:],
	}).SignedString(t.secret)
	if err != nil {
		return nil, fmt.Errorf("faield to sign refresh token: %w", err)
	}

	return &pbsession.Token{
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		AccessExpires:  timestamppb.New(accessTokenExpired),
		RefreshExpires: timestamppb.New(refreshTokenExpired),
		Uid:            int32(u.ID),
	}, nil
}

// hashUserState returns a hash string for users state for critical fields, it is used
// to detect refresh token revocation after users changed password.
func (t *tokenAuth) hashUserState(ctx context.Context, u *ent.User) [32]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s", u.Email, u.Password, t.s.SiteBasic(ctx).ID)))
}
