package service

import (
	commonpb "api/api/common/v1"
	pbadmin "api/api/file/admin/v1"
	pb "api/api/user/common/v1"
	"api/api/user/users/v1"
	"api/external/trans"
	"common/auth"
	"common/cache"
	"common/hashid"
	"common/util"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	im "user/app/middleware"
	"user/ent"
	"user/ent/user"
	"user/internal/biz"
	"user/internal/biz/email"
	"user/internal/biz/setting"
	"user/internal/data"
	"user/internal/data/types"
	"user/internal/pkg/captcha"
	"user/internal/pkg/routes"
	"user/internal/pkg/utils"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pquerna/otp/totp"
	"github.com/samber/lo"
	"github.com/ua-parser/uap-go/uaparser"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	userStatusCondition   = "user_status"
	userGroupCondition    = "user_group"
	userNickCondition     = "user_nick"
	userEmailCondition    = "user_email"
	twoFaEnableSessionKey = "2fa_init_"
)

type UserService struct {
	v1.UnimplementedUserServer
	v1.UnimplementedUserNoAuthServer
	logger      *log.Helper
	settings    setting.Provider
	hasher      hashid.Encoder
	author      auth.Auth
	kv          cache.Driver
	userClient  data.UserClient
	groupClient data.GroupClient
	taskClient  pbadmin.AdminClient
	em          email.DriverManager
	webAuthn    *webauthn.WebAuthn
	uaParser    *uaparser.Parser
	captcha     captcha.CaptchaHelper

	mu sync.Mutex
}

func NewUserService(logger log.Logger, settings setting.Provider, hasher hashid.Encoder, author auth.Auth,
	kv cache.Driver, userClient data.UserClient, emailManager email.DriverManager,
	webAuthn *webauthn.WebAuthn, uaParser *uaparser.Parser, cap captcha.CaptchaHelper, groupClient data.GroupClient) *UserService {
	s := &UserService{
		logger:      log.NewHelper(logger, log.WithMessageKey("service-user")),
		settings:    settings,
		hasher:      hasher,
		author:      author,
		kv:          kv,
		userClient:  userClient,
		em:          emailManager,
		webAuthn:    webAuthn,
		uaParser:    uaParser,
		captcha:     cap,
		groupClient: groupClient,
	}
	s.em.Init()
	return s
}

func (s *UserService) Register(ctx context.Context, req *v1.RegisterRequest) (*v1.GetUserResponse, error) {
	if err := verifyCaptcha(ctx, req.Captcha, s.settings, s.captcha, s.logger); err != nil {
		return nil, err
	}

	isEmailRequired := s.settings.EmailActivationEnabled(ctx)
	args := &data.NewUserArgs{
		Email:         strings.ToLower(req.Email),
		PlainPassword: req.Password,
		Status:        user.StatusActive,
		GroupID:       s.settings.DefaultGroup(ctx),
		Language:      req.Language,
	}
	if isEmailRequired {
		args.Status = user.StatusInactive
	}

	userClient := s.userClient
	uc, tx, _, err := data.WithTx(ctx, userClient)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to start transaction: %w", err)
	}

	expectedUser, err := uc.Create(ctx, args)

	if err != nil {
		_ = data.Rollback(tx)
		if errors.Is(err, data.ErrUserEmailExisted) {
			return nil, pb.ErrorEmailExisted("Email already in use: %w", err)
		}

		if errors.Is(err, data.ErrInactiveUserExisted) {
			if err := s.sendActivationEmail(ctx, expectedUser); err != nil {
				return nil, commonpb.ErrorUnspecified("Failed to send activation email: %w", err)
			}

			return nil, pb.ErrorEmailSentAgain("Failed to send activation email: %w", err)
		}

		return nil, commonpb.ErrorDb("Failed to insert users row: %w", err)
	}

	if err := data.Commit(tx); err != nil {
		return nil, commonpb.ErrorDb("Failed to commit users row: %w", err)
	}

	if isEmailRequired {
		if err := s.sendActivationEmail(ctx, expectedUser); err != nil {
			return nil, commonpb.ErrorUnspecified("Failed to send activation email: %w", err)
		}
		return nil, commonpb.ErrorNotFullySuccess("User is not activated, activation email has been sent")
	}

	return buildUser(expectedUser, s.hasher), nil
}
func (s *UserService) ResetPassword(ctx context.Context, req *v1.ResetPasswordRequest) (*v1.GetUserResponse, error) {
	uid := hashid.FromContext(ctx)
	resetSession, ok := s.kv.Get(fmt.Sprintf("user_reset_%d", uid))
	if !ok || resetSession.(string) != req.Secret {
		return nil, pb.ErrorTempLinkExpired("Link is expired")
	}

	if err := s.kv.Delete(fmt.Sprintf("user_reset_%d", uid)); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to delete user reset session: %w", err)
	}

	u, err := s.userClient.GetActiveByID(ctx, uid)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to update password: %w", err)
	}

	return buildUser(u, s.hasher), nil
}
func (s *UserService) SendResetEmail(ctx context.Context, req *v1.SendResetEmailRequest) (*emptypb.Empty, error) {
	if err := verifyCaptcha(ctx, req.Captcha, s.settings, s.captcha, s.logger); err != nil {
		return nil, err
	}

	u, err := s.userClient.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, pb.ErrorUserNotFound("User not found: %w", err)
	}

	if u.Status == user.StatusManualBanned || u.Status == user.StatusSysBanned {
		return nil, pb.ErrorUserBanned("This users is banned")
	}

	secret := util.RandStringRunes(32)
	if err := s.kv.Set(fmt.Sprintf("%s%d", types.UserResetPrefix, u.ID), secret, 3600); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to create reset session: %w", err)
	}

	base := s.settings.SiteURL(ctx)
	resetUrl := routes.MasterUserResetUrl(base)
	queries := resetUrl.Query()
	queries.Add("id", hashid.EncodeUserID(s.hasher, u.ID))
	queries.Add("secret", secret)
	resetUrl.RawQuery = queries.Encode()

	title, body, err := email.NewResetEmail(ctx, s.settings, u, resetUrl.String())
	if err != nil {
		return nil, pb.ErrorEmailSentAgain("Failed to send activation email: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.em.Driver().Send(ctx, u.Email, title, body); err != nil {
		return nil, pb.ErrorEmailSentAgain("Failed to send activation email: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *UserService) Activate(ctx context.Context, req *v1.SignUserRequest) (*v1.GetUserResponse, error) {
	uid, err := s.hasher.Decode(req.Id, hashid.UserID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to decode user id: %s", err)
	}

	// 查找待激活用户
	inactiveUser, err := s.userClient.GetByID(ctx, uid)
	if err != nil {
		return nil, pb.ErrorUserNotFound("User not fount: %w", err)
	}

	// 检查状态
	if inactiveUser.Status != user.StatusInactive {
		return nil, pb.ErrorUserCannotActivate("This users cannot be activated: %w", err)
	}

	// 激活用户
	activateUser, err := s.userClient.SetStatus(ctx, inactiveUser, user.StatusActive)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update user: %w", err)
	}

	return buildUser(activateUser, s.hasher), nil
}
func (s *UserService) GetUser(ctx context.Context, req *v1.SimpleUserRequest) (*v1.GetUserResponse, error) {
	uid, err := s.hasher.Decode(req.Id, hashid.UserID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to decode user id: %s", err)
	}
	newCtx := context.WithValue(ctx, data.LoadUserGroup{}, true)
	u, err := s.userClient.GetByID(newCtx, uid)
	return buildUser(u, s.hasher), err
}
func (s *UserService) GetUserMe(ctx context.Context, req *emptypb.Empty) (*v1.GetUserResponse, error) {
	return buildUser(data.UserFromContext(ctx), s.hasher), nil
}
func (s *UserService) GetCapacity(ctx context.Context, req *emptypb.Empty) (*v1.CapacityResponse, error) {
	u := im.UserFromContext(ctx)
	group, err := s.groupClient.GetByID(ctx, u.GroupUsers)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get group users: %w", err)
	}
	return &v1.CapacityResponse{
		Total: group.MaxStorage,
		Used:  u.Storage,
	}, nil
}
func (s *UserService) SearchUsers(ctx context.Context, req *v1.SearchUsersRequest) (*v1.SearchUsersResponse, error) {
	users, err := s.userClient.SearchActive(ctx, types.SearchLimit, req.Keyword)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to search users: %w", err)
	}

	return &v1.SearchUsersResponse{
		Users: lo.Map(users, func(u *ent.User, _ int) *v1.GetUserResponse {
			return buildUserRedacted(u, RedactLevelUser, s.hasher)
		}),
	}, nil
}
func (s *UserService) StartRegisterAuthn(ctx context.Context, req *emptypb.Empty) (*v1.PrepareRegisterPasskeyResponse, error) {
	u := data.UserFromContext(ctx)
	existingKeys, err := s.userClient.ListPasskeys(ctx, u.ID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list passkeys: %w", err)
	}

	authSelect := protocol.AuthenticatorSelection{
		RequireResidentKey: protocol.ResidentKeyRequired(),
		UserVerification:   protocol.VerificationPreferred,
	}

	options, sessionData, err := s.webAuthn.BeginRegistration(
		&biz.AuthnUser{
			Hasher: s.hasher,
			User:   u,
		},
		webauthn.WithAuthenticatorSelection(authSelect),
		webauthn.WithExclusions(lo.Map(existingKeys, func(item *ent.Passkey, index int) protocol.CredentialDescriptor {
			return protocol.CredentialDescriptor{
				Type:            protocol.PublicKeyCredentialType,
				CredentialID:    item.Credential.ID,
				Transport:       item.Credential.Transport,
				AttestationType: item.Credential.AttestationType,
			}
		})),
	)
	if err != nil {
		return nil, pb.ErrorInitializeAuthn("Failed to begin registration: %w", err)
	}

	if err := s.kv.Set(fmt.Sprintf("%s%d", types.AuthnSessionKey, u.ID), *sessionData, 300); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to store session data: %w", err)
	}

	optionsBytes, _ := json.Marshal(&options)
	return &v1.PrepareRegisterPasskeyResponse{
		PublicKey: optionsBytes,
	}, nil
}
func (s *UserService) FinishRegisterAuthn(ctx context.Context, req *v1.FinishPasskeyRegisterRequest) (*v1.GetPasskeyResponse, error) {
	u := data.UserFromContext(ctx)
	sessionDataRaw, ok := s.kv.Get(fmt.Sprintf("%s%d", types.AuthnSessionKey, u.ID))
	if !ok {
		//return nil, response.NewError(response.CodeNotFound, "Session not found", nil)
		return nil, commonpb.ErrorNotFound("Session not found")
	}
	_ = s.kv.Delete(types.AuthnSessionKey, strconv.Itoa(u.ID))

	sessionData := sessionDataRaw.(webauthn.SessionData)
	pcc, err := protocol.ParseCredentialCreationResponseBody(strings.NewReader(req.Response))
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse request: %w", err)
	}

	credential, err := s.webAuthn.CreateCredential(&biz.AuthnUser{
		Hasher: s.hasher,
		User:   u,
	}, sessionData, pcc)

	client := s.uaParser.Parse(req.Ua)
	name := util.Replace(map[string]string{
		"{os}":      client.Os.Family,
		"{browser}": client.UserAgent.Family,
	}, req.Name)

	passkey, err := s.userClient.AddPasskey(ctx, u.ID, name, credential)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to add passkey: %w", err)
	}

	return buildPasskey(passkey), nil
}
func (s *UserService) DeletePasskey(ctx context.Context, req *v1.DeletePasskeyRequest) (*emptypb.Empty, error) {
	u := data.UserFromContext(ctx)
	existingKeys, err := s.userClient.ListPasskeys(ctx, u.ID)
	if err != nil {
		return nil, pb.ErrorInitializeAuthn("Failed to list passkeys: %w", err)
	}

	var existingKey *ent.Passkey
	for _, key := range existingKeys {
		if key.CredentialID == req.Id {
			existingKey = key
			break
		}
	}

	if existingKey == nil {
		return nil, commonpb.ErrorNotFound("Passkey not found")
	}

	if err := s.userClient.RemovePasskey(ctx, u.ID, req.Id); err != nil {
		return nil, commonpb.ErrorDb("Failed to remove passkey: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *UserService) GetUserInfo(ctx context.Context, req *v1.RawUserRequest) (*pb.User, error) {
	// 从数据库中获取用户信息
	newCtx := context.WithValue(ctx, data.LoadUserGroup{}, true)
	u, err := s.userClient.GetLoginUserByID(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get users: %w", err)
	}

	return utils.EntUserToProto(u), nil
}

func (s *UserService) ApplyStorageDiff(ctx context.Context, req *v1.ApplyStorageDiffRequest) (*emptypb.Empty, error) {
	diff := lo.MapKeys(req.StorageDiff, func(value int64, k int32) int { return int(k) })
	err := s.userClient.ApplyStorageDiff(ctx, diff)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to apply storage diff: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *UserService) GetAnonymousUser(ctx context.Context, req *emptypb.Empty) (*pb.User, error) {
	s.logger.WithContext(ctx).Infof("Get anonymous user")
	anonymous, err := s.userClient.AnonymousUser(ctx)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get anonymous: %w", err)
	}

	return utils.EntUserToProto(anonymous), nil
}

func (s *UserService) sendActivationEmail(ctx context.Context, newUser *ent.User) error {
	base := s.settings.SiteURL(ctx)
	userID := hashid.EncodeUserID(s.hasher, newUser.ID)
	ttl := time.Now().Add(time.Duration(24) * time.Hour)
	activateURL, err := auth.SignURI(s.author, routes.MasterUserActivateAPIUrl(base, userID).String(), &ttl)
	if err != nil {
		return commonpb.ErrorEncrypt("Failed to sign the activation link: %w", err)
	}

	// 取得签名
	credential := activateURL.Query().Get("sign")

	// 生成对用户访问的激活地址
	finalURL := routes.MasterUserActivateUrl(base)
	queries := finalURL.Query()
	queries.Add("id", userID)
	queries.Add("sign", credential)
	finalURL.RawQuery = queries.Encode()

	// 返送激活地址
	title, body, err := email.NewActivationEmail(ctx, s.settings, newUser, finalURL.String())
	if err != nil {
		return pb.ErrorSendEmailFailed("Failed to send activation email: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.em.Driver().Send(ctx, newUser.Email, title, body); err != nil {
		return pb.ErrorSendEmailFailed("Failed to send activation email: %w", err)
	}

	return nil
}

func (s *UserService) GetAvatar(ctx khttp.Context) error {
	req := ctx.Request()
	resp := ctx.Response()
	var reqBody v1.GetUserAvatarRequest
	if err := ctx.BindVars(&reqBody); err != nil {
		return fmt.Errorf("failed to bind request body: %w", err)
	}

	uid, err := s.hasher.Decode(reqBody.Id, hashid.UserID)
	if err != nil {
		return fmt.Errorf("failed to decode user id: %w", err)
	}
	user, err := s.userClient.GetByID(ctx, uid)
	if err != nil {
		return pb.ErrorUserNotFound("User not found: %w", err)
	}
	if !reqBody.NoCache {
		resp.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", s.settings.PublicResourceMaxAge(ctx)))
	}

	// 未设定头像时，返回 404 错误
	if user.Avatar == "" {
		resp.WriteHeader(http.StatusNotFound)
		return nil
	}

	avatarSettings := s.settings.Avatar(ctx)

	// Gravatar 头像重定向
	if user.Avatar == types.GravatarAvatar {
		gravatarRoot, err := url.Parse(avatarSettings.Gravatar)
		if err != nil {
			return commonpb.ErrorInternalSetting("Failed to parse gravatar server: %w", err)
		}
		emailLowered := strings.ToLower(user.Email)
		has := md5.Sum([]byte(emailLowered))
		avatar, _ := url.Parse(fmt.Sprintf("/avatar/%x?d=mm&s=200", has))

		targetURL := gravatarRoot.ResolveReference(avatar).String()
		ctx.Header().Set("Location", targetURL)
		return ctx.Result(http.StatusFound, nil)
	}

	// 本地头像文件
	if user.Avatar == types.FileAvatar {
		avatarRoot := util.DataPath(avatarSettings.Path)

		avatar, err := os.Open(filepath.Join(avatarRoot, fmt.Sprintf("avatar_%d.png", user.ID)))
		if err != nil {
			s.logger.Warn("Failed to open avatar files", err)
			resp.WriteHeader(http.StatusNotFound)
		}
		defer avatar.Close()

		http.ServeContent(resp, req, "avatar.png", user.UpdatedAt, avatar)
		return nil
	}

	resp.WriteHeader(http.StatusNotFound)
	return nil
}

func (s *UserService) GetActiveUserByDavAccount(ctx context.Context, req *v1.GetActiveUserByDavAccountRequest) (*pb.User, error) {
	user, err := s.userClient.GetActiveByDavAccount(ctx, req.Username, req.Password)
	if err != nil {
		return nil, pb.ErrorUserNotFound("User not found: %w", err)
	}

	return utils.EntUserToProto(user), nil
}

func (s *UserService) GetUserByEmail(ctx context.Context, req *v1.GetUserByEmailRequest) (*pb.User, error) {
	user, err := s.userClient.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, pb.ErrorUserNotFound("User not found: %w", err)
	}

	return utils.EntUserToProto(user), nil
}

func (s *UserService) GetUserSetting(ctx context.Context, req *emptypb.Empty) (*v1.GetSettingResponse, error) {
	user := im.UserFromContext(ctx)
	passkeys, err := s.userClient.ListPasskeys(ctx, user.ID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user passkeys: %w", err)
	}

	return buildUserSettingResponse(user, passkeys), nil
}

func (s *UserService) SetUserSetting(ctx context.Context, req *v1.SetUserRequest) (*emptypb.Empty, error) {
	saveSetting := false
	user := utils.ProtoUserToEnt(trans.FromContext(ctx))
	userClient := s.userClient
	if req.Nick != "" {
		if _, err := userClient.UpdateNickname(ctx, user, req.Nick); err != nil {
			return nil, commonpb.ErrorDb("Failed to update user nick", err)
		}
	}

	if req.Language != "" {
		user.Settings.Language = req.Language
		saveSetting = true
	}

	if req.PreferredTheme != "" {
		user.Settings.PreferredTheme = req.PreferredTheme
		saveSetting = true
	}

	if req.VersionRetentionEnabled != nil {
		user.Settings.VersionRetention = *req.VersionRetentionEnabled
		saveSetting = true
	}

	if req.VersionRetentionExt != nil && len(req.VersionRetentionExt) > 0 {
		user.Settings.VersionRetentionExt = req.VersionRetentionExt
		saveSetting = true
	}

	if req.VersionRetentionMax != nil {
		user.Settings.VersionRetentionMax = *req.VersionRetentionMax
		saveSetting = true
	}

	if req.DisableViewSync != nil {
		user.Settings.DisableViewSync = *req.DisableViewSync
		saveSetting = true
	}

	if req.ShareLinksInProfile != nil {
		user.Settings.ShareLinksInProfile = pb.ShareLinksInProfileLevel(*req.ShareLinksInProfile)
		saveSetting = true
	}

	if req.CurrentPassword != "" && req.NewPassword != "" {
		if err := data.CheckPassword(user, req.CurrentPassword); err != nil {
			return nil, pb.ErrorInvalidPassword("Incorrect password: %w", err)
		}

		if _, err := userClient.UpdatePassword(ctx, user, req.NewPassword); err != nil {
			return nil, commonpb.ErrorDb("Failed to update user password", err)
		}
	}

	if req.TwoFaEnabled != nil {
		if *req.TwoFaEnabled {
			secret, ok := s.kv.Get(fmt.Sprintf("%s%d", twoFaEnableSessionKey, user.ID))
			if !ok {
				return nil, commonpb.ErrorInternalSetting("You have not initiliazed 2FA session")
			}

			if !totp.Validate(req.TwoFaCode, secret.(string)) {
				return nil, pb.ErrorCode2Fa("Incorrect 2FA code")
			}

			if _, err := userClient.UpdateTwoFASecret(ctx, user, secret.(string)); err != nil {
				return nil, commonpb.ErrorDb("Failed to update user 2FA", err)
			}

		} else {
			if !totp.Validate(req.TwoFaCode, user.TwoFactorSecret) {
				return nil, pb.ErrorCode2Fa("Incorrect 2FA code")
			}

			if _, err := userClient.UpdateTwoFASecret(ctx, user, ""); err != nil {
				return nil, commonpb.ErrorDb("Failed to update user 2FA", err)
			}

		}
	}
	if saveSetting {
		if err := userClient.SaveSettings(ctx, user); err != nil {
			return nil, commonpb.ErrorDb("Failed to update user settings", err)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *UserService) InitUser2FA(ctx context.Context, req *emptypb.Empty) (*v1.InitUser2FAResponse, error) {
	user := trans.FromContext(ctx)

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "cloudyst",
		AccountName: user.Email,
	})
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to generate TOTP secret: %w", err)
	}

	if err := s.kv.Set(fmt.Sprintf("%s%d", twoFaEnableSessionKey, user.Id), key.Secret(), 600); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to store TOTP session: %w", err)
	}

	return &v1.InitUser2FAResponse{
		Secret: key.Secret(),
	}, nil
}

func (s *UserService) UpdateAvatar(ctx context.Context, req *v1.UpdateAvatarRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	var err error
	switch req.Type {
	case v1.AvatarType_GRAVATAR:
		_, err = s.userClient.UpdateAvatar(ctx, utils.ProtoUserToEnt(user), types.GravatarAvatar)
	case v1.AvatarType_FILE_AVATAR:
		_, err = s.userClient.UpdateAvatar(ctx, utils.ProtoUserToEnt(user), types.FileAvatar)
	}
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update user avatar: %w", err)
	}
	return &emptypb.Empty{}, nil
}
