package userdata

import (
	pb "api/api/common/v1"
	filepb "api/api/file/common/v1"
	userpb "api/api/user/common/v1"
	"api/external/data/filedata"
	"common/boolset"
	"common/constants"
	"common/hashid"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ShareLinksInProfileLevelPublicShareOnly = ""
	ShareLinksInProfileLevelAllShare        = "all_share"
	ShareLinksInProfileLevelHideShare       = "hide_share"
)

func UserSettingFromProto(setting *userpb.UserSetting) *UserSetting {
	if setting == nil {
		return nil
	}
	return &UserSetting{
		ProfileOff:          setting.ProfileOff,
		PreferredTheme:      setting.PreferredTheme,
		VersionRetention:    setting.VersionRetention,
		VersionRetentionExt: setting.VersionRetentionExt,
		VersionRetentionMax: int(setting.VersionRetentionMax),
		Pined: lo.Map(setting.Pined, func(item *userpb.PinedFile, index int) *PinedFile {
			return PinedFileFromProto(item)
		}),
		Language:        setting.Language,
		DisableViewSync: setting.DisableViewSync,
		FsViewMap: lo.MapEntries(setting.FsViewMap, func(key string, view *filepb.ExplorerView) (string, filedata.ExplorerView) {
			return key, *filedata.ExplorerViewFromProto(view)
		}),
		ShareLinksInProfile: ShareLinksInProfileLevelFromProto(setting.ShareLinksInProfile),
	}
}

func PinedFileFromProto(f *userpb.PinedFile) *PinedFile {
	if f == nil {
		return nil
	}
	return &PinedFile{
		Uri:  f.Uri,
		Name: f.Name,
	}
}

func ShareLinksInProfileLevelFromProto(level userpb.ShareLinksInProfileLevel) string {
	switch level {
	case userpb.ShareLinksInProfileLevel_PUBLIC_SHARE_ONLY:
		return ShareLinksInProfileLevelPublicShareOnly
	case userpb.ShareLinksInProfileLevel_ALL_SHARE:
		return ShareLinksInProfileLevelAllShare
	case userpb.ShareLinksInProfileLevel_HIDE_SHARE:
		return ShareLinksInProfileLevelHideShare
	default:
		return ""
	}
}

func UserSettingToProto(setting *UserSetting) *userpb.UserSetting {
	if setting == nil {
		return nil
	}
	return &userpb.UserSetting{
		ProfileOff:          setting.ProfileOff,
		PreferredTheme:      setting.PreferredTheme,
		VersionRetention:    setting.VersionRetention,
		VersionRetentionExt: setting.VersionRetentionExt,
		VersionRetentionMax: int32(setting.VersionRetentionMax),
		Pined: lo.Map(setting.Pined, func(item *PinedFile, index int) *userpb.PinedFile {
			return PinedFileToProto(item)
		}),
		Language:        setting.Language,
		DisableViewSync: setting.DisableViewSync,
		FsViewMap: lo.MapEntries(setting.FsViewMap, func(key string, view filedata.ExplorerView) (string, *filepb.ExplorerView) {
			return key, filedata.ExplorerViewToProto(&view)
		}),
		ShareLinksInProfile: ShareLinksInProfileLevelToProto(setting.ShareLinksInProfile),
	}
}

func PinedFileToProto(f *PinedFile) *userpb.PinedFile {
	if f == nil {
		return nil
	}
	return &userpb.PinedFile{
		Uri:  f.Uri,
		Name: f.Name,
	}
}

func ShareLinksInProfileLevelToProto(level string) userpb.ShareLinksInProfileLevel {
	switch level {
	case ShareLinksInProfileLevelPublicShareOnly:
		return userpb.ShareLinksInProfileLevel_PUBLIC_SHARE_ONLY
	case ShareLinksInProfileLevelAllShare:
		return userpb.ShareLinksInProfileLevel_ALL_SHARE
	case ShareLinksInProfileLevelHideShare:
		return userpb.ShareLinksInProfileLevel_HIDE_SHARE
	default:
		return userpb.ShareLinksInProfileLevel_PUBLIC_SHARE_ONLY
	}
}

func GroupSettingFromProto(setting *userpb.GroupSetting) *GroupSetting {
	if setting == nil {
		return nil
	}

	return &GroupSetting{
		CompressSize:          setting.CompressSize,
		DecompressSize:        setting.DecompressSize,
		RemoteDownloadOptions: setting.RemoteDownloadOptions.AsMap(),
		SourceBatchSize:       int(setting.SourceBatchSize),
		Aria2BatchSize:        int(setting.Aria2BatchSize),
		MaxWalkedFiles:        int(setting.MaxWalkedFiles),
		TrashRetention:        int(setting.TrashRetention),
		RedirectedSource:      setting.RedirectedSource,
	}
}

func GroupSettingToProto(setting *GroupSetting) *userpb.GroupSetting {
	if setting == nil {
		return nil
	}
	gs := &userpb.GroupSetting{
		CompressSize:     setting.CompressSize,
		DecompressSize:   setting.DecompressSize,
		SourceBatchSize:  int32(setting.SourceBatchSize),
		Aria2BatchSize:   int32(setting.Aria2BatchSize),
		MaxWalkedFiles:   int32(setting.MaxWalkedFiles),
		TrashRetention:   int32(setting.TrashRetention),
		RedirectedSource: setting.RedirectedSource,
	}
	gs.RemoteDownloadOptions, _ = structpb.NewStruct(setting.RemoteDownloadOptions)
	return gs
}

func UserInfoFromProtoUser(hasher hashid.Encoder, user *userpb.User) *pb.UserInfo {
	if user == nil {
		return nil
	}
	ownerInfo := &pb.UserInfo{
		Id:        hashid.EncodeID(hasher, int(user.Id), hashid.UserID),
		Nickname:  user.Nick,
		Status:    UserStatusFromProto(user.Status),
		CreatedAt: user.CreatedAt,
	}
	if user.Avatar != nil {
		ownerInfo.Avatar = user.Avatar.Value
	}
	return ownerInfo
}

func UserToProtoUserInfo(hasher hashid.Encoder, user *User) *pb.UserInfo {
	info := &pb.UserInfo{
		Id:       hashid.EncodeID(hasher, user.ID, hashid.UserID),
		Nickname: user.Nick,
		Status:   user.Status,
	}
	if !user.CreatedAt.IsZero() {
		info.CreatedAt = timestamppb.New(user.CreatedAt)
	}
	return info
}

func UserInfoFromProto(info *pb.UserInfo) *UserInfo {
	if info == nil {
		return nil
	}
	ui := &UserInfo{
		ID:       info.Id,
		Nickname: info.Nickname,
		Avatar:   info.Avatar,
		Status:   info.Status,
	}
	if info.CreatedAt != nil {
		ui.CreatedAt = info.CreatedAt.AsTime()
	}
	return ui
}

func UserInfoToProto(info *UserInfo) *pb.UserInfo {
	if info == nil {
		return nil
	}
	proto := &pb.UserInfo{
		Id:       info.ID,
		Nickname: info.Nickname,
		Avatar:   info.Avatar,
		Status:   info.Status,
	}
	if !info.CreatedAt.IsZero() {
		proto.CreatedAt = timestamppb.New(info.CreatedAt)
	}
	return proto
}

func UserInfoFromUser(hasher hashid.Encoder, user *User) *UserInfo {
	return &UserInfo{
		ID:        hashid.EncodeID(hasher, user.ID, hashid.UserID),
		Nickname:  user.Nick,
		Avatar:    user.Avatar,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	}
}
func AnonymousUserFromProto(protoUser *userpb.User) *User {
	if protoUser == nil {
		return nil
	}
	return &User{
		Settings: UserSettingFromProto(protoUser.Settings),
	}
}
func UserFromProto(protoUser *userpb.User) *User {
	entUser := &User{
		ID:         int(protoUser.Id),
		CreatedAt:  protoUser.CreatedAt.AsTime(),
		UpdatedAt:  protoUser.UpdatedAt.AsTime(),
		Email:      protoUser.Email,
		Nick:       protoUser.Nick,
		Status:     UserStatusFromProto(protoUser.Status),
		Storage:    protoUser.Storage,
		Settings:   UserSettingFromProto(protoUser.Settings),
		GroupUsers: int(protoUser.GroupUsers),
	}

	if protoUser.DeletedAt != nil {
		deletedAt := protoUser.DeletedAt.AsTime()
		entUser.DeletedAt = &deletedAt
	}

	if protoUser.Password != nil {
		entUser.Password = protoUser.Password.Value
	}

	if protoUser.TwoFactorSecret != nil {
		entUser.TwoFactorSecret = protoUser.TwoFactorSecret.Value
	}

	if protoUser.Avatar != nil {
		entUser.Avatar = protoUser.Avatar.Value
	}

	if protoUser.Group != nil {
		entUser.Group = GroupFromProto(protoUser.Group)
	}

	return entUser
}

func UserStatusFromProto(protoStatus userpb.User_Status) string {
	switch protoStatus {
	case userpb.User_STATUS_ACTIVE:
		return constants.StatusActive
	case userpb.User_STATUS_INACTIVE:
		return constants.StatusInactive
	case userpb.User_STATUS_MANUAL_BANNED:
		return constants.StatusManualBanned
	case userpb.User_STATUS_SYS_BANNED:
		return constants.StatusSysBanned
	default:
		return constants.StatusInactive
	}
}

func UserStatusToProto(status string) userpb.User_Status {
	switch status {
	case constants.StatusActive:
		return userpb.User_STATUS_ACTIVE
	case constants.StatusInactive:
		return userpb.User_STATUS_INACTIVE
	case constants.StatusManualBanned:
		return userpb.User_STATUS_MANUAL_BANNED
	case constants.StatusSysBanned:
		return userpb.User_STATUS_SYS_BANNED
	default:
		return userpb.User_STATUS_INACTIVE
	}
}

func GroupFromProto(protoGroup *userpb.Group) *Group {
	return &Group{
		ID:                int(protoGroup.Id),
		CreatedAt:         protoGroup.CreatedAt.AsTime(),
		UpdatedAt:         protoGroup.UpdatedAt.AsTime(),
		Name:              protoGroup.Name,
		MaxStorage:        protoGroup.MaxStorage,
		SpeedLimit:        int(protoGroup.SpeedLimit),
		Permissions:       (*boolset.BooleanSet)(&protoGroup.Permissions),
		Settings:          GroupSettingFromProto(protoGroup.Settings),
		StoragePolicyID:   int(protoGroup.StoragePolicyId),
		StoragePolicyInfo: filedata.StoragePolicyInfoFromProto(protoGroup.StoragePolicyInfo),
	}
}
