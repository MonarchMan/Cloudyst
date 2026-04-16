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
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/data"
	"file/internal/pkg/utils"
	"path"
	"strconv"
	"time"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	entityUserCondition   = "entity_user"
	entityPolicyCondition = "entity_policy"
	entityTypeCondition   = "entity_type"
)

func (s *AdminService) ListEntities(ctx context.Context, req *filepb.ListRequest) (*pb.ListEntitiesResponse, error) {
	fileClient := s.fc
	newCtx := context.WithValue(ctx, data.LoadEntityStoragePolicy{}, true)

	var (
		userID     int
		policyID   int
		err        error
		entityType *int
	)

	if req.Conditions[entityPolicyCondition] != "" {
		policyID, err = strconv.Atoi(req.Conditions[entityPolicyCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid policy ID: %w", err)
		}
	}

	if req.Conditions[entityTypeCondition] != "" {
		typeId, err := strconv.Atoi(req.Conditions[entityTypeCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid entity type: %w", err)
		}

		entityType = &typeId
	}

	res, err := fileClient.ListEntities(newCtx, &data.ListEntityParameters{
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
		EntityType:      entityType,
		UserID:          userID,
		StoragePolicyID: policyID,
	})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list entities: %w", err)
	}

	return &pb.ListEntitiesResponse{
		Pagination: res.PaginationResults,
		Entities: lo.Map(res.Entities, func(item *ent.Entity, index int) *pb.GetEntityResponse {
			return &pb.GetEntityResponse{
				Entity:     utils.EntEntityToProto(item),
				UserHashId: hashid.EncodeUserID(s.hasher, item.CreatedBy),
			}
		}),
	}, nil
}
func (s *AdminService) GetEntity(ctx context.Context, req *pb.SimpleEntityRequest) (*pb.GetEntityResponse, error) {
	fileClient := s.fc
	hasher := s.hasher

	newCtx := context.WithValue(ctx, data.LoadEntityStoragePolicy{}, true)
	newCtx = context.WithValue(newCtx, data.LoadEntityFile{}, true)

	userHashIDMap := make(map[int32]string)
	entity, err := fileClient.GetEntityByID(newCtx, int(req.Id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, commonpb.ErrorNotFound("Entity not found: %w", err)
		}
		return nil, commonpb.ErrorDb("Failed to get entity: %w", err)
	}

	for _, file := range entity.Edges.File {
		userHashIDMap[int32(file.OwnerID)] = hashid.EncodeUserID(hasher, file.OwnerID)
	}

	return &pb.GetEntityResponse{
		Entity:        utils.EntEntityToProto(entity),
		UserHashId:    hashid.EncodeUserID(hasher, entity.CreatedBy),
		UserHashIdMap: userHashIDMap,
	}, nil
}
func (s *AdminService) BatchDeleteEntities(ctx context.Context, req *pb.BatchEntitiesRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	ids := lo.Map(req.Ids, func(item int32, index int) int {
		return int(item)
	})
	if err := m.RecycleEntities(ctx, req.Force, ids...); err != nil {
		return nil, commonpb.ErrorDb("Failed to recycle entities: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetEntityUrl(ctx context.Context, req *pb.SimpleEntityRequest) (*pb.GetUrlResponse, error) {
	user := trans.FromContext(ctx)
	fileClient := s.fc
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	entity, err := fileClient.GetEntityByID(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get entity: %w", err)
	}

	policy, err := s.pc.GetPolicyByID(ctx, entity.StoragePolicyEntities)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get policy: %w", err)
	}

	driver, err := m.GetStorageDriver(ctx, policy)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get storage driver: %w", err)
	}

	es := entitysource.NewEntitySource(fs.NewEntity(entity), driver, policy, s.dep.GeneralAuth(), s.dep.SettingProvider(),
		s.hasher, request.NewClient(s.conf.Server.Sys.Mode), s.dep.Logger(), s.conf, s.mm.MimeDetector(), s.encryptorFactory)

	expire := time.Now().Add(time.Hour * 1)
	url, err := es.Url(ctx, entitysource.WithDownload(true), entitysource.WithExpire(&expire),
		entitysource.WithDisplayName(path.Base(entity.Source)))
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to get url: %w", err)
	}

	return &pb.GetUrlResponse{
		Url: url.Url,
	}, nil
}
