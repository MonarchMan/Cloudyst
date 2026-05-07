package utils

import (
	pb "api/api/file/common/v1"
	pbslave "api/api/file/slave/v1"
	"api/external/data/filedata"
	"api/external/data/userdata"
	"common/boolset"
	"file/ent"
	nodeent "file/ent/node"
	"file/internal/biz/filemanager/fs"
	"file/internal/data"
	"file/internal/data/types"
	"queue"

	"github.com/gofrs/uuid"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func EntFileToProto(file *ent.File) *pb.File {
	if file == nil {
		return nil
	}
	protoFile := &pb.File{
		Id:            int64(file.ID),
		CreatedAt:     timestamppb.New(file.CreatedAt),
		UpdatedAt:     timestamppb.New(file.UpdatedAt),
		Type:          int64(file.Type),
		Name:          file.Name,
		OwnerId:       int64(file.OwnerID),
		OwnerInfo:     userdata.UserInfoToProto(file.OwnerInfo),
		Size:          file.Size,
		PrimaryEntity: int64(file.PrimaryEntity),
		FileParentId:  int64(file.FileParentID),
		IsSymbolic:    file.IsSymbolic,
		Props: &pb.FileProps{
			View: filedata.ExplorerViewToProto(file.Props.View),
		},
		StoragePolicyFiles: int64(file.StoragePolicyFiles),
	}
	if file.Edges.StoragePolicies != nil {
		protoFile.StoragePolicies = EntPolicyToProto(file.Edges.StoragePolicies)
	}

	if file.Edges.Parent != nil {
		protoFile.Parent = EntFileToProto(file.Edges.Parent)
	}

	if file.Edges.Children != nil {
		protoFile.Children = lo.Map(file.Edges.Children, func(item *ent.File, index int) *pb.File {
			return EntFileToProto(item)
		})
	}

	if file.Edges.Metadata != nil {
		protoFile.Metadata = lo.Map(file.Edges.Metadata, func(item *ent.Metadata, index int) *pb.Metadata {
			return EntMetadataToProto(item)
		})
	}

	if file.Edges.Entities != nil {
		protoFile.Entities = lo.Map(file.Edges.Entities, func(item *ent.Entity, index int) *pb.Entity {
			return EntEntityToProto(item)
		})
	}

	if file.Edges.Shares != nil {
		protoFile.Shares = lo.Map(file.Edges.Shares, func(item *ent.Share, index int) *pb.Share {
			return EntShareToProto(item)
		})
	}

	if file.Edges.DirectLinks != nil {
		protoFile.DirectLinks = lo.Map(file.Edges.DirectLinks, func(item *ent.DirectLink, index int) *pb.DirectLink {
			return EntDirectLinkToProto(item)
		})
	}

	return protoFile
}

func ProtoFileToEnt(protoFile *pb.File) *ent.File {
	if protoFile == nil {
		return nil
	}
	entFile := &ent.File{
		ID:         int(protoFile.Id),
		CreatedAt:  protoFile.CreatedAt.AsTime(),
		UpdatedAt:  protoFile.UpdatedAt.AsTime(),
		Type:       int(protoFile.Type),
		Name:       protoFile.Name,
		OwnerID:    int(protoFile.OwnerId),
		OwnerInfo:  userdata.UserInfoFromProto(protoFile.OwnerInfo),
		Size:       protoFile.Size,
		IsSymbolic: protoFile.IsSymbolic,
		Props: &types.FileProps{
			View: filedata.ExplorerViewFromProto(protoFile.Props.View),
		},
		PrimaryEntity:      int(protoFile.PrimaryEntity),
		FileParentID:       int(protoFile.FileParentId),
		StoragePolicyFiles: int(protoFile.StoragePolicyFiles),
	}

	entFile.Edges = ent.FileEdges{}
	if protoFile.StoragePolicies != nil {
		entFile.Edges.StoragePolicies = ProtoPolicyToEnt(protoFile.StoragePolicies)
	}

	if protoFile.Parent != nil {
		entFile.Edges.Parent = ProtoFileToEnt(protoFile.Parent)
	}

	if protoFile.Children != nil {
		entFile.Edges.Children = lo.Map(protoFile.Children, func(item *pb.File, index int) *ent.File {
			return ProtoFileToEnt(item)
		})
	}

	if protoFile.Metadata != nil {
		entFile.Edges.Metadata = lo.Map(protoFile.Metadata, func(item *pb.Metadata, index int) *ent.Metadata {
			return ProtoMetadataToEnt(item)
		})
	}

	if protoFile.Entities != nil {
		entFile.Edges.Entities = lo.Map(protoFile.Entities, func(item *pb.Entity, index int) *ent.Entity {
			return ProtoEntityToEnt(item)
		})
	}

	if protoFile.Shares != nil {
		entFile.Edges.Shares = lo.Map(protoFile.Shares, func(item *pb.Share, index int) *ent.Share {
			return ProtoShareToEnt(item)
		})
	}

	if protoFile.DirectLinks != nil {
		entFile.Edges.DirectLinks = lo.Map(protoFile.DirectLinks, func(item *pb.DirectLink, index int) *ent.DirectLink {
			return ProtoDirectLinkToEnt(item)
		})
	}

	return entFile
}

func EntDirectLinkToProto(directLink *ent.DirectLink) *pb.DirectLink {
	if directLink == nil {
		return nil
	}
	protoDirectLink := &pb.DirectLink{
		Id:        int64(directLink.ID),
		CreatedAt: timestamppb.New(directLink.CreatedAt),
		UpdatedAt: timestamppb.New(directLink.UpdatedAt),
		Name:      directLink.Name,
		Downloads: int64(directLink.Downloads),
		FileId:    int64(directLink.FileID),
		Speed:     int64(directLink.Speed),
	}

	if directLink.DeletedAt != nil {
		protoDirectLink.DeletedAt = timestamppb.New(*directLink.DeletedAt)
	}

	if directLink.Edges.File != nil {
		protoDirectLink.File = EntFileToProto(directLink.Edges.File)
	}
	return protoDirectLink
}

func EntShareToProto(share *ent.Share) *pb.Share {
	if share == nil {
		return nil
	}
	protoShare := &pb.Share{
		Id:        int64(share.ID),
		CreatedAt: timestamppb.New(share.CreatedAt),
		UpdatedAt: timestamppb.New(share.UpdatedAt),
		Password:  share.Password,
		Views:     int64(share.Views),
		Downloads: int64(share.Downloads),
		Props:     SharePropsToProto(share.Props),
		OwnerId:   int64(share.OwnerID),
		OwnerInfo: userdata.UserInfoToProto(share.OwnerInfo),
	}

	if share.RemainDownloads != nil {
		remainDownloads := int64(*share.RemainDownloads)
		protoShare.RemainDownloads = &remainDownloads
	}

	if share.Expires != nil {
		protoShare.Expires = timestamppb.New(*share.Expires)
	}

	if share.DeletedAt != nil {
		protoShare.DeletedAt = timestamppb.New(*share.DeletedAt)
	}

	if share.Edges.File != nil {
		protoShare.File = EntFileToProto(share.Edges.File)
	}

	return protoShare
}

func ProtoShareToEnt(share *pb.Share) *ent.Share {
	if share == nil {
		return nil
	}
	entShare := &ent.Share{
		ID:        int(share.GetId()),
		CreatedAt: share.CreatedAt.AsTime(),
		UpdatedAt: share.UpdatedAt.AsTime(),
		Password:  share.GetPassword(),
		Views:     int(share.GetViews()),
		Downloads: int(share.GetDownloads()),
		Props:     SharePropsFromProto(share.GetProps()),
		OwnerID:   int(share.GetOwnerId()),
		OwnerInfo: userdata.UserInfoFromProto(share.GetOwnerInfo()),
	}

	// Handle Expires field
	if share.GetExpires() != nil {
		expires := share.GetExpires().AsTime()
		entShare.Expires = &expires
	}

	// Handle RemainDownloads field
	if share.RemainDownloads != nil {
		remainDownloads := int(share.GetRemainDownloads())
		entShare.RemainDownloads = &remainDownloads
	}

	// Handle DeletedAt field
	if share.GetDeletedAt() != nil {
		deletedAt := share.GetDeletedAt().AsTime()
		entShare.DeletedAt = &deletedAt
	}

	if share.File != nil {
		entShare.Edges = ent.ShareEdges{File: ProtoFileToEnt(share.File)}
	}

	return entShare
}

func SharePropsToProto(props *types.ShareProps) *pb.ShareProps {
	if props == nil {
		return nil
	}
	return &pb.ShareProps{
		ShareView:  props.ShareView,
		ShowReadMe: props.ShowReadMe,
	}
}

func SharePropsFromProto(props *pb.ShareProps) *types.ShareProps {
	if props == nil {
		return nil
	}
	return &types.ShareProps{
		ShareView:  props.ShareView,
		ShowReadMe: props.ShowReadMe,
	}
}

func EntMetadataToProto(metadata *ent.Metadata) *pb.Metadata {
	if metadata == nil {
		return nil
	}
	protoMetadata := &pb.Metadata{
		Id:        int64(metadata.ID),
		CreatedAt: timestamppb.New(metadata.CreatedAt),
		UpdatedAt: timestamppb.New(metadata.UpdatedAt),
		Name:      metadata.Name,
		Value:     metadata.Value,
		FileId:    int64(metadata.FileID),
		IsPublic:  metadata.IsPublic,
	}

	if metadata.DeletedAt != nil {
		protoMetadata.DeletedAt = timestamppb.New(*metadata.DeletedAt)
	}

	if metadata.Edges.File != nil {
		protoMetadata.File = EntFileToProto(metadata.Edges.File)
	}

	return protoMetadata
}

func EntPolicyToProto(storagePolicy *ent.StoragePolicy) *pb.StoragePolicy {
	if storagePolicy == nil {
		return nil
	}
	protoStoragePolicy := &pb.StoragePolicy{
		Id:           int64(storagePolicy.ID),
		CreatedAt:    timestamppb.New(storagePolicy.CreatedAt),
		UpdatedAt:    timestamppb.New(storagePolicy.UpdatedAt),
		Name:         storagePolicy.Name,
		Type:         storagePolicy.Type,
		Server:       storagePolicy.Server,
		BucketName:   storagePolicy.BucketName,
		IsPrivate:    storagePolicy.IsPrivate,
		AccessKey:    storagePolicy.AccessKey,
		SecretKey:    storagePolicy.SecretKey,
		MaxSize:      storagePolicy.MaxSize,
		DirNameRule:  storagePolicy.DirNameRule,
		FileNameRule: storagePolicy.FileNameRule,
		Settings:     PolicySettingToProto(storagePolicy.Settings),
		NodeId:       int64(storagePolicy.NodeID),
	}

	if storagePolicy.Edges.Files != nil {
		protoStoragePolicy.Files = lo.Map(storagePolicy.Edges.Files, func(item *ent.File, index int) *pb.File {
			return EntFileToProto(item)
		})
	}

	if storagePolicy.Edges.Entities != nil {
		protoStoragePolicy.Entities = lo.Map(storagePolicy.Edges.Entities, func(item *ent.Entity, index int) *pb.Entity {
			return EntEntityToProto(item)
		})
	}

	if storagePolicy.Edges.Node != nil {
		protoStoragePolicy.Node = EntNodeToProto(storagePolicy.Edges.Node)
	}

	return protoStoragePolicy
}

func PolicySettingToProto(settings *types.PolicySetting) *pb.PolicySetting {
	if settings == nil {
		return nil
	}
	return &pb.PolicySetting{
		Token:                   settings.Token,
		FileType:                settings.FileType,
		IsFileTypeDenyList:      settings.IsFileTypeDenyList,
		NameRegexp:              settings.NameRegexp,
		IsNameRegexpDenyList:    settings.IsNameRegexpDenyList,
		OauthRedirect:           settings.OauthRedirect,
		CustomProxy:             settings.CustomProxy,
		ProxyServer:             settings.ProxyServer,
		InternalProxy:           settings.InternalProxy,
		OdDriver:                settings.OdDriver,
		Region:                  settings.Region,
		ServerSideEndpoint:      settings.ServerSideEndpoint,
		ChunkSize:               settings.ChunkSize,
		TpsLimit:                settings.TPSLimit,
		TpsLimitBurst:           int32(settings.TPSLimitBurst),
		S3ForcePathStyle:        settings.S3ForcePathStyle,
		ThumbExts:               settings.ThumbExts,
		ThumbSupportAllExts:     settings.ThumbSupportAllExts,
		ThumbMaxSize:            settings.ThumbMaxSize,
		Relay:                   settings.Relay,
		PreAllocate:             settings.PreAllocate,
		MediaMetaExts:           settings.MediaMetaExts,
		MediaMetaGeneratorProxy: settings.MediaMetaGeneratorProxy,
		ThumbGeneratorProxy:     settings.ThumbGeneratorProxy,
		NativeMediaProcessing:   settings.NativeMediaProcessing,
		S3DeleteBatchSize:       int32(settings.S3DeleteBatchSize),
		StreamSaver:             settings.StreamSaver,
		UseCname:                settings.UseCname,
		SourceAuth:              settings.SourceAuth,
		QiniuUploadCdn:          settings.QiniuUploadCdn,
		ChunkConcurrency:        int32(settings.ChunkConcurrency),
	}
}

func PolicySettingFromProto(settings *pb.PolicySetting) *types.PolicySetting {
	if settings == nil {
		return nil
	}
	return &types.PolicySetting{
		Token:                   settings.Token,
		FileType:                settings.FileType,
		IsFileTypeDenyList:      settings.IsFileTypeDenyList,
		NameRegexp:              settings.NameRegexp,
		IsNameRegexpDenyList:    settings.IsNameRegexpDenyList,
		OauthRedirect:           settings.OauthRedirect,
		CustomProxy:             settings.CustomProxy,
		ProxyServer:             settings.ProxyServer,
		InternalProxy:           settings.InternalProxy,
		OdDriver:                settings.OdDriver,
		Region:                  settings.Region,
		ServerSideEndpoint:      settings.ServerSideEndpoint,
		ChunkSize:               settings.ChunkSize,
		TPSLimit:                settings.TpsLimit,
		TPSLimitBurst:           int(settings.TpsLimitBurst),
		S3ForcePathStyle:        settings.S3ForcePathStyle,
		ThumbExts:               settings.ThumbExts,
		ThumbSupportAllExts:     settings.ThumbSupportAllExts,
		ThumbMaxSize:            settings.ThumbMaxSize,
		Relay:                   settings.Relay,
		PreAllocate:             settings.PreAllocate,
		MediaMetaExts:           settings.MediaMetaExts,
		MediaMetaGeneratorProxy: settings.MediaMetaGeneratorProxy,
		ThumbGeneratorProxy:     settings.ThumbGeneratorProxy,
		NativeMediaProcessing:   settings.NativeMediaProcessing,
		S3DeleteBatchSize:       int(settings.S3DeleteBatchSize),
		StreamSaver:             settings.StreamSaver,
		UseCname:                settings.UseCname,
		SourceAuth:              settings.SourceAuth,
		QiniuUploadCdn:          settings.QiniuUploadCdn,
		ChunkConcurrency:        int(settings.ChunkConcurrency),
	}
}

func NodeSettingToProto(settings *types.NodeSetting) *pb.NodeSetting {
	if settings == nil {
		return nil
	}
	return &pb.NodeSetting{
		Provider:       DownloaderProviderToProto(settings.Provider),
		Qbittorrent:    QBittorrentSettingToProto(settings.QBittorrentSetting),
		Aria2:          Aria2SettingToProto(settings.Aria2Setting),
		Interval:       int32(settings.Interval),
		WaitForSeeding: settings.WaitForSeeding,
	}
}

func DownloaderProviderToProto(provider types.DownloaderProvider) pb.DownloaderProvider {
	switch provider {
	case types.DownloaderProviderQBittorrent:
		return pb.DownloaderProvider_DOWNLOADER_PROVIDER_QBITTORRENT
	case types.DownloaderProviderAria2:
		return pb.DownloaderProvider_DOWNLOADER_PROVIDER_ARIA2
	default:
		return pb.DownloaderProvider_DOWNLOADER_PROVIDER_UNSPECIFIED
	}
}

func QBittorrentSettingToProto(settings *types.QBittorrentSetting) *pb.QBittorrentSetting {
	if settings == nil {
		return nil
	}
	qbs := &pb.QBittorrentSetting{
		Server:   settings.Server,
		User:     settings.User,
		TempPath: settings.TempPath,
	}
	qbs.Options, _ = structpb.NewStruct(settings.Options)
	return qbs
}

func Aria2SettingToProto(settings *types.Aria2Setting) *pb.Aria2Setting {
	if settings == nil {
		return nil
	}
	a2s := &pb.Aria2Setting{
		Server:   settings.Server,
		Token:    settings.Token,
		TempPath: settings.TempPath,
	}
	a2s.Options, _ = structpb.NewStruct(settings.Options)
	return a2s
}

func EntNodeToProto(node *ent.Node) *pb.Node {
	if node == nil {
		return nil
	}
	protoNode := &pb.Node{
		Id:           int64(node.ID),
		CreatedAt:    timestamppb.New(node.CreatedAt),
		UpdatedAt:    timestamppb.New(node.UpdatedAt),
		Status:       getNodeStatus(node),
		Name:         node.Name,
		Type:         getNodeType(node),
		Server:       node.Server,
		SlaveKey:     node.SlaveKey,
		Capabilities: []byte(*node.Capabilities),
		Settings:     node.Settings,
		Weight:       int64(node.Weight),
	}

	if node.DeletedAt != nil {
		protoNode.DeletedAt = timestamppb.New(*node.DeletedAt)
	}

	if node.Edges.StoragePolicy != nil {
		protoNode.StoragePolicy = lo.Map(node.Edges.StoragePolicy, func(item *ent.StoragePolicy, index int) *pb.StoragePolicy {
			return EntPolicyToProto(item)
		})
	}

	return protoNode
}

func getNodeType(node *ent.Node) pb.Node_Type {
	switch node.Type {
	case nodeent.TypeMaster:
		return pb.Node_TYPE_MASTER
	case nodeent.TypeSlave:
		return pb.Node_TYPE_SLAVE
	default:
		return pb.Node_TYPE_UNSPECIFIED
	}
}

func getNodeStatus(node *ent.Node) pb.Node_Status {
	switch node.Status {
	case nodeent.StatusActive:
		return pb.Node_STATUS_ACTIVE
	case nodeent.StatusSuspended:
		return pb.Node_STATUS_SUSPENDED
	default:
		return pb.Node_STATUS_UNSPECIFIED
	}
}

func EntEntityToProto(entity *ent.Entity) *pb.Entity {
	protoEntity := &pb.Entity{
		Id:                    int64(entity.ID),
		CreatedAt:             timestamppb.New(entity.CreatedAt),
		UpdatedAt:             timestamppb.New(entity.UpdatedAt),
		DeletedAt:             nil,
		Type:                  int64(entity.Type),
		Source:                entity.Source,
		Size:                  entity.Size,
		ReferenceCount:        int64(entity.ReferenceCount),
		StoragePolicyEntities: int64(entity.StoragePolicyEntities),
		CreatedBy:             int64(entity.CreatedBy),
	}

	if entity.Props != nil {
		protoEntity.Props = &pb.EntityProps{
			UnlinkOnly: entity.Props.UnlinkOnly,
		}
		if entity.Props.EncryptMetadata != nil {
			protoEntity.Props.EncryptMetadata = &pb.EncryptMetadata{
				Algorithm:    string(entity.Props.EncryptMetadata.Algorithm),
				Key:          entity.Props.EncryptMetadata.Key,
				KeyPlainText: entity.Props.EncryptMetadata.KeyPlainText,
				Iv:           entity.Props.EncryptMetadata.IV,
			}
		}
	}

	if entity.UploadSessionID != nil {
		protoEntity.UploadSessionId = entity.UploadSessionID.Bytes()
	}

	if entity.Edges.File != nil {
		protoEntity.File = lo.Map(entity.Edges.File, func(item *ent.File, index int) *pb.File {
			return EntFileToProto(item)
		})
	}

	if entity.Edges.StoragePolicy != nil {
		protoEntity.StoragePolicy = EntPolicyToProto(entity.Edges.StoragePolicy)
	}

	return protoEntity
}

func EntSettingToProto(setting *ent.Setting) *pb.Setting {
	protoSetting := &pb.Setting{
		Id:        int64(setting.ID),
		CreatedAt: timestamppb.New(setting.CreatedAt),
		UpdatedAt: timestamppb.New(setting.UpdatedAt),
		Name:      setting.Name,
		Value:     setting.Value,
	}

	if setting.DeletedAt != nil {
		protoSetting.DeletedAt = timestamppb.New(*setting.DeletedAt)
	}

	return protoSetting
}

func EntTaskToProto(task *ent.Task) *pb.Task {
	protoTask := &pb.Task{
		Id:           int64(task.ID),
		CreatedAt:    timestamppb.New(task.CreatedAt),
		UpdatedAt:    timestamppb.New(task.UpdatedAt),
		DeletedAt:    nil,
		Type:         task.Type,
		Status:       getTaskStatus(task),
		PublicState:  TaskStateToProto(task.PublicState),
		PrivateState: task.PrivateState,
		TraceId:      task.TraceID,
		UserId:       int64(task.UserID),
	}

	return protoTask
}

func getTaskStatus(task *ent.Task) pb.Task_Status {
	switch task.Status {
	case queue.StatusSuspending:
		return pb.Task_STATUS_SUSPENDING
	case queue.StatusProcessing:
		return pb.Task_STATUS_PROCESSING
	case queue.StatusCompleted:
		return pb.Task_STATUS_COMPLETED
	case queue.StatusCanceled:
		return pb.Task_STATUS_CANCELED
	case queue.StatusError:
		return pb.Task_STATUS_ERROR
	default:
		return pb.Task_STATUS_QUEUED
	}
}

func TaskStateToProto(publicState *queue.TaskPublicState) *pb.TaskPublicState {
	if publicState == nil {
		return nil
	}
	return &pb.TaskPublicState{
		Error:            publicState.Error,
		ErrorHistory:     publicState.ErrorHistory,
		ExecutedDuration: durationpb.New(publicState.ExecutedDuration),
		RetryCount:       int32(publicState.RetryCount),
		ResumeTime:       publicState.ResumeTime,
		SlaveTaskProps:   SlaveTaskPropsToProto(publicState.SlaveTaskProps),
	}
}

func SlaveTaskPropsToProto(props *queue.SlaveTaskProps) *pb.SlaveTaskProps {
	if props == nil {
		return nil
	}
	return &pb.SlaveTaskProps{
		NodeId:            int32(props.NodeID),
		MasterSiteUrl:     props.MasterSiteURl,
		MasterSiteId:      props.MasterSiteID,
		MasterSiteVersion: props.MasterSiteVersion,
	}
}

func SlaveTaskPropsFromProto(props *pb.SlaveTaskProps) *queue.SlaveTaskProps {
	if props == nil {
		return nil
	}
	return &queue.SlaveTaskProps{
		NodeID:            int(props.NodeId),
		MasterSiteURl:     props.MasterSiteUrl,
		MasterSiteID:      props.MasterSiteId,
		MasterSiteVersion: props.MasterSiteVersion,
	}
}

func ProtoPolicyToEnt(policy *pb.StoragePolicy) *ent.StoragePolicy {
	// Handle nil input
	if policy == nil {
		return nil
	}
	entPolicy := &ent.StoragePolicy{
		ID:           int(policy.Id),
		CreatedAt:    policy.CreatedAt.AsTime(),
		UpdatedAt:    policy.UpdatedAt.AsTime(),
		Name:         policy.Name,
		Type:         policy.Type,
		Server:       policy.Server,
		BucketName:   policy.BucketName,
		IsPrivate:    policy.IsPrivate,
		AccessKey:    policy.AccessKey,
		SecretKey:    policy.SecretKey,
		MaxSize:      policy.MaxSize,
		DirNameRule:  policy.DirNameRule,
		FileNameRule: policy.FileNameRule,
		NodeID:       int(policy.NodeId),
		Settings:     PolicySettingFromProto(policy.Settings),
	}

	// Handle DeletedAt field
	if policy.GetDeletedAt() != nil {
		deletedAt := policy.GetDeletedAt().AsTime()
		entPolicy.DeletedAt = &deletedAt
	}

	// Handle Edges
	entPolicy.Edges = ent.StoragePolicyEdges{}
	if policy.GetFiles() != nil {
		entPolicy.Edges.Files = lo.Map(policy.GetFiles(), func(item *pb.File, index int) *ent.File {
			return ProtoFileToEnt(item)
		})
	}

	if policy.GetEntities() != nil {
		entPolicy.Edges.Entities = lo.Map(policy.GetEntities(), func(item *pb.Entity, index int) *ent.Entity {
			return ProtoEntityToEnt(item)
		})
	}

	if policy.GetNode() != nil {
		entPolicy.Edges.Node = ProtoNodeToEnt(policy.GetNode())
	}

	return entPolicy
}

func ProtoMetadataToEnt(metadata *pb.Metadata) *ent.Metadata {
	// Handle nil input
	if metadata == nil {
		return nil
	}
	entMetadata := &ent.Metadata{
		ID:        int(metadata.GetId()),
		CreatedAt: metadata.CreatedAt.AsTime(),
		UpdatedAt: metadata.UpdatedAt.AsTime(),
		Name:      metadata.Name,
		Value:     metadata.Value,
		FileID:    int(metadata.GetFileId()),
		IsPublic:  metadata.GetIsPublic(),
	}

	// Handle DeletedAt field
	if metadata.GetDeletedAt() != nil {
		deletedAt := metadata.GetDeletedAt().AsTime()
		entMetadata.DeletedAt = &deletedAt
	}

	// Handle Edges
	entMetadata.Edges = ent.MetadataEdges{}
	if metadata.GetFile() != nil {
		entMetadata.Edges.File = ProtoFileToEnt(metadata.GetFile())
	}

	return entMetadata
}

func ProtoEntityToEnt(entity *pb.Entity) *ent.Entity {
	// Handle nil input
	if entity == nil {
		return nil
	}
	entEntity := &ent.Entity{
		ID:                    int(entity.Id),
		CreatedAt:             entity.CreatedAt.AsTime(),
		UpdatedAt:             entity.UpdatedAt.AsTime(),
		Type:                  int(entity.Type),
		Source:                entity.Source,
		Size:                  entity.Size,
		ReferenceCount:        int(entity.ReferenceCount),
		StoragePolicyEntities: int(entity.StoragePolicyEntities),
		CreatedBy:             int(entity.CreatedBy),
	}

	// Handle optional fields
	if entity.UploadSessionId != nil {
		uploadSessionID, _ := uuid.FromBytes(entity.UploadSessionId)
		entEntity.UploadSessionID = &uploadSessionID
	}

	if entity.Props != nil {
		entEntity.Props = fromProtoEntityProps(entity.Props)
	}

	// Handle DeletedAt field
	if entity.GetDeletedAt() != nil {
		deletedAt := entity.GetDeletedAt().AsTime()
		entEntity.DeletedAt = &deletedAt
	}

	// Handle Edges
	entEntity.Edges = ent.EntityEdges{}
	if entity.GetFile() != nil {
		entEntity.Edges.File = lo.Map(entity.GetFile(), func(item *pb.File, index int) *ent.File {
			return ProtoFileToEnt(item)
		})
	}

	if entity.GetStoragePolicy() != nil {
		entEntity.Edges.StoragePolicy = ProtoPolicyToEnt(entity.GetStoragePolicy())
	}

	return entEntity
}

func fromProtoEntityProps(protoProps *pb.EntityProps) *types.EntityProps {
	props := &types.EntityProps{
		UnlinkOnly: protoProps.UnlinkOnly,
	}
	if protoProps.EncryptMetadata != nil {
		props.EncryptMetadata = &types.EncryptMetadata{
			Algorithm:    types.Cipher(protoProps.EncryptMetadata.Algorithm),
			Key:          protoProps.EncryptMetadata.Key,
			KeyPlainText: protoProps.EncryptMetadata.KeyPlainText,
			IV:           protoProps.EncryptMetadata.Iv,
		}
	}
	return props
}

func ProtoDirectLinkToEnt(directLink *pb.DirectLink) *ent.DirectLink {
	// Handle nil input
	if directLink == nil {
		return nil
	}
	entDirectLink := &ent.DirectLink{
		ID:        int(directLink.GetId()),
		CreatedAt: directLink.CreatedAt.AsTime(),
		UpdatedAt: directLink.UpdatedAt.AsTime(),
		Name:      directLink.Name,
		Downloads: int(directLink.GetDownloads()),
		FileID:    int(directLink.GetFileId()),
		Speed:     int(directLink.GetSpeed()),
	}

	// Handle DeletedAt field
	if directLink.GetDeletedAt() != nil {
		deletedAt := directLink.GetDeletedAt().AsTime()
		entDirectLink.DeletedAt = &deletedAt
	}

	// Handle Edges
	entDirectLink.Edges = ent.DirectLinkEdges{}
	if directLink.GetFile() != nil {
		entDirectLink.Edges.File = ProtoFileToEnt(directLink.GetFile())
	}

	return entDirectLink
}

func ProtoSettingToEnt(setting *pb.Setting) *ent.Setting {
	// Handle nil input
	if setting == nil {
		return nil
	}
	entSetting := &ent.Setting{
		ID:        int(setting.GetId()),
		CreatedAt: setting.CreatedAt.AsTime(),
		UpdatedAt: setting.UpdatedAt.AsTime(),
		Name:      setting.Name,
		Value:     setting.Value,
	}

	// Handle DeletedAt field
	if setting.GetDeletedAt() != nil {
		deletedAt := setting.GetDeletedAt().AsTime()
		entSetting.DeletedAt = &deletedAt
	}

	return entSetting
}

func ProtoTaskToEnt(task *pb.Task) *ent.Task {
	// Handle nil input
	if task == nil {
		return nil
	}
	entTask := &ent.Task{
		ID:           int(task.Id),
		CreatedAt:    task.CreatedAt.AsTime(),
		UpdatedAt:    task.UpdatedAt.AsTime(),
		Type:         task.Type,
		Status:       getEntTaskStatus(task.Status),
		TraceID:      task.TraceId,
		PrivateState: task.PrivateState,
		UserID:       int(task.UserId),
	}

	// Handle optional fields
	if task.GetPublicState() != nil {
		entTask.PublicState = &queue.TaskPublicState{
			Error:            task.PublicState.Error,
			ErrorHistory:     task.PublicState.ErrorHistory,
			ExecutedDuration: task.PublicState.ExecutedDuration.AsDuration(),
			RetryCount:       int(task.PublicState.RetryCount),
			ResumeTime:       task.PublicState.ResumeTime,
			SlaveTaskProps:   SlaveTaskPropsFromProto(task.PublicState.SlaveTaskProps),
		}
	}

	// Handle DeletedAt field
	if task.DeletedAt != nil {
		deletedAt := task.DeletedAt.AsTime()
		entTask.DeletedAt = &deletedAt
	}

	return entTask
}

func getEntTaskStatus(status pb.Task_Status) queue.TaskStatus {
	switch status {
	case pb.Task_STATUS_SUSPENDING:
		return queue.StatusSuspending
	case pb.Task_STATUS_PROCESSING:
		return queue.StatusProcessing
	case pb.Task_STATUS_COMPLETED:
		return queue.StatusCompleted
	case pb.Task_STATUS_CANCELED:
		return queue.StatusCanceled
	case pb.Task_STATUS_ERROR:
		return queue.StatusError
	default:
		return queue.StatusQueued
	}
}

func ProtoNodeToEnt(node *pb.Node) *ent.Node {
	// Handle nil input
	if node == nil {
		return nil
	}
	entNode := &ent.Node{
		ID:        int(node.GetId()),
		CreatedAt: node.CreatedAt.AsTime(),
		UpdatedAt: node.UpdatedAt.AsTime(),
		Name:      node.Name,
		Server:    node.Server,
		SlaveKey:  node.SlaveKey,
		Settings:  node.Settings,
		Weight:    int(node.Weight),
		Type:      getEntNodeType(node.Type),
		Status:    getEntNodeStatus(node.Status),
	}

	// Handle optional fields
	if node.Capabilities != nil {
		capabilities := boolset.BooleanSet(node.Capabilities)
		entNode.Capabilities = &capabilities
	}

	// Handle DeletedAt field
	if node.DeletedAt != nil {
		deletedAt := node.DeletedAt.AsTime()
		entNode.DeletedAt = &deletedAt
	}

	// Handle Edges
	entNode.Edges = ent.NodeEdges{}
	if node.StoragePolicy != nil {
		entNode.Edges.StoragePolicy = lo.Map(node.StoragePolicy, func(item *pb.StoragePolicy, index int) *ent.StoragePolicy {
			return ProtoPolicyToEnt(item)
		})
	}

	return entNode
}

func getEntNodeType(type_ pb.Node_Type) nodeent.Type {
	switch type_ {
	case pb.Node_TYPE_MASTER:
		return nodeent.TypeMaster
	case pb.Node_TYPE_SLAVE:
		return nodeent.TypeSlave
	default:
		return nodeent.TypeMaster // default value
	}
}

func getEntNodeStatus(status pb.Node_Status) nodeent.Status {
	switch status {
	case pb.Node_STATUS_ACTIVE:
		return nodeent.StatusActive
	case pb.Node_STATUS_SUSPENDED:
		return nodeent.StatusSuspended
	default:
		return nodeent.StatusActive // default value
	}
}

func ToFsUploadRequest(req *pbslave.UploadRequest) *fs.UploadRequest {
	if req == nil {
		return nil
	}
	return &fs.UploadRequest{
		Props:  ToFsUploadProps(req.Props),
		Mode:   fs.WriteMode(req.WriteMode),
		Offset: req.Offset,
	}
}

func FromFsUploadRequest(req *fs.UploadRequest) *pbslave.UploadRequest {
	if req == nil {
		return nil
	}

	return &pbslave.UploadRequest{
		Props:     FromFsUploadProps(req.Props),
		WriteMode: int32(req.Mode),
		Offset:    req.Offset,
	}
}

func ToFsUploadSession(session *pbslave.UploadSession) *fs.UploadSession {
	if session == nil {
		return nil
	}

	return &fs.UploadSession{
		UID:            int(session.UserId),
		Policy:         ProtoPolicyToEnt(session.Policy),
		FileID:         int(session.FileId),
		EntityID:       int(session.EntityId),
		Callback:       session.Callback,
		CallbackSecret: session.CallbackSecret,
		UploadID:       session.UploadId,
		UploadURL:      session.UploadUrl,
		Credential:     session.Credential,
		ChunkSize:      session.ChunkSize,
		SentinelTaskID: int(session.SentinelTaskId),
		NewFileCreated: session.NewFileCreated,
		Importing:      session.Importing,
		LockToken:      session.LockToken,
		Props:          ToFsUploadProps(session.Props),
	}
}

func FromFsUploadSession(session *fs.UploadSession) *pbslave.UploadSession {
	if session == nil {
		return nil
	}

	return &pbslave.UploadSession{
		UserId:         int32(session.UID),
		Policy:         EntPolicyToProto(session.Policy),
		FileId:         int32(session.FileID),
		EntityId:       int32(session.EntityID),
		Callback:       session.Callback,
		CallbackSecret: session.CallbackSecret,
		UploadId:       session.UploadID,
		UploadUrl:      session.UploadURL,
		Credential:     session.Credential,
		ChunkSize:      session.ChunkSize,
		SentinelTaskId: int32(session.SentinelTaskID),
		NewFileCreated: session.NewFileCreated,
		Importing:      session.Importing,
		LockToken:      session.LockToken,
		Props:          FromFsUploadProps(session.Props),
	}
}

func ToFsUploadProps(props *pbslave.UploadProps) *fs.UploadProps {
	if props == nil {
		return nil
	}

	uploadProps := &fs.UploadProps{
		Uri:                    nil,
		Size:                   props.Size,
		UploadSessionID:        props.UploadSessionId,
		PreferredStoragePolicy: int(props.PreferredStoragePolicy),
		SavePath:               props.SavePath,
		MimeType:               props.MimeType,
		Metadata:               props.Metadata,
		PreviousVersion:        props.PreviousVersion,
		ExpireAt:               props.ExpireAt.AsTime(),
	}

	uri, err := fs.NewUriFromString(props.Uri)
	if err != nil {
		return nil
	}
	uploadProps.Uri = uri

	if props.LastModified != nil {
		lastModified := props.LastModified.AsTime()
		uploadProps.LastModified = &lastModified
	}

	if props.EntityType != nil {
		entityType := int(*props.EntityType)
		uploadProps.EntityType = &entityType
	}

	return uploadProps
}

func FromFsUploadProps(props *fs.UploadProps) *pbslave.UploadProps {
	if props == nil {
		return nil
	}

	uploadProps := &pbslave.UploadProps{
		Uri:                    props.Uri.String(),
		Size:                   props.Size,
		UploadSessionId:        props.UploadSessionID,
		PreferredStoragePolicy: int32(props.PreferredStoragePolicy),
		SavePath:               props.SavePath,
		MimeType:               props.MimeType,
		Metadata:               props.Metadata,
		PreviousVersion:        props.PreviousVersion,
		ExpireAt:               timestamppb.New(props.ExpireAt),
	}

	if props.LastModified != nil {
		uploadProps.LastModified = timestamppb.New(*props.LastModified)
	}

	if props.EntityType != nil {
		entityType := int32(*props.EntityType)
		uploadProps.EntityType = &entityType
	}

	return uploadProps
}

func ToFsPhysicalObject(obj *pbslave.PhysicalObject) *fs.PhysicalObject {
	if obj == nil {
		return nil
	}

	fsObj := &fs.PhysicalObject{
		Name:         obj.Name,
		Source:       obj.Source,
		RelativePath: obj.RelativePath,
		Size:         obj.Size,
		IsDir:        obj.IsDir,
	}
	if obj.LastModify != nil {
		fsObj.LastModify = obj.LastModify.AsTime()
	}
	return fsObj
}

func FromFsPhysicalObject(obj *fs.PhysicalObject) *pbslave.PhysicalObject {
	if obj == nil {
		return nil
	}

	return &pbslave.PhysicalObject{
		Name:         obj.Name,
		Source:       obj.Source,
		RelativePath: obj.RelativePath,
		Size:         obj.Size,
		IsDir:        obj.IsDir,
		LastModify:   timestamppb.New(obj.LastModify),
	}
}

func PaginationResultsToProto(res *data.PaginationResults) *pb.PaginationResults {
	if res == nil {
		return nil
	}
	return &pb.PaginationResults{
		Page:          int32(res.Page),
		PageSize:      int32(res.PageSize),
		TotalItems:    int32(res.TotalItems),
		NextPageToken: res.NextPageToken,
		IsCursor:      res.IsCursor,
	}
}

func PaginationArgsFromProto(args *pb.PaginationArgs) *data.PaginationArgs {
	if args == nil {
		return nil
	}
	return &data.PaginationArgs{
		UseCursorPagination: args.UseCursorPagination,
		Page:                int(args.Page),
		PageToken:           args.PageToken,
		PageSize:            int(args.PageSize),
		OrderBy:             args.OrderBy,
		Order:               OrderDirectionFromProto(args.OrderDirection),
	}
}

func OrderDirectionFromProto(orderDirection string) data.OrderDirection {
	switch orderDirection {
	case "asc":
		return data.OrderDirectionAsc
	case "desc":
		return data.OrderDirectionDesc
	default:
		return ""
	}
}
