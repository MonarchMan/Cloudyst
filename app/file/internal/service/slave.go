package service

import (
	commonpb "api/api/common/v1"
	filepb "api/api/file/common/v1"
	pb "api/api/file/slave/v1"
	pbuser "api/api/user/users/v1"
	"api/external/trans"
	"common/cache"
	"context"
	"encoding/base64"
	"file/internal/biz/credmanager"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/driver/local"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/mediameta"
	"file/internal/biz/setting"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"fmt"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SlaveService struct {
	pb.UnimplementedSlaveServer
	dep       filemanager.ManagerDep
	dbfsDep   filemanager.DbfsDep
	credm     credmanager.CredManager
	uc        pbuser.UserClient
	kv        cache.Driver
	settings  setting.Provider
	extractor mediameta.ExtractorStateManager
	l         *log.Helper
}

func NewSlaveService(dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep, credm credmanager.CredManager,
	uc pbuser.UserClient, kv cache.Driver, settings setting.Provider, extractor mediameta.ExtractorStateManager,
	l log.Logger) *SlaveService {
	return &SlaveService{
		dep:       dep,
		dbfsDep:   dbfsDep,
		credm:     credm,
		uc:        uc,
		kv:        kv,
		settings:  settings,
		extractor: extractor,
		l:         log.NewHelper(l, log.WithMessageKey("service-slave")),
	}
}

func (s *SlaveService) GetCredential(ctx context.Context, req *pb.SimpleSlaveRequest) (*pb.GetCredentialResponse, error) {
	cred, err := s.credm.Obtain(ctx, req.Id)
	if cred == nil || err != nil {
		return nil, commonpb.ErrorNotFound("Credential not found: %w", err)
	}

	return &pb.GetCredentialResponse{
		Token:    cred.String(),
		ExpireAt: timestamppb.New(cred.Expiry()),
	}, nil
}
func (s *SlaveService) StatelessPrepareUpload(ctx context.Context, req *pb.StatelessPrepareUploadRequest) (*pb.StatelessPrepareUploadResponse, error) {
	userClient := s.uc
	user, err := rpc.GetUserInfo(ctx, int(req.UserId), userClient)
	if err != nil {
		return nil, err
	}

	newCtx := context.WithValue(ctx, trans.UserCtx{}, user)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	uploadRequest := utils.ToFsUploadRequest(req.UploadRequest)
	uploadSession, err := m.PrepareUpload(newCtx, uploadRequest)
	if err != nil {
		return nil, err
	}

	return &pb.StatelessPrepareUploadResponse{
		UploadSession: utils.FromFsUploadSession(uploadSession),
		UploadRequest: req.UploadRequest,
	}, nil
}
func (s *SlaveService) StatelessCompleteUpload(ctx context.Context, req *pb.StatelessCompleteUploadRequest) (*pb.StatelessCompleteUploadResponse, error) {
	userClient := s.uc
	user, err := rpc.GetUserInfo(ctx, int(req.UserId), userClient)
	if err != nil {
		return nil, err
	}

	newCtx := context.WithValue(ctx, trans.UserCtx{}, user)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	_, err = m.CompleteUpload(newCtx, utils.ToFsUploadSession(req.UploadSession))
	//fileBytes, _ := json.Marshal(file)
	return &pb.StatelessCompleteUploadResponse{}, err
}
func (s *SlaveService) StatelessFailUpload(ctx context.Context, req *pb.StatelessCompleteUploadRequest) (*emptypb.Empty, error) {
	userClient := s.uc
	user, err := rpc.GetUserInfo(ctx, int(req.UserId), userClient)
	if err != nil {
		return nil, err
	}
	newCtx := context.WithValue(ctx, trans.UserCtx{}, user)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	m.OnUploadFailed(newCtx, utils.ToFsUploadSession(req.UploadSession))
	return &emptypb.Empty{}, nil
}
func (s *SlaveService) StatelessCreateFile(ctx context.Context, req *pb.StatelessCreateFileRequest) (*emptypb.Empty, error) {
	userClient := s.uc
	user, err := rpc.GetUserInfo(ctx, int(req.UserId), userClient)
	if err != nil {
		return nil, err
	}

	uri, err := fs.NewUriFromString(req.Path)
	if err != nil {
		return nil, err
	}

	newCtx := context.WithValue(ctx, trans.UserCtx{}, user)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	_, err = m.Create(newCtx, uri, int(req.Type))
	return &emptypb.Empty{}, commonpb.ErrorDb("Failed to create file: %w", err)
}

func (s *SlaveService) Upload(ctx context.Context, req *pb.SlaveUploadRequest) (*emptypb.Empty, error) {
	uploadSessionRaw, ok := s.kv.Get(manager.UploadSessionCachePrefix + req.SessionId)
	if !ok {
		return nil, filepb.ErrorUploadSessionExpired("upload session expired")
	}
	uploadSession, ok := uploadSessionRaw.(fs.UploadSession)

	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	defer m.Recycle()

	return processChunkUpload(ctx, m, &uploadSession, req.Index, nil, fs.ModeOverwrite)
}
func (s *SlaveService) CreateUploadSession(ctx context.Context, req *pb.CreateUploadSessionRequest) (*emptypb.Empty, error) {
	mode := fs.ModeNone
	if req.Overwrite {
		mode = fs.ModeOverwrite
	}

	newReq := &fs.UploadRequest{
		Mode:  mode,
		Props: utils.ToFsUploadProps(req.Session.Props),
	}

	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	_, err := m.CreateUploadSession(ctx, newReq, fs.WithUploadSession(utils.ToFsUploadSession(req.Session)))
	if err != nil {
		return nil, commonpb.ErrorCacheOperation("Failed to create upload session in slave node", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *SlaveService) DeleteUploadSession(ctx context.Context, req *pb.DeleteUploadSessionRequest) (*emptypb.Empty, error) {
	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	defer m.Recycle()

	err := m.CancelUploadSession(ctx, nil, req.SessionId)
	if err != nil {
		return nil, fmt.Errorf("slave failed to delete upload session: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *SlaveService) GetMetadata(ctx context.Context, req *pb.GetMetadataRequest) (*pb.GetMetadataResponse, error) {
	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	defer m.Recycle()

	src, err := base64.URLEncoding.DecodeString(req.Src)
	if err != nil {
		return nil, fmt.Errorf("failed to base64 decode src: %w", err)
	}

	entity, err := local.NewLocalFileEntity(types.EntityTypeVersion, string(src))
	if err != nil {
		return nil, filepb.ErrorParentNotExist("Path not exist: %s", err)
	}

	entitySource, err := m.GetEntitySource(ctx, 0, fs.WithEntity(entity))
	if err != nil {
		return nil, fmt.Errorf("failed to get entity source: %w", err)
	}
	defer entitySource.Close()

	res, err := s.extractor.GetMediaMetaExtractor().Extract(ctx, req.Ext, entitySource, mediameta.WithLanguage(req.Language))
	if err != nil {
		return nil, fmt.Errorf("failed to extract media metadata: %w", err)
	}

	meta := make([]*pb.MediaMeta, len(res))
	for i := range res {
		// 直接取地址，避免值传递
		meta[i] = &res[i]
	}

	return &pb.GetMetadataResponse{
		Meta: meta,
	}, err
}
func (s *SlaveService) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	defer m.Recycle()

	d := m.LocalDriver(nil)
	// try to delete thumbnail sidecar
	sidecarSuffix := s.settings.ThumbSlaveSidecarSuffix(ctx)
	failed, err := d.Delete(ctx, lo.Map(req.Files, func(item string, index int) string {
		return item + sidecarSuffix
	})...)
	if err != nil {
		s.l.WithContext(ctx).Warnf("Failed to delete thumbnail sidecar [%s]: %s", strings.Join(failed, ", "), err)
	}

	failed, err = d.Delete(ctx, req.Files...)
	if err != nil {
		return &pb.DeleteResponse{Failed: failed}, err
	}
	return nil, nil
}
func (s *SlaveService) Ping(ctx context.Context, req *pb.PingRequest) (*emptypb.Empty, error) {
	//master, err := url.Parse(req.Callback)
	//if err != nil {
	//	return nil, commonpb.ErrorParamInvalid("Failed to parse callback url: %s", err)
	//}
	//
	//version := constants.BackendVersion
	return nil, nil
}
func (s *SlaveService) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	defer m.Recycle()

	d := m.LocalDriver(nil)
	objects, err := d.List(ctx, req.Path, func(i int) {}, req.Recursive)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	files := make([]*pb.PhysicalObject, len(objects))
	for i, obj := range objects {
		files[i] = utils.FromFsPhysicalObject(&obj)
	}

	return &pb.ListFilesResponse{
		Files: files,
	}, nil
}

func (s *SlaveService) ServeEntity(ctx khttp.Context) error {
	req := ctx.Request()
	resp := ctx.Response()
	var reqBody pb.EntityRequest
	if err := ctx.BindVars(&reqBody); err != nil {
		return fmt.Errorf("failed to bind request body: %w", err)
	}

	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	defer m.Recycle()

	src, err := base64.URLEncoding.DecodeString(reqBody.Src)
	if err != nil {
		return fmt.Errorf("failed to decode src: %w", err)
	}

	entity, err := local.NewLocalFileEntity(types.EntityTypeVersion, string(src))
	if err != nil {
		return filepb.ErrorParentNotExist("Path not exist: %s", err)
	}

	entitySource, err := m.GetEntitySource(ctx, 0, fs.WithEntity(entity))
	if err != nil {
		return fmt.Errorf("failed to get entity source: %w", err)
	}
	defer entitySource.Close()

	maxAge := s.settings.PublicResourceMaxAge(ctx)
	req.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
	entitySource.Serve(resp, req,
		entitysource.WithSpeedLimit(reqBody.Speed),
		entitysource.WithDownload(reqBody.IsDownload),
		entitysource.WithDisplayName(reqBody.Name),
		entitysource.WithContext(ctx),
	)
	return nil
}

func (s *SlaveService) Thumb(ctx khttp.Context) error {
	req := ctx.Request()
	resp := ctx.Response()
	var reqBody pb.GetThumbRequest
	m := manager.NewFileManager(s.dep, s.dbfsDep, nil)
	defer m.Recycle()

	src, err := base64.URLEncoding.DecodeString(reqBody.Src)
	if err != nil {
		return fmt.Errorf("failed to decode src: %w", err)
	}
	entity, err := local.NewLocalFileEntity(types.EntityTypeThumbnail, string(src)+s.settings.ThumbSlaveSidecarSuffix(ctx))
	if err != nil {
		srcEntity, err := local.NewLocalFileEntity(types.EntityTypeVersion, string(src))
		if err != nil {
			return filepb.ErrorParentNotExist("Path not exist: %s", err)
		}

		entity, err = m.SubmitAndAwaitThumbnailTask(ctx, nil, reqBody.Ext, srcEntity)
		if err != nil {
			return fmt.Errorf("failed to submit and await thumbnail task: %w", err)
		}
	}

	entitySource, err := m.GetEntitySource(ctx, 0, fs.WithEntity(entity))
	if err != nil {
		return fmt.Errorf("failed to get entity source: %w", err)
	}
	defer entitySource.Close()

	maxAge := s.settings.PublicResourceMaxAge(ctx)
	req.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
	entitySource.Serve(resp, req,
		entitysource.WithContext(ctx),
	)
	return nil
}
