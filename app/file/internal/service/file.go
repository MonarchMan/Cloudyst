package service

import (
	commonpb "api/api/common/v1"
	filepb "api/api/file/common/v1"
	pbfile "api/api/file/files/v1"
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	ftypes "api/external/data/file"
	"api/external/trans"
	"common/auth"
	"common/auth/requestinfo"
	"common/cache"
	"common/hashid"
	"common/request"
	"common/util"
	"context"
	"file/internal/biz/cluster"
	"file/internal/biz/cluster/routes"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/eventhub"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"file/internal/biz/thumb"
	"file/internal/conf"
	"file/internal/data"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"file/internal/pkg/wopi"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gofrs/uuid"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FileService struct {
	pbfile.UnimplementedFileServer
	userClient       pbuser.UserClient
	directLinkClient data.DirectLinkClient
	fileClient       data.FileClient
	policyClient     data.StoragePolicyClient

	managerDep filemanager.ManagerDep
	dbfsDep    filemanager.DbfsDep
	hasher     hashid.Encoder
	settings   setting.Provider
	kv         cache.Driver
	auth       auth.Auth
	l          *log.Helper
	conf       *conf.Bootstrap
	mm         mime.MimeManager
	eventHub   eventhub.EventHub
}

func NewFileService(c pbuser.UserClient, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep, fc data.FileClient,
	pc data.StoragePolicyClient, hasher hashid.Encoder, settings setting.Provider, kv cache.Driver,
	auth auth.Auth, l log.Logger, bs *conf.Bootstrap, mm mime.MimeManager, eventHub eventhub.EventHub) *FileService {
	return &FileService{
		userClient:   c,
		managerDep:   dep,
		dbfsDep:      dbfsDep,
		fileClient:   fc,
		policyClient: pc,
		hasher:       hasher,
		settings:     settings,
		kv:           kv,
		auth:         auth,
		l:            log.NewHelper(l, log.WithMessageKey("service-file")),
		conf:         bs,
		mm:           mm,
		eventHub:     eventHub,
	}
}

func (s *FileService) ListDirectory(ctx context.Context, req *pbfile.ListFileRequest) (*pbfile.ListFileResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %s", req.Uri)
	}

	parent, res, err := m.List(ctx, uri, &manager.ListArgs{
		Page:           int(req.Page),
		PageSize:       int(req.PageSize),
		Order:          req.OrderBy,
		OrderDirection: utils.FromProtoOrderDirection(req.OrderDirection),
	})
	if err != nil {
		return nil, err
	}

	listResponse := buildListResponse(ctx, user.Id, parent, res, s.hasher, s.settings)
	return listResponse, nil
}

func (s *FileService) ListArchiveFiles(ctx context.Context, req *pbfile.ListArchiveFilesRequest) (*pbfile.ListArchiveFilesResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri")
	}

	files, err := m.ListArchiveFiles(ctx, uri, req.Entity, req.TextEncoding)
	if err != nil {
		return nil, commonpb.ErrorUnspecified("failed to list archive files: %w", err)
	}

	return &pbfile.ListArchiveFilesResponse{Files: files}, nil
}
func (s *FileService) CreateFile(ctx context.Context, req *pbfile.CreateFileRequest) (*pbfile.FileResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %s", req.Uri)
	}

	fileType := types.FileTypeFromString(req.Type)
	opts := []fs.Option{
		fs.WithMetadata(req.Metadata),
	}
	if req.ErrOnConflict {
		opts = append(opts, dbfs.WithErrorOnConflict())
	}
	file, err := m.Create(ctx, uri, fileType, opts...)
	if err != nil {
		return nil, commonpb.ErrorUnspecified("failed to create files: %w", err)
	}

	return buildFileResponse(ctx, user.Id, file, s.hasher, nil, s.settings), nil
}
func (s *FileService) RenameFile(ctx context.Context, req *pbfile.RenameFileRequest) (*pbfile.FileResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %s", req.Uri)
	}

	file, err := m.Rename(ctx, uri, req.NewName)
	if err != nil {
		return nil, commonpb.ErrorUnspecified("failed to rename files: %w", err)
	}

	return buildFileResponse(ctx, user.Id, file, s.hasher, nil, s.settings), nil
}
func (s *FileService) MoveOrCopyFile(ctx context.Context, req *pbfile.MoveOrCopyFileRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uris, err := fs.NewUriFromStrings(req.Uris...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri")
	}

	dst, err := fs.NewUriFromString(req.Dst)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %s", req.Dst)
	}

	return &emptypb.Empty{}, m.MoveOrCopy(ctx, uris, dst, req.Copy)
}
func (s *FileService) FileUrl(ctx context.Context, req *pbfile.FileUrlRequest) (*pbfile.FileUrlResponse, error) {
	if req.Archive {
		return s.GetArchiveDownloadSession(ctx, req)
	}

	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uris, err := fs.NewUriFromStrings(req.Uris...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri")
	}

	expire := time.Now().Add(s.settings.EntityUrlValidDuration(ctx))
	urlReq := lo.Map(uris, func(uri *fs.URI, index int) manager.GetEntityUrlArgs {
		return manager.GetEntityUrlArgs{
			URI:               uri,
			PreferredEntityID: req.Entity,
		}
	})

	var newCtx = ctx
	if req.UsePrimarySiteUrl {
		newCtx = setting.UseFirstSiteUrl(newCtx)
	}

	res, earliestExpire, err := m.GetEntityUrls(newCtx, urlReq,
		fs.WithIsDownload(req.Download),
		fs.WithNoCache(req.NoCache),
		fs.WithUrlExpire(&expire),
	)
	if err != nil && !req.SkipError {
		return nil, fmt.Errorf("failed to get entity url: %w", err)
	}

	return &pbfile.FileUrlResponse{
		Urls:       res,
		Expires:    timestamppb.New(*earliestExpire),
		IsRedirect: req.Redirect,
	}, nil
}
func (s *FileService) PutContent(ctx context.Context, req *pbfile.FileUpdateRequest) (*pbfile.FileResponse, error) {
	return s.putContentWithLockSession(ctx, req, nil)
}
func (s *FileService) PutContentStream(stream grpc.ClientStreamingServer[pbfile.StreamFileUpdateRequest, pbfile.FileResponse]) error {
	// 1. 先读第一包拿 file_info
	firstReq, err := stream.Recv()
	if err != nil {
		return err
	}
	info := firstReq.GetFileInfo()
	if info == nil || info.Size <= 0 {
		return commonpb.ErrorParamInvalid("file size must be greater than 0")
	}
	uri, err := fs.NewUriFromString(info.Uri)
	if err != nil {
		return commonpb.ErrorParamInvalid("unknown uri: %s", info.Uri)
	}

	// 2. 构造 StreamReader，extractChunk 只关心 chunk_data 包
	reader := utils.NewStreamReader(stream, func(req *pbfile.StreamFileUpdateRequest) ([]byte, bool) {
		chunk, ok := req.Payload.(*pbfile.StreamFileUpdateRequest_ChunkData)
		if !ok {
			return nil, false
		}
		return chunk.ChunkData, true
	})
	defer reader.Close()

	// 3. 直接作为 io.ReadCloser 传给存储层
	fileData := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:             uri,
			Size:            info.Size,
			PreviousVersion: info.Previous,
		},
		File: reader,
		Mode: fs.ModeOverwrite,
	}

	ctx := stream.Context()
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	res, err := m.Update(ctx, fileData)
	if err != nil {
		return fmt.Errorf("failed to update files: %w", err)
	}
	// 4. 上传完成后，校验实际接收大小
	if info.Size != reader.ReceivedSize() {
		// 校验失败，传输中断或数据损坏，应该删除残缺文件并报错
		if err := m.Delete(ctx, []*fs.URI{uri}); err != nil {
			s.l.Errorf("failed to delete file %s: %v", uri, err)
		}
		return commonpb.ErrorParamInvalid("file size mismatch")
	}

	return stream.SendAndClose(buildFileResponse(ctx, user.Id, res, s.hasher, nil, s.settings))
}
func (s *FileService) GetThumb(ctx context.Context, req *pbfile.FileThumbRequest) (*pbfile.FileThumbResponse, error) {
	return &pbfile.FileThumbResponse{}, nil
}
func (s *FileService) DeleteFile(ctx context.Context, req *pbfile.DeleteFileRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uris, err := fs.NewUriFromStrings(req.Uris...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri")
	}

	if req.UnlinkOnly {
		return nil, commonpb.ErrorUnspecified("advance delete permission is required")
	}

	// Delete files
	if err = m.Delete(ctx, uris, fs.WithUnlinkOnly(req.UnlinkOnly), fs.WithSkipSoftDelete(req.SkipSoftDelete)); err != nil {
		return nil, commonpb.ErrorUnspecified("failed to delete files: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) UnlockFile(ctx context.Context, req *pbfile.UnlockFileRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	// Unlock files
	if err := m.Unlock(ctx, req.Tokens...); err != nil {
		return nil, commonpb.ErrorUnspecified("failed to unlock files: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) RestoreFile(ctx context.Context, req *pbfile.DeleteFileRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uris, err := fs.NewUriFromStrings(req.Uris...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri")
	}

	// Restore files
	if err = m.Restore(ctx, uris...); err != nil {
		return nil, commonpb.ErrorUnspecified("failed to restore files: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) PatchMetadata(ctx context.Context, req *pbfile.PatchMetadataRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uris, err := fs.NewUriFromStrings(req.Uris...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri")
	}

	return &emptypb.Empty{}, m.PatchMedata(ctx, uris, req.Patches...)
}
func (s *FileService) CreateUploadSession(ctx context.Context, req *pbfile.CreateUploadSessionRequest) (*pbfile.UploadSessionResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	var entityType *int
	switch req.EntityType {
	case "live_photo":
		livePhoto := types.EntityTypeLivePhoto
		entityType = &livePhoto
	case "version":
		version := types.EntityTypeVersion
		entityType = &version
	}

	policyId, err := s.hasher.Decode(req.PolicyId, hashid.PolicyID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown policy id: %w", err)
	}

	uploadRequest := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:  uri,
			Size: req.Size,

			MimeType:               req.MimeType,
			Metadata:               req.Metadata,
			EntityType:             entityType,
			PreferredStoragePolicy: policyId,
		},
	}

	if req.LastModified > 0 {
		lastModified := time.UnixMilli(req.LastModified)
		uploadRequest.Props.LastModified = &lastModified
	}

	credential, err := m.CreateUploadSession(ctx, uploadRequest)
	if err != nil {
		return nil, err
	}

	return buildUploadSessionResponse(credential, s.hasher), nil
}
func (s *FileService) UploadFile(ctx context.Context, req *pbfile.UploadFileRequest) (*emptypb.Empty, error) {
	uploadSessionRaw, ok := s.kv.Get(manager.UploadSessionCachePrefix + req.SessionId)
	if !ok {
		return nil, filepb.ErrorUploadSessionExpired("upload session expired")
	}

	uploadSession := uploadSessionRaw.(fs.UploadSession)

	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	if uploadSession.UID != int(user.Id) {
		return nil, filepb.ErrorUploadSessionExpired("upload session expired")
	}

	placeholder, err := m.ConfirmUploadSession(ctx, &uploadSession, int(req.Index))
	if err != nil {
		return nil, err
	}

	return processChunkUpload(ctx, m, &uploadSession, req.Index, placeholder, fs.ModeOverwrite)
}

func processChunkUpload(ctx context.Context, m manager.FileManager, session *fs.UploadSession, index int32,
	file fs.File, mode fs.WriteMode) (*emptypb.Empty, error) {
	// 取得并校验文件大小是否符合分片要求
	chunkSize := session.ChunkSize
	isLastChunk := session.ChunkSize == 0 || int64(index+1)*chunkSize >= session.Props.Size
	expectedLength := chunkSize
	if isLastChunk {
		expectedLength = session.Props.Size - int64(index)*chunkSize
	}

	rc, fileSize, err := request.SniffContentLength(ctx)
	if err != nil || (expectedLength != fileSize) {
		return nil, filepb.ErrorInvalidContentLength("Invalid Content-Length (expected: %d)", expectedLength)
	}

	// 非首个分片时需要允许覆盖
	if index > 0 {
		mode |= fs.ModeOverwrite
	}

	req := &fs.UploadRequest{
		File:   rc,
		Offset: chunkSize * int64(index),
		Props:  session.Props.Copy(),
		Mode:   mode,
	}

	// 执行上传
	ctx = context.WithValue(ctx, cluster.SlaveNodeIDCtx{}, strconv.Itoa(session.Policy.NodeID))
	err = m.Upload(ctx, req, session.Policy, session)
	if err != nil {
		return nil, err
	}

	if rc, ok := req.File.(request.LimitReaderCloser); ok {
		if rc.Count() != expectedLength {
			return nil, commonpb.ErrorIoFailed("uploaded data(%d) does not match purposed size(%d)", rc.Count(), req.Props.Size)
		}
	}

	// Finish upload
	if isLastChunk {
		_, err := m.CompleteUpload(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("failed to complete upload: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) DeleteUploadSession(ctx context.Context, req *pbfile.DeleteUploadSessionRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	return &emptypb.Empty{}, m.CancelUploadSession(ctx, uri, req.Id)
}
func (s *FileService) GetFileInfo(ctx context.Context, req *pbfile.GetFileInfoRequest) (*pbfile.FileResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	if req.Id != "" && req.Uri == "" {
		fileId, err := s.hasher.Decode(req.Id, hashid.FileID)
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("unknown files id: %w", err)
		}

		file, err := m.TraverseFile(ctx, fileId)
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("failed to traverse files: %w", err)
		}

		req.Uri = file.Uri(false).String()
	}

	if req.Uri == "" {
		return nil, commonpb.ErrorParamInvalid("uri is required")
	}

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	opts := []fs.Option{dbfs.WithFilePublicMetadata(), dbfs.WithNotRoot()}
	if req.ExtendedInfo {
		opts = append(opts, dbfs.WithExtendedInfo(), dbfs.WithEntityUser(), dbfs.WithFileShareIfOwned())
	}
	if req.FolderSummary {
		opts = append(opts, dbfs.WithLoadFolderSummary())
	}

	file, err := m.Get(ctx, uri, opts...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("failed to get files: %w", err)
	}

	if file == nil {
		return nil, commonpb.ErrorNotFound("files not found: %s", req.Uri)
	}

	return buildFileResponse(ctx, user.Id, file, s.hasher, nil, s.settings), nil
}
func (s *FileService) SetCurrentVersion(ctx context.Context, req *pbfile.SetCurrentVersionRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	versionId, err := s.hasher.Decode(req.Version, hashid.EntityID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown version id: %w", err)
	}

	if err := m.SetCurrentVersion(ctx, uri, versionId); err != nil {
		return nil, commonpb.ErrorUnspecified("failed to set current version: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) DeleteVersion(ctx context.Context, req *pbfile.DeleteVersionRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	versionId, err := s.hasher.Decode(req.Version, hashid.EntityID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown version id: %w", err)
	}

	if err := m.DeleteVersion(ctx, uri, versionId); err != nil {
		return nil, fmt.Errorf("failed to delete version: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) CreateViewerSession(ctx context.Context, req *pbfile.CreateViewerSessionRequest) (*pbfile.ViewerSessionResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	// Find the given viewer
	viewers := s.settings.FileViewers(ctx)
	var targetViewer *ftypes.Viewer
	for _, group := range viewers {
		for _, viewer := range group.Viewers {
			if viewer.ID == req.ViewerId && !viewer.Disabled {
				targetViewer = &viewer
				break
			}
		}

		if targetViewer != nil {
			break
		}
	}

	if targetViewer == nil {
		return nil, commonpb.ErrorParamInvalid("unknown viewer id: %w", err)
	}

	viewerSession, err := m.CreateViewerSession(ctx, uri, req.Version, targetViewer)
	if err != nil {
		return nil, err
	}

	res := &pbfile.ViewerSessionResponse{
		Session: &pbfile.ViewerSession{
			Id:          viewerSession.ID,
			AccessToken: viewerSession.AccessToken,
			Expires:     viewerSession.Expires,
		},
	}
	if targetViewer.Type == types.ViewerTypeWopi {
		// For WOPI viewer, generate WOPI src
		base := s.settings.SiteURL(setting.UseFirstSiteUrl(ctx))
		fileId := hashid.EncodeFileID(s.hasher, viewerSession.File.ID())
		wopiSrc, err := wopi.GenerateWopiSrc(base, ftypes.ViewerAction(req.PreferAction), targetViewer, viewerSession, fileId)
		if err != nil {
			return nil, commonpb.ErrorInternalSetting("failed to generate wopi src: %w", err)
		}
		res.WopiSrc = wopiSrc.String()
	}

	return res, nil
}
func (s *FileService) GetSource(ctx context.Context, req *pbfile.GetSourceRequest) (*pbfile.GetSourceResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uris, err := fs.NewUriFromStrings(req.Uris...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	res, err := m.GetDirectLink(ctx, uris...)
	return buildDirectLinkResponse(res), err
}
func (s *FileService) DeleteDirectLink(ctx context.Context, req *pbfile.DeleteDirectLinkRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	linkId := hashid.FromContext(ctx)
	newCtx := context.WithValue(ctx, data.LoadDirectLinkFile{}, true)
	link, err := s.directLinkClient.GetByID(newCtx, linkId)
	if err != nil || link.Edges.File == nil {
		return nil, commonpb.ErrorNotFound("direct link not found: %w", err)
	}

	if link.Edges.File.OwnerID != int(user.Id) {
		return nil, commonpb.ErrorNotFound("direct link not found: %w", err)
	}
	if err := s.directLinkClient.Delete(ctx, link.ID); err != nil {
		return nil, commonpb.ErrorDb("failed to delete direct link: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) PatchView(ctx context.Context, req *pbfile.PatchViewRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	view := utils.FromProtoView(req.View)
	if err := m.PatchView(ctx, uri, view); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
func (s *FileService) AnonymousPermLink(ctx context.Context, req *pbfile.AnonymousPermLinkRequest) (*pbfile.RedirectResponse, error) {
	return s.redirectDirectLink(ctx, req.Id, req.Name, false)
}

func (s *FileService) AnonymousPermLinkD(ctx context.Context, req *pbfile.AnonymousPermLinkRequest) (*pbfile.RedirectResponse, error) {
	return s.redirectDirectLink(ctx, req.Id, req.Name, true)
}

func (s *FileService) DeleteFilesByUserId(ctx context.Context, req *pbfile.SimpleUserRequest) (*emptypb.Empty, error) {
	//ae := make(map[string]string)
	//for _, id := range req.Ids {
	//	fc, tx, ctx, err := data.WithTx(ctx, s.fileClient)
	//	if err != nil {
	//		ae[string(id)] = fmt.Sprintf("Failed to start transaction: %v", err)
	//		continue
	//	}
	//
	//	if err := fc.DeleteByUser(ctx, int(id)); err != nil {
	//		_ = data.Rollback(tx)
	//		ae[string(id)] = fmt.Sprintf("Failed to delete user files: %v", err)
	//		continue
	//	}
	//
	//	if err := data.Commit(tx); err != nil {
	//		ae[string(id)] = fmt.Sprintf("Failed to commit transaction: %v", err)
	//	}
	//}
	//
	//if len(ae) > 0 {
	//	return nil, commonpb.ErrorNotFullySuccess("some files failed to delete").WithMetadata(ae)
	//}

	fc, tx, ctx, err := data.WithTx(ctx, s.fileClient)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to start transaction: %v", err)
	}

	uids := lo.Map(req.Ids, func(id int32, index int) int {
		return int(id)
	})
	if _, err := fc.DeleteByUserIds(ctx, uids...); err != nil {
		_ = data.Rollback(tx)
		return nil, commonpb.ErrorDb("Failed to delete user files: %v", err)
	}

	if err := data.Commit(tx); err != nil {
		return nil, commonpb.ErrorDb("Failed to commit transaction: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *FileService) CountByTimeRange(ctx context.Context, req *pbfile.TimeRangeRequest) (*pbfile.CountByTimeRangeResponse, error) {
	var start, end time.Time
	if req.Start != nil {
		start = req.Start.AsTime()
	}
	if req.End != nil {
		end = req.End.AsTime()
	}
	count, err := s.fileClient.CountByTimeRange(ctx, &start, &end)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to count files: %v", err)
	}
	return &pbfile.CountByTimeRangeResponse{
		Count: int32(count),
	}, nil
}

type ExplorerEventRequest struct {
	Uri string `form:"uri" binding:"required"`
}

func (s *FileService) HandleExplorerEventsPush(ctx khttp.Context) error {
	var reqBody ExplorerEventRequest
	if err := ctx.Bind(&reqBody); err != nil {
		return fmt.Errorf("failed to bind request body: %v", err)
	}

	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(reqBody.Uri)
	if err != nil {
		return commonpb.ErrorParamInvalid("unknown uri: %v", err)
	}

	// Make sure target is a valid folder that the user can listen to
	parent, _, err := m.List(ctx, uri, &manager.ListArgs{
		Page:     0,
		PageSize: 1,
	})
	if err != nil {
		return commonpb.ErrorDb("Requested uri not available: %v", err)
	}

	requestInfo := requestinfo.RequestInfoFromContext(ctx)
	if requestInfo.ClientID == "" {
		return commonpb.ErrorParamInvalid("client ID is required")
	}

	// Client ID must be a valid UUID
	if _, err := uuid.FromString(requestInfo.ClientID); err != nil {
		return commonpb.ErrorParamInvalid("invalid client ID: %v", err)
	}

	// Subscribe
	rx, resumed, err := s.eventHub.Subscribe(ctx, parent.ID(), requestInfo.ClientID)
	if err != nil {
		return commonpb.ErrorInternalSetting("Failed to subscribe to events: %v", err)
	}

	w := ctx.Response()
	req := ctx.Request()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	keepAliveTicker := time.NewTicker(30 * time.Second)
	defer keepAliveTicker.Stop()

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported")
	}
	if resumed {
		_, err = fmt.Fprint(w, "event: resumed\ndata: \n\n")
		flusher.Flush()
	} else {
		_, err = fmt.Fprint(w, "event: subscribed\ndata: \n\n)")
		flusher.Flush()
	}

	for {
		select {
		// TODO: close connection after access token expired
		case <-req.Context().Done():
			// Server shutdown or request cancelled
			s.eventHub.Unsubscribe(ctx, parent.ID(), requestInfo.ClientID)
			s.l.Debug("Request context done, unsubscribed from event hub")
			return nil
		case <-ctx.Done():
			s.eventHub.Unsubscribe(ctx, parent.ID(), requestInfo.ClientID)
			s.l.Debug("Unsubscribed from event hub")
			if err != nil {
				s.l.Errorf("Error occurred: %+v", err.Error())
				s.eventHub.Unsubscribe(ctx, parent.ID(), requestInfo.ClientID)
				s.l.Debug("Unsubscribed from event hub")
			}
			return nil
		case evt, ok := <-rx:
			if !ok {
				// Channel closed, EventHub is shutting down
				s.l.Debug("Event hub closed, disconnecting client")
				return nil
			}
			_, err = fmt.Fprintf(w, "event: %s\n", evt.Type)
			s.l.Debug("Event sent: %+v", evt)
			flusher.Flush()
		case <-keepAliveTicker.C:
			_, err = fmt.Fprintf(w, "event: keep-alive\n")
			flusher.Flush()
		}
	}
}

func (s *FileService) UploadAvatar(ctx khttp.Context) error {
	req := ctx.Request()
	user := trans.FromContext(ctx)
	avatarSettings := s.settings.AvatarProcess(ctx)
	if req.ContentLength == -1 || req.ContentLength > avatarSettings.MaxFileSize {
		io.Copy(io.Discard, req.Body)
		return filepb.ErrorFileTooLarge(fmt.Sprintf("Avatar file size must be less than %d bytes", avatarSettings.MaxFileSize))
	}

	if req.ContentLength == 0 {
		return errors.BadRequest("avatar file is empty", "default avatar will be used")
	}

	return s.updateAvatarFile(ctx, user, req.Header.Get("Content-Type"), req.Body, avatarSettings)
}

func (s *FileService) updateAvatarFile(ctx khttp.Context, user *userpb.User, contentType string, file io.ReadCloser, avatarSettings *setting.AvatarProcess) error {
	ext := "png"
	switch contentType {
	case "image/jpeg", "image/jpg":
		ext = "jpg"
	case "image/gif":
		ext = "gif"
	}
	avatar, err := thumb.NewThumbFromFile(file, ext)
	if err != nil {
		return commonpb.ErrorParamInvalid("Invalid image: %w", err)
	}

	// Resize and save avatar
	avatar.CreateAvatar(avatarSettings.MaxWidth)
	avatarRoot := util.DataPath(avatarSettings.Path)
	f, err := util.CreateNestedFile(filepath.Join(avatarRoot, fmt.Sprintf("avatar_%d.png", user.Id)))
	if err != nil {
		return commonpb.ErrorIoFailed("Failed to create avatar file: %w", err)
	}
	defer f.Close()

	if err := avatar.Save(f, &setting.ThumbEncode{
		Quality: 100,
		Format:  "png",
	}); err != nil {
		return commonpb.ErrorIoFailed("Failed to save avatar file: %w", err)
	}

	if _, err := s.userClient.UpdateAvatar(ctx, &pbuser.UpdateAvatarRequest{
		Type: pbuser.AvatarType_FILE_AVATAR,
	}); err != nil {
		return err
	}

	return nil
}

type (
	ArchiveDownloadSession struct {
		Uris        []*fs.URI `json:"uris"`
		RequesterID int       `json:"requester_id"`
	}
)

func (s *FileService) GetArchiveDownloadSession(ctx context.Context, req *pbfile.FileUrlRequest) (*pbfile.FileUrlResponse, error) {
	user := trans.FromContext(ctx)
	uris, err := fs.NewUriFromStrings(req.Uris...)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Unknown uri: %w", err)
	}

	// users permission

	archiveSession := &ArchiveDownloadSession{
		Uris:        uris,
		RequesterID: int(user.Id),
	}
	sessionId := uuid.Must(uuid.NewV4()).String()

	settings := s.settings
	kv := s.kv

	ttl := settings.ArchiveDownloadSessionTTL(ctx)
	expire := time.Now().Add(time.Duration(ttl) * time.Second)
	if err := kv.Set(types.ArchiveDownloadSessionPrefix+sessionId, *archiveSession, ttl); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to create archive download session: %w", err)
	}

	base := settings.SiteURL(ctx)
	downloadUrl := routes.MasterArchiveDownloadUrl(base, sessionId)
	finalUrl, err := auth.SignURI(s.auth, downloadUrl.String(), &expire)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to sign archive download url: %w", err)
	}

	return &pbfile.FileUrlResponse{
		Urls:    []*pbfile.EntityUrl{{Url: finalUrl.String()}},
		Expires: timestamppb.New(expire),
	}, nil
}

// ServeEntity serves the entity content.
func (s *FileService) ServeEntity(ctx khttp.Context) error {
	var reqBody pbfile.GetEntityRequest
	if err := ctx.BindVars(&reqBody); err != nil {
		return fmt.Errorf("failed to bind request body: %w", err)
	}

	entityId, err := s.hasher.Decode(reqBody.Id, hashid.EntityID)
	settings := s.settings
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	entitySource, err := m.GetEntitySource(ctx, entityId)
	if err != nil {
		return fmt.Errorf("failed to get entity source: %w", err)
	}
	defer entitySource.Close()

	w := ctx.Response()
	req := ctx.Request()

	maxAge := settings.PublicResourceMaxAge(ctx)
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))

	isDownload := req.URL.Query().Get(routes.IsDownloadQuery) != ""
	isThumb := req.URL.Query().Get(routes.IsThumbQuery) != ""
	entitySource.Serve(w, req,
		entitysource.WithSpeedLimit(reqBody.Speed),
		entitysource.WithDownload(isDownload),
		entitysource.WithDisplayName(reqBody.Name),
		entitysource.WithContext(ctx),
		entitysource.WithThumb(isThumb),
	)
	return nil
}

func (s *FileService) putContentWithLockSession(ctx context.Context, req *pbfile.FileUpdateRequest, ls fs.LockSession) (*pbfile.FileResponse, error) {
	rc, fileSize, err := request.SniffContentLength(ctx)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid content length: %w", err)
	}

	settings := s.settings
	if fileSize > settings.MaxOnlineEditSize(ctx) {
		return nil, fs.ErrFileSizeTooBig
	}

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Unknown uri: %w", err)
	}

	fileData := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:             uri,
			Size:            fileSize,
			PreviousVersion: req.Previous,
		},
		File: rc,
		Mode: fs.ModeOverwrite,
	}

	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.managerDep, s.dbfsDep, user)
	defer m.Recycle()

	var newCtx context.Context = ctx
	if ls != nil {
		newCtx = fs.LockSessionToContext(ctx, ls)
	}

	res, err := m.Update(newCtx, fileData)
	if err != nil {
		return nil, fmt.Errorf("failed to update files: %w", err)
	}

	return buildFileResponse(ctx, user.Id, res, s.hasher, nil, s.settings), nil
}

func (s *FileService) redirectDirectLink(ctx context.Context, id, name string, download bool) (*pbfile.RedirectResponse, error) {
	sourceLinkID, err := s.hasher.Decode(id, hashid.SourceLinkID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse object id: %w", err)
	}
	newCtx := context.WithValue(ctx, data.LoadDirectLinkFile{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileEntity{}, true)
	dl, err := s.directLinkClient.GetByNameID(newCtx, sourceLinkID, name)
	if err != nil {
		return nil, commonpb.ErrorNotFound("Direct link not found: %w", err)
	}

	m := manager.NewFileManager(s.managerDep, s.dbfsDep, nil)
	defer m.Recycle()

	// Request entity url
	expire := time.Now().Add(s.settings.EntityUrlValidDuration(ctx))
	res, earliestExpire, err := m.GetUrlForRedirectedDirectLink(ctx, dl,
		fs.WithUrlExpire(&expire),
		fs.WithIsDownload(download),
	)
	if err != nil {
		return nil, err
	}
	return &pbfile.RedirectResponse{
		Url:    res,
		Expire: int32(earliestExpire.Sub(time.Now()).Seconds()),
	}, nil
}

func (s *FileService) validateBatchFileCount(ctx context.Context, uris ...string) error {
	limit := s.settings.MaxBatchedFile(ctx)
	if len(uris) > limit {
		return filepb.ErrorTooManyUris("Max batched file count is %d", limit)
	}
	return nil
}
