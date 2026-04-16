package service

import (
	pbdevice "api/api/user/device/v1"
	pb "api/api/user/users/v1"
	"common/hashid"
	"user/ent"
	"user/internal/data"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	RedactLevelAnonymous = iota
	RedactLevelUser
)

func buildUser(u *ent.User, hasher hashid.Encoder) *pb.GetUserResponse {
	return &pb.GetUserResponse{
		Id:                  hashid.EncodeUserID(hasher, u.ID),
		Email:               u.Email,
		Nickname:            u.Nick,
		Status:              string(u.Status),
		Avatar:              u.Avatar,
		CreatedAt:           timestamppb.New(u.CreatedAt),
		PreferredTheme:      u.Settings.PreferredTheme,
		Anonymous:           u.ID == 0,
		Group:               buildGroup(u.Edges.Group, hasher),
		Pined:               u.Settings.Pined,
		Language:            u.Settings.Language,
		DisableViewSync:     u.Settings.DisableViewSync,
		ShareLinksInProfile: string(u.Settings.ShareLinksInProfile),
	}
}

func buildGroup(group *ent.Group, hasher hashid.Encoder) *pb.GetGroupResponse {
	if group == nil {
		return nil
	}
	return &pb.GetGroupResponse{
		Id:                  hashid.EncodeGroupID(hasher, group.ID),
		Name:                group.Name,
		Permissions:         *group.Permissions,
		DirectLinkBatchSize: group.Settings.SourceBatchSize,
		TrashRetention:      group.Settings.TrashRetention,
	}
}

func buildUserRedacted(u *ent.User, level int, idEncoder hashid.Encoder) *pb.GetUserResponse {
	userRaw := buildUser(u, idEncoder)

	user := &pb.GetUserResponse{
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

func redactedGroup(group *pb.GetGroupResponse) *pb.GetGroupResponse {
	if group == nil {
		return nil
	}

	return &pb.GetGroupResponse{
		Id:   group.Id,
		Name: group.Name,
	}
}

func buildPasskey(passkey *ent.Passkey) *pb.GetPasskeyResponse {
	resp := &pb.GetPasskeyResponse{
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

func buildUserSettingResponse(user *ent.User, passkeys []*ent.Passkey) *pb.GetSettingResponse {
	return &pb.GetSettingResponse{
		VersionRetentionEnabled: user.Settings.VersionRetention,
		VersionRetentionExt:     user.Settings.VersionRetentionExt,
		VersionRetentionMax:     user.Settings.VersionRetentionMax,
		PasswordLess:            user.Password != "",
		TwoFaEnabled:            user.TwoFactorSecret != "",
		Passkeys: lo.Map(passkeys, func(item *ent.Passkey, index int) *pb.GetPasskeyResponse {
			return buildPasskey(item)
		}),
		DiableViewSync:      false,
		ShareLinksInProfile: "",
	}
}
