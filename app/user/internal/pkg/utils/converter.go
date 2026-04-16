package utils

import (
	pb "api/api/user/common/v1"
	"common/boolset"
	"user/ent"
	"user/ent/user"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func EntUserToProto(user *ent.User) *pb.User {
	protoUser := &pb.User{
		Id:         int64(user.ID),
		CreatedAt:  timestamppb.New(user.CreatedAt),
		UpdatedAt:  timestamppb.New(user.UpdatedAt),
		Email:      user.Email,
		Nick:       user.Nick,
		Status:     GetUserStatus(user.Status),
		Storage:    user.Storage,
		Settings:   user.Settings,
		GroupUsers: int64(user.GroupUsers),
	}

	if user.DeletedAt != nil {
		protoUser.DeletedAt = timestamppb.New(*user.DeletedAt)
	}

	if user.Edges.Group != nil {
		protoUser.Group = EntGroupToProto(user.Edges.Group)
	}

	if user.Edges.Passkey != nil {
		protoUser.Passkey = lo.Map(user.Edges.Passkey, func(item *ent.Passkey, index int) *pb.Passkey {
			return EntPasskeyToProto(item)
		})
	}

	if user.Edges.DavAccounts != nil {
		protoUser.DavAccounts = lo.Map(user.Edges.DavAccounts, func(item *ent.DavAccount, index int) *pb.DavAccount {
			return EntDavAccountToProto(item)
		})
	}

	return protoUser
}

func ProtoUserToEnt(protoUser *pb.User) *ent.User {
	entUser := &ent.User{
		ID:         int(protoUser.Id),
		CreatedAt:  protoUser.CreatedAt.AsTime(),
		UpdatedAt:  protoUser.UpdatedAt.AsTime(),
		Email:      protoUser.Email,
		Nick:       protoUser.Nick,
		Status:     ProtoUserStatusToEnt(protoUser.Status),
		Storage:    protoUser.Storage,
		Settings:   protoUser.Settings,
		GroupUsers: int(protoUser.GroupUsers),
		Edges:      ent.UserEdges{},
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
		entUser.Edges.Group = ProtoGroupToEnt(protoUser.Group)
	}

	if protoUser.Passkey != nil {
		entUser.Edges.Passkey = lo.Map(protoUser.Passkey, func(item *pb.Passkey, index int) *ent.Passkey {
			return ProtoPasskeyToEnt(item)
		})
	}

	if protoUser.DavAccounts != nil {
		entUser.Edges.DavAccounts = lo.Map(protoUser.DavAccounts, func(item *pb.DavAccount, index int) *ent.DavAccount {
			return ProtoDavAccountToEnt(item)
		})
	}

	return entUser
}

func EntGroupToProto(group *ent.Group) *pb.Group {
	protoGroup := &pb.Group{
		Id:                int64(group.ID),
		CreatedAt:         timestamppb.New(group.CreatedAt),
		UpdatedAt:         timestamppb.New(group.UpdatedAt),
		Name:              group.Name,
		MaxStorage:        &wrapperspb.Int64Value{Value: group.MaxStorage},
		SpeedLimit:        &wrapperspb.Int64Value{Value: int64(group.SpeedLimit)},
		Permissions:       *group.Permissions,
		Settings:          group.Settings,
		StoragePolicyId:   &wrapperspb.Int64Value{Value: int64(group.StoragePolicyID)},
		StoragePolicyInfo: group.StoragePolicyInfo,
	}

	if group.DeletedAt != nil {
		protoGroup.DeletedAt = timestamppb.New(*group.DeletedAt)
	}

	if group.Edges.Users != nil {
		protoGroup.Users = lo.Map(group.Edges.Users, func(item *ent.User, index int) *pb.User {
			return EntUserToProto(item)
		})
	}

	return protoGroup
}

func EntPasskeyToProto(passkey *ent.Passkey) *pb.Passkey {
	protoPasskey := &pb.Passkey{
		Id:           int64(passkey.ID),
		CreatedAt:    timestamppb.New(passkey.CreatedAt),
		UpdatedAt:    timestamppb.New(passkey.UpdatedAt),
		UserId:       int64(passkey.UserID),
		CredentialId: passkey.CredentialID,
		Credential:   "",
		Name:         passkey.Name,
	}

	if passkey.DeletedAt != nil {
		protoPasskey.DeletedAt = timestamppb.New(*passkey.DeletedAt)
	}

	if passkey.UsedAt != nil {
		protoPasskey.UsedAt = timestamppb.New(*passkey.UsedAt)
	}

	if passkey.Edges.Users != nil {
		protoPasskey.Users = EntUserToProto(passkey.Edges.Users)
	}

	return protoPasskey
}

func ProtoGroupToEnt(protoGroup *pb.Group) *ent.Group {
	entGroup := &ent.Group{
		ID:                int(protoGroup.Id),
		CreatedAt:         protoGroup.CreatedAt.AsTime(),
		UpdatedAt:         protoGroup.UpdatedAt.AsTime(),
		Name:              protoGroup.Name,
		MaxStorage:        protoGroup.MaxStorage.GetValue(),
		SpeedLimit:        int(protoGroup.SpeedLimit.GetValue()),
		Settings:          protoGroup.Settings,
		StoragePolicyID:   int(protoGroup.StoragePolicyId.GetValue()),
		StoragePolicyInfo: protoGroup.StoragePolicyInfo,
		Edges:             ent.GroupEdges{},
	}

	if protoGroup.DeletedAt != nil {
		deletedAt := protoGroup.DeletedAt.AsTime()
		entGroup.DeletedAt = &deletedAt
	}

	if protoGroup.Permissions != nil {
		permissions := boolset.BooleanSet(protoGroup.Permissions)
		entGroup.Permissions = &permissions
	}

	if protoGroup.Users != nil {
		entGroup.Edges.Users = lo.Map(protoGroup.Users, func(item *pb.User, index int) *ent.User {
			return ProtoUserToEnt(item)
		})
	}

	return entGroup
}

func ProtoPasskeyToEnt(protoPasskey *pb.Passkey) *ent.Passkey {
	entPasskey := &ent.Passkey{
		ID:           int(protoPasskey.Id),
		CreatedAt:    protoPasskey.CreatedAt.AsTime(),
		UpdatedAt:    protoPasskey.UpdatedAt.AsTime(),
		UserID:       int(protoPasskey.UserId),
		CredentialID: protoPasskey.CredentialId,
		Name:         protoPasskey.Name,
		Credential:   nil, // This would need to be handled based on how the credential is stored
		Edges:        ent.PasskeyEdges{},
	}

	if protoPasskey.DeletedAt != nil {
		deletedAt := protoPasskey.DeletedAt.AsTime()
		entPasskey.DeletedAt = &deletedAt
	}

	if protoPasskey.UsedAt != nil {
		usedAt := protoPasskey.UsedAt.AsTime()
		entPasskey.UsedAt = &usedAt
	}

	if protoPasskey.Users != nil {
		entPasskey.Edges.Users = ProtoUserToEnt(protoPasskey.Users)
	}

	return entPasskey
}

func EntDavAccountToProto(dav *ent.DavAccount) *pb.DavAccount {
	protoDavAccount := &pb.DavAccount{
		Id:        int64(dav.ID),
		CreatedAt: timestamppb.New(dav.CreatedAt),
		UpdatedAt: timestamppb.New(dav.UpdatedAt),
		Name:      dav.Name,
		Uri:       dav.URI,
		Password:  dav.Password,
		Options:   []byte(*dav.Options),
		Props:     EntDavPropsToProto(dav.Props),
		OwnerId:   int64(dav.OwnerID),
	}

	return protoDavAccount
}

func EntDavPropsToProto(props *pb.DavAccountProps) *pb.DavAccountProps {
	if props == nil {
		return nil
	}
	return &pb.DavAccountProps{}
}

func ProtoDavAccountToEnt(davAccount *pb.DavAccount) *ent.DavAccount {
	// Handle nil input
	if davAccount == nil {
		return nil
	}
	entDavAccount := &ent.DavAccount{
		ID:        int(davAccount.GetId()),
		CreatedAt: davAccount.GetCreatedAt().AsTime(),
		UpdatedAt: davAccount.GetUpdatedAt().AsTime(),
		Name:      davAccount.GetName(),
		URI:       davAccount.GetUri(),
		Password:  davAccount.GetPassword(),
		OwnerID:   int(davAccount.GetOwnerId()),
	}

	// Handle optional fields
	if davAccount.GetOptions() != nil {
		options := boolset.BooleanSet(davAccount.GetOptions())
		entDavAccount.Options = &options
	}

	if davAccount.GetProps() != nil {
		entDavAccount.Props = davAccount.GetProps()
	}

	return entDavAccount
}

func GetUserStatus(status user.Status) pb.User_Status {
	switch status {
	case user.StatusActive:
		return pb.User_STATUS_ACTIVE
	case user.StatusManualBanned:
		return pb.User_STATUS_MANUAL_BANNED
	case user.StatusInactive:
		return pb.User_STATUS_INACTIVE
	default:
		return pb.User_STATUS_INACTIVE
	}
}
func ProtoUserStatusToEnt(protoStatus pb.User_Status) user.Status {
	switch protoStatus {
	case pb.User_STATUS_ACTIVE:
		return user.StatusActive
	case pb.User_STATUS_INACTIVE:
		return user.StatusInactive
	case pb.User_STATUS_MANUAL_BANNED:
		return user.StatusManualBanned
	case pb.User_STATUS_SYS_BANNED:
		return user.StatusSysBanned
	default:
		return user.DefaultStatus
	}
}
