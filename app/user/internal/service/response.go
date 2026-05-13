package service

import (
	userpb "api/api/user/common/v1"
	pbdevice "api/api/user/device/v1"
	pbsession "api/api/user/session/v1"
	pbuser "api/api/user/users/v1"
	"api/external/data/userdata"
	"common/hashid"
	"user/ent"
	"user/internal/biz"
	"user/internal/data"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	RedactLevelAnonymous = iota
	RedactLevelUser
)

func buildUser(u *ent.User, hasher hashid.Encoder) *pbuser.GetUserResponse {
	return &pbuser.GetUserResponse{
		Id:             hashid.EncodeUserID(hasher, u.ID),
		Email:          u.Email,
		Nickname:       u.Nick,
		Status:         string(u.Status),
		Avatar:         u.Avatar,
		CreatedAt:      timestamppb.New(u.CreatedAt),
		PreferredTheme: u.Settings.PreferredTheme,
		Anonymous:      u.ID == 0,
		Group:          buildGroup(u.Edges.Group, hasher),
		Pined: lo.Map(u.Settings.Pined, func(item *userdata.PinedFile, _ int) *userpb.PinedFile {
			return userdata.PinedFileToProto(item)
		}),
		Language:            u.Settings.Language,
		DisableViewSync:     u.Settings.DisableViewSync,
		ShareLinksInProfile: u.Settings.ShareLinksInProfile,
	}
}

func buildGroup(group *ent.Group, hasher hashid.Encoder) *pbuser.GetGroupResponse {
	if group == nil {
		return nil
	}
	return &pbuser.GetGroupResponse{
		Id:                  hashid.EncodeGroupID(hasher, group.ID),
		Name:                group.Name,
		Permissions:         *group.Permissions,
		DirectLinkBatchSize: int32(group.Settings.SourceBatchSize),
		TrashRetention:      int32(group.Settings.TrashRetention),
	}
}

func buildUserRedacted(u *ent.User, level int, idEncoder hashid.Encoder) *pbuser.GetUserResponse {
	userRaw := buildUser(u, idEncoder)

	user := &pbuser.GetUserResponse{
		Id:                  userRaw.Id,
		Nickname:            userRaw.Nickname,
		Avatar:              userRaw.Avatar,
		CreatedAt:           userRaw.CreatedAt,
		ShareLinksInProfile: userRaw.ShareLinksInProfile,
	}

	if userRaw.Group != nil {
		user.Group = redactedGroup(userRaw.Group)
	}

	if level == RedactLevelUser {
		user.Email = userRaw.Email
	}

	return user
}

func redactedGroup(group *pbuser.GetGroupResponse) *pbuser.GetGroupResponse {
	if group == nil {
		return nil
	}

	return &pbuser.GetGroupResponse{
		Id:   group.Id,
		Name: group.Name,
	}
}

func buildPasskey(passkey *ent.Passkey) *pbuser.GetPasskeyResponse {
	resp := &pbuser.GetPasskeyResponse{
		Id:        passkey.CredentialID,
		Name:      passkey.Name,
		CreatedAt: timestamppb.New(passkey.CreatedAt),
	}
	if passkey.UsedAt != nil {
		resp.UsedAt = timestamppb.New(*passkey.UsedAt)
	}
	return resp
}

func buildListDavAccountsResponse(res *data.ListDavAccountResult, hasher hashid.Encoder) *pbdevice.ListDavAccountsResponse {
	return &pbdevice.ListDavAccountsResponse{
		Pagination: res.PaginationResults,
		DavAccounts: lo.Map(res.Accounts, func(item *ent.DavAccount, index int) *pbdevice.GetDavAccountResponse {
			return buildDavAccountResponse(item, hasher)
		}),
	}
}

func buildDavAccountResponse(account *ent.DavAccount, hasher hashid.Encoder) *pbdevice.GetDavAccountResponse {
	resp := &pbdevice.GetDavAccountResponse{
		Id:        hashid.EncodeDavAccountID(hasher, account.ID),
		CreatedAt: timestamppb.New(account.CreatedAt),
		Name:      account.Name,
		Uri:       account.URI,
		Password:  account.Password,
	}
	if account.Options != nil {
		resp.Options = *account.Options
	}
	return resp
}

func buildUserSettingResponse(user *ent.User, passkeys []*ent.Passkey) *pbuser.GetSettingResponse {
	return &pbuser.GetSettingResponse{
		VersionRetentionEnabled: user.Settings.VersionRetention,
		VersionRetentionExt:     user.Settings.VersionRetentionExt,
		VersionRetentionMax:     int32(user.Settings.VersionRetentionMax),
		PasswordLess:            user.Password != "",
		TwoFaEnabled:            user.TwoFactorSecret != "",
		Passkeys: lo.Map(passkeys, func(item *ent.Passkey, index int) *pbuser.GetPasskeyResponse {
			return buildPasskey(item)
		}),
		DiableViewSync:      false,
		ShareLinksInProfile: "",
	}
}

func buildBuiltinLoginResponse(hasher hashid.Encoder, user *ent.User, token *biz.Token) *pbsession.BuiltinLoginResponse {
	return &pbsession.BuiltinLoginResponse{
		User:  buildUser(user, hasher),
		Token: buildToken(token),
	}
}

func buildToken(token *biz.Token) *pbsession.Token {
	return &pbsession.Token{
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		AccessExpires:  timestamppb.New(token.AccessExpires),
		RefreshExpires: timestamppb.New(token.RefreshExpires),
		Uid:            int32(token.UID),
	}
}
