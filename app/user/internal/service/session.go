package service

import (
	commonpb "api/api/common/v1"
	userpb "api/api/user/common/v1"
	"api/api/user/session/v1"
	pbuser "api/api/user/users/v1"
	"common/cache"
	"common/constants"
	"common/hashid"
	"common/request"
	"context"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"user/ent"
	"user/ent/user"
	"user/internal/biz"
	"user/internal/biz/setting"
	"user/internal/data"
	"user/internal/data/types"
	"user/internal/pkg/captcha"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofrs/uuid"
	"github.com/pquerna/otp/totp"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	gob.Register(webauthn.SessionData{})
}

type SessionService struct {
	v1.UnimplementedSessionServer
	userClient data.UserClient
	kv         cache.Driver
	hasher     hashid.Encoder
	tokenAuth  biz.TokenAuth
	settings   setting.Provider
	webAuthn   *webauthn.WebAuthn
	captcha    captcha.CaptchaHelper
	l          *log.Helper
}

const (
	captchaNotMatch = "CAPTCHA not match."
	captchaFailed   = "captcha validation failed."
	captchaRefresh  = "Verification failed, please refresh the page and retry."

	tcCaptchaEndpoint = "captcha.tencentcloudapi.com"
	turnstileEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

type (
	turnstileResponse struct {
		Success bool `json:"success"`
	}
	capResponse struct {
		Success bool `json:"success"`
	}
)

func NewSessionService(userClient data.UserClient, kv cache.Driver, hasher hashid.Encoder, tokenAuth biz.TokenAuth,
	settings setting.Provider, captcha captcha.CaptchaHelper, l log.Logger) *SessionService {
	ctx := context.Background()
	siteBasic := settings.SiteBasic(ctx)
	wConfig := &webauthn.Config{
		RPDisplayName: siteBasic.Name,
		RPID:          settings.SiteURL(ctx).Hostname(),
		RPOrigins: lo.Map(settings.AllSiteURLs(ctx), func(item *url.URL, index int) string {
			item.Path = ""
			return item.String()
		}), // The origin URLs allowed for WebAuthn requests
	}
	webAuthn, err := webauthn.New(wConfig)
	h := log.NewHelper(l, log.WithMessageKey("service-session"))
	if err != nil {
		h.Errorf("Failed to create webauthn instance: %w", err)
	}
	return &SessionService{
		userClient: userClient,
		kv:         kv,
		hasher:     hasher,
		tokenAuth:  tokenAuth,
		settings:   settings,
		webAuthn:   webAuthn,
		captcha:    captcha,
		l:          h,
	}
}

func (s *SessionService) Login(ctx context.Context, req *v1.LoginRequest) (*v1.BuiltinLoginResponse, error) {
	err := verifyCaptcha(ctx, req.Captcha, s.settings, s.captcha, s.l)
	if err != nil {
		return nil, err
	}

	expectedUser, twoFaSession, err := s.LoginValidation(ctx, req)
	if err != nil {
		return nil, err
	}

	if twoFaSession == "" {
		return s.IssueToken(ctx, expectedUser)
	}

	return nil, commonpb.ErrorNotFullySuccess(twoFaSession)
}
func (s *SessionService) Login2FA(ctx context.Context, req *v1.Login2FARequest) (*v1.BuiltinLoginResponse, error) {
	sessionRaw, ok := s.kv.Get(fmt.Sprintf("user_2fa_%s", req.SessionId))
	if !ok {
		return nil, commonpb.ErrorUnspecified("Session not found")
	}

	uid := sessionRaw.(int)
	newCtx := context.WithValue(ctx, data.LoadUserGroup{}, true)
	expectedUser, err := s.userClient.GetByID(newCtx, uid)
	if err != nil {
		return nil, commonpb.ErrorUnspecified("User not found")
	}

	if expectedUser.TwoFactorSecret != "" {
		if !totp.Validate(req.Otp, expectedUser.TwoFactorSecret) {
			return nil, userpb.ErrorCode2Fa("Incorrect 2FA code")
		}
	}
	s.kv.Delete("user_2fa_" + req.SessionId)
	return s.IssueToken(ctx, expectedUser)
}
func (s *SessionService) RefreshToken(ctx context.Context, req *v1.RefreshTokenRequest) (*v1.Token, error) {
	token, err := s.tokenAuth.Refresh(ctx, req.RefreshToken)
	if err != nil {
		//return nil, response.NewError(response.CodeCredentialInvalid, "Failed to issue token pair", err)
		return nil, userpb.ErrorCredentialInvalid("Failed to issue token pair: %w", err)
	}

	return token, nil
}
func (s *SessionService) SignOut(ctx context.Context, req *v1.RefreshTokenRequest) (*emptypb.Empty, error) {
	claims, err := s.tokenAuth.Claims(ctx, req.RefreshToken)
	if err != nil {
		return nil, userpb.ErrorCredentialInvalid("Failed to parse token: %w", err)
	}

	if claims.RootTokenID != nil {
		tokenSettings := s.settings.TokenAuth(ctx)
		s.kv.Set(fmt.Sprintf("%s%s", constants.RevokeTokenPrefix, claims.RootTokenID.String()), true,
			int(tokenSettings.RefreshTokenTTL.Seconds()+10))
	}

	return &emptypb.Empty{}, nil
}
func (s *SessionService) PrepareLogin(ctx context.Context, req *v1.PrepareLoginRequest) (*v1.PrepareLoginResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadUserPasskey{}, true)
	expectedUser, err := s.userClient.GetByEmail(newCtx, req.Email)
	if err != nil {
		return nil, commonpb.ErrorUnspecified("User not found")
	}

	return &v1.PrepareLoginResponse{
		WebAuthnEnabled: len(expectedUser.Edges.Passkey) > 0,
		PasswordEnabled: expectedUser.Password != "",
	}, nil

}
func (s *SessionService) StartLoginAuthn(ctx context.Context, req *emptypb.Empty) (*v1.PreparePasskeyLoginResponse, error) {
	options, sessionData, err := s.webAuthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, userpb.ErrorInitializeAuthn("Failed to initialize WebAuthn: %w", err)
	}

	sessionId := uuid.Must(uuid.NewV4()).String()
	if err := s.kv.Set(fmt.Sprintf("%s%s", types.AuthnSessionKey, sessionId), *sessionData, 300); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to store WebAuthn session: %w", err)
	}

	optionsBytes, _ := json.Marshal(&options)
	return &v1.PreparePasskeyLoginResponse{
		Options:   optionsBytes,
		SessionId: sessionId,
	}, nil
}
func (s *SessionService) FinishLoginAuthn(ctx context.Context, req *v1.FinishPasskeyLoginRequest) (*v1.BuiltinLoginResponse, error) {
	u, err := s.FinishPasskeyLogin(ctx, req)
	if err != nil {
		return nil, err
	}

	return s.IssueToken(ctx, u)
}

func verifyCaptcha(ctx context.Context, req *pbuser.Captcha, settings setting.Provider, helper captcha.CaptchaHelper,
	l *log.Helper) error {
	if !settings.LoginCaptchaEnabled(ctx) {
		return nil
	}

	switch settings.CaptchaType(ctx) {
	case setting.CaptchaNormal, setting.CaptchaTcaptcha:
		if req.Ticket == "" || !helper.GetCaptcha().Verify(req.Ticket, req.Captcha, false) {
			return userpb.ErrorCaptcha(captchaNotMatch)
		}
	case setting.CaptchaReCaptcha:
		captchaSetting := settings.ReCaptcha(ctx)
		reCAPTCHA, err := captcha.NewReCAPTCHA(captchaSetting.Secret, captcha.V2, 10*time.Second)
		if err != nil {
			l.Warnf("reCAPTCHA verification failed, %s", err)
			break
		}
		err = reCAPTCHA.Verify(req.Captcha)
		if err != nil {
			l.Warnf("reCAPTCHA verification failed, %s", err)
			return userpb.ErrorCaptcha(captchaRefresh)
		}
	case setting.CaptchaTurnstile:
		captchaSetting := settings.TurnstileCaptcha(ctx)

		rc := request.NewClient(constants.MasterMode,
			request.WithLogger(l.Logger()),
			request.WithHeader(http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}}))

		formData := url.Values{}
		formData.Set("secret", captchaSetting.Secret)
		formData.Set("response", req.Ticket)
		res, err := rc.Request("POST", turnstileEndpoint, strings.NewReader(formData.Encode())).
			CheckHTTPResponse(http.StatusOK).
			GetResponse()
		if err != nil {
			return userpb.ErrorCaptcha(captchaFailed)
		}

		var trunstileRes turnstileResponse
		err = json.Unmarshal([]byte(res), &trunstileRes)
		if err != nil {
			l.Warnf("Turnstile verification failed, %s", err)
			return userpb.ErrorCaptcha(captchaFailed)
		}

		if !trunstileRes.Success {
			return userpb.ErrorCaptcha(captchaFailed)
		}
	case setting.CaptchaCap:
		captchaSetting := settings.CapCaptcha(ctx)
		if captchaSetting.InstanceURL == "" || captchaSetting.SiteKey == "" || captchaSetting.SecretKey == "" {
			l.Warnf("Cap verification failed: missing configuration")
			return userpb.ErrorCaptcha("Captcha configuration error")
		}

		rc := request.NewClient(constants.MasterMode,
			request.WithLogger(l.Logger()),
			request.WithHeader(http.Header{"Content-Type": []string{"application/json"}}))

		// Cap 2.0 API format: /{siteKey}/siteverify
		capEndpoint := strings.TrimSuffix(captchaSetting.InstanceURL, "/") + "/" + captchaSetting.SiteKey + "/siteverify"
		requestBody := map[string]string{
			"secret":   captchaSetting.SecretKey,
			"response": req.Ticket,
		}
		requestData, err := json.Marshal(requestBody)
		if err != nil {
			l.Warnf("Cap verification failed: %s", err)
			return userpb.ErrorCaptcha(captchaFailed)
		}

		res, err := rc.Request("POST", capEndpoint, strings.NewReader(string(requestData))).
			CheckHTTPResponse(http.StatusOK).
			GetResponse()
		if err != nil {
			l.Warnf("Cap verification failed: %s", err)
			return userpb.ErrorCaptcha(captchaFailed)
		}

		var capRes capResponse
		err = json.Unmarshal([]byte(res), &capRes)
		if err != nil {
			l.Warnf("Cap verification failed: %s", err)
			return userpb.ErrorCaptcha(captchaFailed)
		}

		if !capRes.Success {
			l.Warnf("Cap verification failed: validation returned false")
			return userpb.ErrorCaptcha(captchaFailed)
		}
	}
	return nil
}

func (s *SessionService) LoginValidation(ctx context.Context, req *v1.LoginRequest) (*ent.User, string, error) {
	newCtx := context.WithValue(ctx, data.LoadUserGroup{}, true)
	expectedUser, err := s.userClient.GetByEmail(newCtx, req.Username)

	// 校验
	if err != nil {
		err = userpb.ErrorInvalidPassword("Incorrect password or email address: %w", err)
	} else if checkErr := data.CheckPassword(expectedUser, req.Password); checkErr != nil {
		err = userpb.ErrorInvalidPassword("Incorrect password or email address: %w", err)
	} else if expectedUser.Status == user.StatusManualBanned || expectedUser.Status == user.StatusSysBanned {
		err = userpb.ErrorUserBanned("This account is already banned")
	} else if expectedUser.Status == user.StatusInactive {
		err = userpb.ErrorUserNotActivated("This account is not activated")
	}

	if err != nil {
		return nil, "", err
	}

	if expectedUser.TwoFactorSecret != "" {
		twoFaSessionID := uuid.Must(uuid.NewV4())
		s.kv.Set(fmt.Sprintf("user_2fa_%s", twoFaSessionID), expectedUser.ID, 600)
		return expectedUser, twoFaSessionID.String(), nil
	}

	return expectedUser, "", nil
}

func (s *SessionService) IssueToken(ctx context.Context, user *ent.User) (*v1.BuiltinLoginResponse, error) {
	token, err := s.tokenAuth.Issue(ctx, user, nil)
	if err != nil {
		return nil, commonpb.ErrorEncrypt("Failed to issue token pair: %w", err)
	}

	return &v1.BuiltinLoginResponse{
		User:  buildUser(user, s.hasher),
		Token: token,
	}, nil
}

func (s *SessionService) FinishPasskeyLogin(ctx context.Context, req *v1.FinishPasskeyLoginRequest) (*ent.User, error) {
	sessionDataRaw, ok := s.kv.Get(fmt.Sprintf("%s%s", types.AuthnSessionKey, req.SessionId))
	if !ok {
		return nil, commonpb.ErrorUnspecified("Session not found")
	}

	_ = s.kv.Delete(types.AuthnSessionKey, req.Response)

	sessionData := sessionDataRaw.(webauthn.SessionData)
	pcc, err := protocol.ParseCredentialRequestResponseBody(strings.NewReader(req.Response))
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse request: %w", err)
	}

	var loginedUser *ent.User
	discoverUserHanlde := func(rawId, userHanlde []byte) (user webauthn.User, err error) {
		uid, err := s.hasher.Decode(string(userHanlde), hashid.UserID)
		if err != nil {
			return nil, err
		}

		newCtx := context.WithValue(ctx, data.LoadUserPasskey{}, true)
		ctx = context.WithValue(newCtx, data.LoadUserGroup{}, true)
		u, err := s.userClient.GetLoginUserByID(ctx, uid)
		if err != nil {
			return nil, commonpb.ErrorDb("Failed to get users: %w", err)
		}

		if data.IsAnonymousUser(u) {
			return nil, commonpb.ErrorUnspecified("anonymous users")
		}

		loginedUser = u
		return &biz.AuthnUser{
			Hasher:      s.hasher,
			User:        u,
			Credentials: u.Edges.Passkey,
		}, nil
	}

	credential, err := s.webAuthn.ValidateDiscoverableLogin(discoverUserHanlde, sessionData, pcc)
	if err != nil {
		return nil, userpb.ErrorWebAuthnCredentialInvalid("Failed to validate login: %w", err)
	}

	// 找到用过的 credential
	usedCredentialId := base64.StdEncoding.EncodeToString(credential.ID)
	usedCredential, found := lo.Find(loginedUser.Edges.Passkey, func(item *ent.Passkey) bool {
		return item.CredentialID == usedCredentialId
	})

	if !found {
		return nil, commonpb.ErrorInternalSetting("Passkey login passed but credential used is unknown")
	}

	// 更新用户
	if err := s.userClient.MarkPasskeyUsed(ctx, loginedUser.ID, usedCredential.CredentialID); err != nil {
		return nil, commonpb.ErrorDb("Failed to update passkey: %w", err)
	}

	return loginedUser, nil
}
