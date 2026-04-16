package admin

import (
	commonpb "api/api/common/v1"
	pb "api/api/file/admin/v1"
	filepb "api/api/file/common/v1"
	"api/external/trans"
	"common/hashid"
	"common/request"
	"context"
	"file/ent"
	"file/internal/biz/cluster/routes"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"file/internal/data"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"strconv"
	"time"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	fileNameCondition       = "file_name"
	fileUserCondition       = "file_user"
	filePolicyCondition     = "file_policy"
	fileMetadataCondition   = "file_metadata"
	fileSharedCondition     = "file_shared"
	fileDirectLinkCondition = "file_direct_link"
)

func (s *AdminService) ListFiles(ctx context.Context, req *filepb.ListRequest) (*pb.ListFileResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadFileEntity{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileMetadata{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileShare{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileDirectLink{}, true)

	var (
		err        error
		userID     int
		policyID   int
		metadata   string
		shared     bool
		directLink bool
	)

	if req.Conditions[fileUserCondition] != "" {
		userID, err = strconv.Atoi(req.Conditions[fileUserCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid users ID: %w", err)
		}
	}

	if req.Conditions[filePolicyCondition] != "" {
		policyID, err = strconv.Atoi(req.Conditions[filePolicyCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid policy ID: %w", err)
		}
	}

	if req.Conditions[fileMetadataCondition] != "" {
		metadata = req.Conditions[fileMetadataCondition]
	}

	if req.Conditions[fileSharedCondition] != "" && setting.IsTrueValue(req.Conditions[fileSharedCondition]) {
		shared = true
	}

	if req.Conditions[fileDirectLinkCondition] != "" && setting.IsTrueValue(req.Conditions[fileDirectLinkCondition]) {
		directLink = true
	}

	res, err := s.fc.FlattenListFiles(ctx, &data.FlattenListFileParameters{
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
		UserID:          userID,
		StoragePolicyID: policyID,
		Name:            req.Conditions[fileNameCondition],
		HasMetadata:     metadata,
		Shared:          shared,
		HasDirectLink:   directLink,
	})

	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list files: %w", err)
	}

	return &pb.ListFileResponse{
		Pagination: res.PaginationResults,
		Files: lo.Map(res.Files, func(file *ent.File, _ int) *pb.GetFileResponse {
			return &pb.GetFileResponse{
				File:       utils.EntFileToProto(file),
				UserHashId: hashid.EncodeUserID(s.hasher, file.OwnerID),
				FileHashId: hashid.EncodeFileID(s.hasher, file.ID),
			}
		}),
	}, nil
}
func (s *AdminService) GetFile(ctx context.Context, req *pb.SimpleFileRequest) (*pb.GetFileResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadFileEntity{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileMetadata{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileShare{}, true)
	newCtx = context.WithValue(newCtx, data.LoadEntityStoragePolicy{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileDirectLink{}, true)

	file, err := s.fc.GetByID(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get file: %w", err)
	}

	directLinkMap := make(map[int32]string)
	siteURL := s.settings.SiteURL(ctx)
	for _, directLink := range file.Edges.DirectLinks {
		directLinkMap[int32(directLink.ID)] = routes.MasterDirectLink(siteURL, hashid.EncodeSourceLinkID(s.hasher, directLink.ID), directLink.Name).String()
	}

	return &pb.GetFileResponse{
		File:          utils.EntFileToProto(file),
		UserHashId:    hashid.EncodeUserID(s.hasher, file.OwnerID),
		FileHashId:    hashid.EncodeFileID(s.hasher, file.ID),
		DirectLinkMap: directLinkMap,
	}, nil
}
func (s *AdminService) UpdateFile(ctx context.Context, req *pb.UpsertFileRequest) (*pb.GetFileResponse, error) {
	fc, tx, ctx, err := data.WithTx(ctx, s.fc)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to start transaction: %w", err)
	}

	newFile, err := fc.Update(ctx, utils.ProtoFileToEnt(req.File))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update files: %w", err)
	}

	if err := data.Commit(tx); err != nil {
		_ = data.Rollback(tx)
		return nil, commonpb.ErrorDb("Failed to commit transaction: %w", err)
	}

	fileRequest := pb.SimpleFileRequest{Id: int32(newFile.ID)}

	return s.GetFile(ctx, &fileRequest)
}
func (s *AdminService) GetFileUrl(ctx context.Context, req *pb.SimpleFileRequest) (*pb.GetFileUrlResponse, error) {
	user := trans.FromContext(ctx)

	newCtx := context.WithValue(ctx, data.LoadFileEntity{}, true)
	file, err := s.fc.GetByID(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get file: %w", err)
	}

	// 找出主实体
	var primaryEntity *ent.Entity
	for _, entity := range file.Edges.Entities {
		if entity.Type == int(types.EntityTypeVersion) && entity.ID == file.PrimaryEntity {
			primaryEntity = entity
			break
		}
	}

	if primaryEntity == nil {
		return nil, commonpb.ErrorNotFound("Primary entity not exist", "Primary entity not exist")
	}

	// 找出策略
	policy, err := s.pc.GetPolicyByID(newCtx, primaryEntity.StoragePolicyEntities)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get policy: %w", err)
	}

	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	driver, err := m.GetStorageDriver(newCtx, policy)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to get storage driver: %w", err)
	}

	es := entitysource.NewEntitySource(fs.NewEntity(primaryEntity), driver, policy, s.auth, s.settings,
		s.hasher, request.NewClient(s.conf.Server.Sys.Mode), s.l, s.conf, s.mm.MimeDetector(), s.encryptorFactory)

	expire := time.Now().Add(time.Hour * 1)
	url, err := es.Url(newCtx, entitysource.WithExpire(&expire), entitysource.WithDisplayName(file.Name))
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to get url: %w", err)
	}

	return &pb.GetFileUrlResponse{
		Url: url.Url,
	}, nil
}
func (s *AdminService) BatchDeleteFiles(ctx context.Context, req *pb.BatchFileRequest) (*emptypb.Empty, error) {
	fileClient := s.fc

	newCtx := context.WithValue(ctx, data.LoadFileShare{}, true)
	ids := lo.Map(req.Ids, func(id int32, _ int) int {
		return int(id)
	})
	files, _, err := fileClient.GetByIDs(newCtx, ids, 0)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete files: %w", err)
	}

	fc, tx, ctx, err := data.WithTx(ctx, fileClient)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create transaction: %w", err)
	}

	_, diff, err := fc.Delete(ctx, files, nil)
	if err != nil {
		_ = data.Rollback(tx)
		return nil, commonpb.ErrorDb("Failed to delete files: %w", err)
	}

	tx.AppendStorageDiff(diff)
	if err := data.CommitWithStorageDiff(ctx, tx, s.l, s.uc); err != nil {
		return nil, commonpb.ErrorDb("Failed to commit transaction: %w", err)
	}

	return &emptypb.Empty{}, nil
}
