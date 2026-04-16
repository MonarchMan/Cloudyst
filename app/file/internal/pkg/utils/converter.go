package utils

import (
	pb "api/api/file/common/v1"
	pbslave "api/api/file/slave/v1"
	"common/boolset"
	"file/ent"
	nodeent "file/ent/node"
	taskent "file/ent/task"
	"file/internal/biz/filemanager/fs"
	"file/internal/data/types"

	"github.com/gofrs/uuid"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func EntFileToProto(file *ent.File) *pb.File {
	if file == nil {
		return nil
	}
	protoFile := &pb.File{
		Id:        int64(file.ID),
		CreatedAt: timestamppb.New(file.CreatedAt),
		UpdatedAt: timestamppb.New(file.UpdatedAt),
		Type:      int64(file.Type),
		Name:      file.Name,
		OwnerId:   int64(file.OwnerID),
		OwnerInfo: file.OwnerInfo,
		Size:      file.Size,
		PrimaryEntity: &wrapperspb.Int64Value{
			Value: int64(file.PrimaryEntity),
		},
		FileParentId: &wrapperspb.Int64Value{
			Value: int64(file.FileParentID),
		},
		IsSymbolic: file.IsSymbolic,
		Props: &pb.FileProps{
			View: ToProtoView(file.Props.View),
		},
		StoragePolicyFiles: &wrapperspb.Int64Value{
			Value: int64(file.StoragePolicyFiles),
		},
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
		OwnerInfo:  protoFile.OwnerInfo,
		Size:       protoFile.Size,
		IsSymbolic: protoFile.IsSymbolic,
		Props: &types.FileProps{
			View: FromProtoView(protoFile.Props.View),
		},
	}

	if protoFile.PrimaryEntity != nil {
		entFile.PrimaryEntity = int(protoFile.PrimaryEntity.Value)
	}

	if protoFile.FileParentId != nil {
		entFile.FileParentID = int(protoFile.FileParentId.Value)
	}

	if protoFile.StoragePolicyFiles != nil {
		entFile.StoragePolicyFiles = int(protoFile.StoragePolicyFiles.Value)
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

func FromProtoView(protoView *pb.ExplorerView) *types.ExplorerView {
	if protoView == nil {
		return nil
	}
	return &types.ExplorerView{
		PageSize:       int(protoView.PageSize),
		Order:          protoView.Order,
		OrderDirection: FromProtoOrderDirection(protoView.OrderDirection),
		View:           fromProtoViewType(protoView.View),
		Thumbnail:      protoView.Thumbnail,
		GalleryWidth:   int(protoView.GalleryWidth),
		Columns: lo.Map(protoView.Columns, func(item *pb.ListViewColumn, index int) types.ListViewColumn {
			width := int(item.Width)
			return types.ListViewColumn{
				Type:  int(item.Type),
				Width: &width,
			}
		}),
	}
}

func fromProtoViewType(viewType pb.ViewType) string {
	switch viewType {
	case pb.ViewType_VIEW_TYPE_LIST:
		return "list"
	case pb.ViewType_VIEW_TYPE_GRID:
		return "grid"
	case pb.ViewType_VIEW_TYPE_GALLERY:
		return "gallery"
	default:
		return ""
	}
}

func FromProtoOrderDirection(direction pb.OrderDirection) string {
	switch direction {
	case pb.OrderDirection_ORDER_DIRECTION_ASC:
		return "ASC"
	case pb.OrderDirection_ORDER_DIRECTION_DESC:
		return "DESC"
	default:
		return ""
	}
}

func ToProtoView(view *types.ExplorerView) *pb.ExplorerView {
	if view == nil {
		return nil
	}
	return &pb.ExplorerView{
		PageSize:       int32(view.PageSize),
		Order:          view.Order,
		OrderDirection: toProtoOrderDirection(view.OrderDirection),
		View:           toProtoViewType(view.View),
		Thumbnail:      view.Thumbnail,
		GalleryWidth:   int32(view.GalleryWidth),
		Columns: lo.Map(view.Columns, func(item types.ListViewColumn, index int) *pb.ListViewColumn {
			column := &pb.ListViewColumn{
				Type: int32(item.Type),
			}
			if item.Width != nil {
				column.Width = int32(*item.Width)
			}
			return column
		}),
	}
}

func toProtoViewType(view string) pb.ViewType {
	switch view {
	case "list":
		return pb.ViewType_VIEW_TYPE_LIST
	case "grid":
		return pb.ViewType_VIEW_TYPE_GRID
	case "gallery":
		return pb.ViewType_VIEW_TYPE_GALLERY
	default:
		return pb.ViewType_VIEW_TYPE_UNSPECIFIED
	}
}

func toProtoOrderDirection(direction string) pb.OrderDirection {
	switch direction {
	case "ASC":
		return pb.OrderDirection_ORDER_DIRECTION_ASC
	case "DESC":
		return pb.OrderDirection_ORDER_DIRECTION_DESC
	default:
		return pb.OrderDirection_ORDER_DIRECTION_UNSPECIFIED
	}
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
		Password: &wrapperspb.StringValue{
			Value: share.Password,
		},
		Views:     int64(share.Views),
		Downloads: int64(share.Downloads),
		Props:     share.Props,
		OwnerId:   int64(share.OwnerID),
		OwnerInfo: share.OwnerInfo,
	}

	if share.RemainDownloads != nil {
		protoShare.RemainDownloads = &wrapperspb.Int64Value{
			Value: int64(*share.RemainDownloads),
		}
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
		CreatedAt: share.GetCreatedAt().AsTime(),
		UpdatedAt: share.GetUpdatedAt().AsTime(),
		Password:  share.GetPassword().GetValue(),
		Views:     int(share.GetViews()),
		Downloads: int(share.GetDownloads()),
		Props:     share.GetProps(),
		OwnerID:   int(share.GetOwnerId()),
		OwnerInfo: share.GetOwnerInfo(),
	}

	// Handle Expires field
	if share.GetExpires() != nil {
		expires := share.GetExpires().AsTime()
		entShare.Expires = &expires
	}

	// Handle RemainDownloads field
	if share.GetRemainDownloads() != nil {
		remainDownloads := int(share.GetRemainDownloads().GetValue())
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
		Id:        int64(storagePolicy.ID),
		CreatedAt: timestamppb.New(storagePolicy.CreatedAt),
		UpdatedAt: timestamppb.New(storagePolicy.UpdatedAt),
		Name:      storagePolicy.Name,
		Type:      storagePolicy.Type,
		Server: &wrapperspb.StringValue{
			Value: storagePolicy.Server,
		},
		BucketName: &wrapperspb.StringValue{
			Value: storagePolicy.BucketName,
		},
		IsPrivate: &wrapperspb.BoolValue{
			Value: storagePolicy.IsPrivate,
		},
		AccessKey: &wrapperspb.StringValue{
			Value: storagePolicy.AccessKey,
		},
		SecretKey: &wrapperspb.StringValue{
			Value: storagePolicy.SecretKey,
		},
		MaxSize: &wrapperspb.Int64Value{
			Value: storagePolicy.MaxSize,
		},
		DirNameRule: &wrapperspb.StringValue{
			Value: storagePolicy.DirNameRule,
		},
		FileNameRule: &wrapperspb.StringValue{
			Value: storagePolicy.FileNameRule,
		},
		Settings: storagePolicy.Settings,
		NodeId: &wrapperspb.Int64Value{
			Value: int64(storagePolicy.NodeID),
		},
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

func EntNodeToProto(node *ent.Node) *pb.Node {
	if node == nil {
		return nil
	}
	protoNode := &pb.Node{
		Id:        int64(node.ID),
		CreatedAt: timestamppb.New(node.CreatedAt),
		UpdatedAt: timestamppb.New(node.UpdatedAt),
		Status:    getNodeStatus(node),
		Name:      node.Name,
		Type:      getNodeType(node),
		Server: &wrapperspb.StringValue{
			Value: node.Server,
		},
		SlaveKey: &wrapperspb.StringValue{
			Value: node.SlaveKey,
		},
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
		CreatedBy: &wrapperspb.Int64Value{
			Value: int64(entity.CreatedBy),
		},
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
		protoEntity.UploadSessionId = &wrapperspb.BytesValue{
			Value: entity.UploadSessionID.Bytes(),
		}
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
		Value: &wrapperspb.StringValue{
			Value: setting.Value,
		},
	}

	if setting.DeletedAt != nil {
		protoSetting.DeletedAt = timestamppb.New(*setting.DeletedAt)
	}

	return protoSetting
}

func EntTaskToProto(task *ent.Task) *pb.Task {
	protoTask := &pb.Task{
		Id:          int64(task.ID),
		CreatedAt:   timestamppb.New(task.CreatedAt),
		UpdatedAt:   timestamppb.New(task.UpdatedAt),
		DeletedAt:   nil,
		Type:        task.Type,
		Status:      getTaskStatus(task),
		PublicState: EntTaskStateToProto(task.PublicState),
		PrivateState: &wrapperspb.StringValue{
			Value: task.PrivateState,
		},
		TraceId: task.TraceID,
		UserId: &wrapperspb.Int64Value{
			Value: int64(task.UserID),
		},
	}

	return protoTask
}

func getTaskStatus(task *ent.Task) pb.Task_Status {
	switch task.Status {
	case taskent.StatusSuspending:
		return pb.Task_STATUS_SUSPENDING
	case taskent.StatusProcessing:
		return pb.Task_STATUS_PROCESSING
	case taskent.StatusCompleted:
		return pb.Task_STATUS_COMPLETED
	case taskent.StatusCanceled:
		return pb.Task_STATUS_CANCELED
	case taskent.StatusError:
		return pb.Task_STATUS_ERROR
	default:
		return pb.Task_STATUS_QUEUED
	}
}

func EntTaskStateToProto(publicState *pb.TaskPublicState) *pb.TaskPublicState {
	if publicState == nil {
		return nil
	}
	return &pb.TaskPublicState{
		Error:            publicState.Error,
		ErrorHistory:     publicState.ErrorHistory,
		ExecutedDuration: publicState.ExecutedDuration,
		RetryCount:       publicState.RetryCount,
		ResumeTime:       publicState.ResumeTime,
		SlaveTaskProps:   publicState.SlaveTaskProps,
	}
}

func ProtoPolicyToEnt(policy *pb.StoragePolicy) *ent.StoragePolicy {
	// Handle nil input
	if policy == nil {
		return nil
	}
	entPolicy := &ent.StoragePolicy{
		ID:        int(policy.GetId()),
		CreatedAt: policy.GetCreatedAt().AsTime(),
		UpdatedAt: policy.GetUpdatedAt().AsTime(),
		Name:      policy.GetName(),
		Type:      policy.GetType(),
	}

	// Handle optional fields
	if policy.GetServer() != nil {
		entPolicy.Server = policy.GetServer().GetValue()
	}

	if policy.GetBucketName() != nil {
		entPolicy.BucketName = policy.GetBucketName().GetValue()
	}

	if policy.GetIsPrivate() != nil {
		entPolicy.IsPrivate = policy.GetIsPrivate().GetValue()
	}

	if policy.GetAccessKey() != nil {
		entPolicy.AccessKey = policy.GetAccessKey().GetValue()
	}

	if policy.GetSecretKey() != nil {
		entPolicy.SecretKey = policy.GetSecretKey().GetValue()
	}

	if policy.GetMaxSize() != nil {
		entPolicy.MaxSize = policy.GetMaxSize().GetValue()
	}

	if policy.GetDirNameRule() != nil {
		entPolicy.DirNameRule = policy.GetDirNameRule().GetValue()
	}

	if policy.GetFileNameRule() != nil {
		entPolicy.FileNameRule = policy.GetFileNameRule().GetValue()
	}

	entPolicy.Settings = policy.GetSettings()

	if policy.GetNodeId() != nil {
		entPolicy.NodeID = int(policy.GetNodeId().GetValue())
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
		CreatedAt: metadata.GetCreatedAt().AsTime(),
		UpdatedAt: metadata.GetUpdatedAt().AsTime(),
		Name:      metadata.GetName(),
		Value:     metadata.GetValue(),
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
		ID:                    int(entity.GetId()),
		CreatedAt:             entity.GetCreatedAt().AsTime(),
		UpdatedAt:             entity.GetUpdatedAt().AsTime(),
		Type:                  int(entity.GetType()),
		Source:                entity.GetSource(),
		Size:                  entity.GetSize(),
		ReferenceCount:        int(entity.GetReferenceCount()),
		StoragePolicyEntities: int(entity.GetStoragePolicyEntities()),
	}

	// Handle optional fields
	if entity.GetCreatedBy() != nil {
		entEntity.CreatedBy = int(entity.GetCreatedBy().GetValue())
	}

	if entity.GetUploadSessionId() != nil {
		uploadSessionID, _ := uuid.FromBytes(entity.GetUploadSessionId().GetValue())
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
		CreatedAt: directLink.GetCreatedAt().AsTime(),
		UpdatedAt: directLink.GetUpdatedAt().AsTime(),
		Name:      directLink.GetName(),
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
		CreatedAt: setting.GetCreatedAt().AsTime(),
		UpdatedAt: setting.GetUpdatedAt().AsTime(),
		Name:      setting.GetName(),
	}

	// Handle optional fields
	if setting.GetValue() != nil {
		entSetting.Value = setting.GetValue().GetValue()
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
		ID:        int(task.GetId()),
		CreatedAt: task.GetCreatedAt().AsTime(),
		UpdatedAt: task.GetUpdatedAt().AsTime(),
		Type:      task.GetType(),
		Status:    getEntTaskStatus(task.GetStatus()),
		TraceID:   task.GetTraceId(),
	}

	// Handle optional fields
	if task.GetPublicState() != nil {
		entTask.PublicState = &pb.TaskPublicState{
			Error:            task.GetPublicState().GetError(),
			ErrorHistory:     task.GetPublicState().GetErrorHistory(),
			ExecutedDuration: task.GetPublicState().GetExecutedDuration(),
			RetryCount:       task.GetPublicState().GetRetryCount(),
			ResumeTime:       task.GetPublicState().GetResumeTime(),
			SlaveTaskProps:   task.GetPublicState().GetSlaveTaskProps(),
		}
	}

	if task.GetPrivateState() != nil {
		entTask.PrivateState = task.GetPrivateState().GetValue()
	}

	if task.GetUserId() != nil {
		entTask.UserID = int(task.GetUserId().GetValue())
	}

	// Handle DeletedAt field
	if task.GetDeletedAt() != nil {
		deletedAt := task.GetDeletedAt().AsTime()
		entTask.DeletedAt = &deletedAt
	}

	return entTask
}

func getEntTaskStatus(status pb.Task_Status) taskent.Status {
	switch status {
	case pb.Task_STATUS_SUSPENDING:
		return taskent.StatusSuspending
	case pb.Task_STATUS_PROCESSING:
		return taskent.StatusProcessing
	case pb.Task_STATUS_COMPLETED:
		return taskent.StatusCompleted
	case pb.Task_STATUS_CANCELED:
		return taskent.StatusCanceled
	case pb.Task_STATUS_ERROR:
		return taskent.StatusError
	default:
		return taskent.StatusQueued
	}
}

func ProtoNodeToEnt(node *pb.Node) *ent.Node {
	// Handle nil input
	if node == nil {
		return nil
	}
	entNode := &ent.Node{
		ID:        int(node.GetId()),
		CreatedAt: node.GetCreatedAt().AsTime(),
		UpdatedAt: node.GetUpdatedAt().AsTime(),
		Name:      node.GetName(),
		Server:    node.GetServer().GetValue(),
		SlaveKey:  node.GetSlaveKey().GetValue(),
		Settings:  node.GetSettings(),
		Weight:    int(node.GetWeight()),
		Type:      getEntNodeType(node.GetType()),
		Status:    getEntNodeStatus(node.GetStatus()),
	}

	// Handle optional fields
	if node.GetCapabilities() != nil {
		capabilities := boolset.BooleanSet(node.GetCapabilities())
		entNode.Capabilities = &capabilities
	}

	// Handle DeletedAt field
	if node.GetDeletedAt() != nil {
		deletedAt := node.GetDeletedAt().AsTime()
		entNode.DeletedAt = &deletedAt
	}

	// Handle Edges
	entNode.Edges = ent.NodeEdges{}
	if node.GetStoragePolicy() != nil {
		entNode.Edges.StoragePolicy = lo.Map(node.GetStoragePolicy(), func(item *pb.StoragePolicy, index int) *ent.StoragePolicy {
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
