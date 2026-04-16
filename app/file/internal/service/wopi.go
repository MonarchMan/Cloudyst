package service

import (
	commonpb "api/api/common/v1"
	pbfile "api/api/file/files/v1"
	pb "api/api/file/wopi/v1"
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	ftypes "api/external/data/file"
	"api/external/trans"
	"common/cache"
	"common/constants"
	"common/hashid"
	"context"
	"file/internal/biz/cluster/routes"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/lock"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"file/internal/data"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"file/internal/pkg/wopi"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type WopiService struct {
	fileService *FileService
	hasher      hashid.Encoder
	settings    setting.Provider
	dep         filemanager.ManagerDep
	dbfsDep     filemanager.DbfsDep
	kv          cache.Driver
	uc          pbuser.UserClient
}

func NewWopiService(fs *FileService, hasher hashid.Encoder, settings setting.Provider, dep filemanager.ManagerDep,
	dbfsDep filemanager.DbfsDep, kv cache.Driver, uc pbuser.UserClient) *WopiService {
	return &WopiService{
		fileService: fs,
		hasher:      hasher,
		settings:    settings,
		dep:         dep,
		dbfsDep:     dbfsDep,
		kv:          kv,
		uc:          uc,
	}
}

func (s *WopiService) CheckFileInfo(ctx khttp.Context) error {
	res, err := s.fileInfo(ctx)
	w := ctx.Response()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set(wopi.ServerErrorHeader, err.Error())
	}
	_ = ctx.JSON(http.StatusOK, res)
	return nil
}

func (s *WopiService) fileInfo(ctx khttp.Context) (*pb.WopiFileInfo, error) {
	uri, m, user, viewerSession, err := s.prepareFs(ctx)
	if err != nil {
		return nil, err
	}

	hasher := s.hasher
	settings := s.settings

	opts := []fs.Option{
		dbfs.WithFilePublicMetadata(),
		dbfs.WithExtendedInfo(),
		dbfs.WithNotRoot(),
		dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile, dbfs.NavigatorCapabilityInfo),
	}
	file, err := m.Get(ctx, uri, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	if file == nil {
		return nil, commonpb.ErrorNotFound("file not found")
	}

	versionType := types.EntityTypeVersion
	find, targetEntity := fs.FindDesiredEntity(file, viewerSession.Version, hasher, &versionType)
	if !find {
		return nil, commonpb.ErrorNotFound("version not found: %w", err)
	}

	isSelf := file.OwnerID() == int(user.Id)
	canEdit := file.PrimaryEntityID() == targetEntity.ID() && isSelf && uri.FileSystem() == constants.FileSystemMy
	cantPutRelative := !canEdit
	siteUrl := settings.SiteURL(ctx)
	info := &pb.WopiFileInfo{
		BaseFileName:            file.DisplayName(),
		Version:                 hashid.EncodeEntityID(hasher, targetEntity.ID()),
		BreadcrumbBrandName:     settings.SiteBasic(ctx).Name,
		BreadcrumbBrandUrl:      siteUrl.String(),
		FileSharingPostMessage:  isSelf,
		EnableShare:             isSelf,
		FileVersionPostMessage:  true,
		ClosePostMessage:        true,
		PostMessageOrigin:       "*",
		FileNameMaxLength:       dbfs.MaxFileNameLength,
		LastModifiedTime:        file.UpdatedAt().Format(time.RFC3339),
		IsAnonymousUser:         data.IsAnonymousUser(user),
		UserId:                  hashid.EncodeUserID(hasher, int(user.Id)),
		ReadOnly:                !canEdit,
		Size:                    targetEntity.Size(),
		OwnerId:                 hashid.EncodeUserID(hasher, file.OwnerID()),
		SupportsRename:          true,
		SupportsReviewing:       true,
		SupportsLocks:           true,
		UserCanReview:           canEdit,
		UserCanWrite:            canEdit,
		UserCanNotWriteRelative: cantPutRelative,
		BreadcrumbFolderName:    uri.Dir(),
		BreadcrumbFolderUrl:     routes.FrontendHomeUrl(siteUrl, uri.DirUri().String()).String(),
	}

	return info, nil
}
func (s *WopiService) PutFile(ctx khttp.Context) error {
	_, err := s.PutFileRelative(ctx, false)
	w := ctx.Response()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set(wopi.ServerErrorHeader, err.Error())
	}
	return nil
}
func (s *WopiService) ModifyFile(ctx khttp.Context) error {
	var (
		action string
		err    error
	)
	if tr, ok := transport.FromServerContext(ctx); ok {
		if ht, ok := tr.(khttp.Transporter); ok {
			reqHeader := ht.RequestHeader()
			action = reqHeader.Get(wopi.OverwriteHeader)
		}
	}

	w := ctx.Response()
	switch action {
	case wopi.MethodLock:
		err = s.Lock(ctx)
	case wopi.MethodRefreshLock:
		err = s.RefreshLock(ctx)
	case wopi.MethodUnlock:
		err = s.Unlock(ctx)
	case wopi.MethodPutRelative:
		_, err = s.PutFileRelative(ctx, true)
	default:
		w.WriteHeader(http.StatusNotImplemented)
		return nil
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set(wopi.ServerErrorHeader, err.Error())
	}
	return nil
}

func (s *WopiService) GetFile(ctx khttp.Context) error {
	uri, m, _, viewerSession, err := s.prepareFs(ctx)
	if err != nil {
		return err
	}

	file, err := m.Get(ctx, uri, dbfs.WithExtendedInfo(), dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile),
		dbfs.WithNotRoot())
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}

	versionType := types.EntityTypeVersion
	find, targetEntity := fs.FindDesiredEntity(file, viewerSession.Version, s.hasher, &versionType)
	if !find {
		return commonpb.ErrorNotFound("version not found: %w", err)
	}

	if targetEntity.Size() > s.settings.MaxOnlineEditSize(ctx) {
		return fs.ErrFileSizeTooBig
	}

	entitySource, err := m.GetEntitySource(ctx, targetEntity.ID(), fs.WithEntity(targetEntity))
	if err != nil {
		return fmt.Errorf("failed to get entity source: %w", err)
	}

	defer entitySource.Close()

	w := ctx.Response()
	req := ctx.Request()
	entitySource.Serve(w, req, entitysource.WithContext(ctx))

	return nil
}

func (s *WopiService) PutFileRelative(ctx context.Context, isPutRelative bool) (string, error) {
	uri, m, user, viewerSession, err := s.prepareFs(ctx)
	if err != nil {
		return "", err
	}

	// Make sure files exists and readable
	file, err := m.Get(ctx, uri, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityUploadFile), dbfs.WithNotRoot())
	if err != nil {
		return "", fmt.Errorf("failed to get files: %w", err)
	}

	var lockSession fs.LockSession
	reqHeader, respHeader, err := utils.HeaderFromContext(ctx)
	if err != nil {
		return "", err
	}
	lockToken := reqHeader.Get(wopi.LockTokenHeader)
	if lockToken != "" {
		release, ls, err := m.ConfirmLock(ctx, file, file.Uri(false), lockToken)
		if err != nil {
			app := lock.Application{
				Type:     string(fs.ApplicationViewer),
				ViewerID: viewerSession.ViewerID,
			}
			ls, err := m.Lock(ctx, wopi.LockDuration, int(user.Id), true, app, file.Uri(false), lockToken)
			if err != nil {
				return "", err
			}

			respHeader.Set(wopi.LockTokenHeader, "")
			_ = m.Unlock(ctx, ls.LastToken())
		} else {
			defer release()
		}
		lockSession = ls
	}

	fileUri := viewerSession.Uri
	if isPutRelative {
		// If the header contains only a files extension (starts with a period), then the resulting files name will consist of this extension and the initial files name without extension.
		// If the header contains a full files name, then it will be a name for the resulting files.
		fileName, err := wopi.UTF7Decode(reqHeader.Get(wopi.SuggestedTargetHeader))
		if err != nil {
			return "", fmt.Errorf("failed to decode X-WOPI-SuggestedTarget header (UTF-7): %w", err)
		}

		fileUriParsed, err := fs.NewUriFromString(fileUri)
		if err != nil {
			return "", fmt.Errorf("failed to parse files uri: %w", err)
		}

		if strings.HasPrefix(fileName, ".") {
			fileName = strings.TrimSuffix(fileUriParsed.Name(), filepath.Ext(fileUriParsed.Name())) + fileName
		}
		fileUri = fileUriParsed.DirUri().JoinRaw(fileName).String()
	}

	newReq := &pbfile.FileUpdateRequest{
		Uri: fileUri,
	}
	res, err := s.fileService.putContentWithLockSession(ctx, newReq, lockSession)
	if err != nil {
		return "", err
	}

	respHeader.Set(wopi.ItemVersionHeader, res.PrimaryEntity)
	if isPutRelative {
		return res.Name, nil
	}
	return "", nil
}

func (s *WopiService) prepareFs(ctx context.Context) (*fs.URI, manager.FileManager, *userpb.User, *manager.ViewerSessionCache, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	viewerSession := manager.ViewerSessionFromContext(ctx)
	uri, err := fs.NewUriFromString(viewerSession.Uri)
	if err != nil {
		return nil, nil, nil, nil, commonpb.ErrorParamInvalid("unknown uri: %w", err)
	}

	return uri, m, user, viewerSession, nil
}

func (s *WopiService) Lock(ctx context.Context) error {
	uri, m, user, viewerSession, err := s.prepareFs(ctx)
	if err != nil {
		return err
	}

	file, err := m.Get(ctx, uri, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityUploadFile), dbfs.WithNotRoot())
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}

	reqHeader, respHeader, err := utils.HeaderFromContext(ctx)
	if err != nil {
		return err
	}
	lockToken := reqHeader.Get(wopi.LockTokenHeader)
	release, _, err := m.ConfirmLock(ctx, file, file.Uri(false), lockToken)
	if err != nil {
		app := lock.Application{
			Type:     string(fs.ApplicationViewer),
			ViewerID: viewerSession.ViewerID,
		}
		_, err = m.Lock(ctx, wopi.LockDuration, int(user.Id), true, app, file.Uri(false), lockToken)
		if err != nil {
			return err
		}

		respHeader.Set(wopi.LockTokenHeader, "")
		return nil
	}

	release()
	_, err = m.Refresh(ctx, wopi.LockDuration, lockToken)
	if err != nil {
		return err
	}

	respHeader.Set(wopi.LockTokenHeader, lockToken)
	return nil
}

func (s *WopiService) hashId(fileId string) (int, error) {
	id, err := s.hasher.Decode(fileId, hashid.FileID)
	if err != nil {
		return -1, commonpb.ErrorParamInvalid("Failed to parse file id: %w", err)
	}
	return id, nil
}

func (s *WopiService) ValidateViewerSession(ctx context.Context, fileId int, token ...string) (context.Context, error) {
	if len(token) != 2 {
		return nil, commonpb.ErrorForbidden("malformed access token")
	}

	sessionRaw, exist := s.kv.Get(manager.ViewerSessionCachePrefix + token[0])
	if !exist {
		return nil, commonpb.ErrorForbidden("invalid access token")
	}

	session := sessionRaw.(manager.ViewerSessionCache)
	user, err := rpc.GetUserInfo(ctx, session.UserID, s.uc)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, trans.UserCtx{}, user)

	if fileId != session.FileID {
		return nil, commonpb.ErrorForbidden("invalid file")
	}

	viewers := s.settings.FileViewers(ctx)
	var v *ftypes.Viewer
	for _, group := range viewers {
		for _, viewer := range group.Viewers {
			if viewer.ID == session.ViewerID && !viewer.Disabled {
				v = &viewer
				break
			}
		}

		if v != nil {
			break
		}
	}

	if v == nil {
		return nil, commonpb.ErrorInternalSetting("viewer not found")
	}

	ctx = context.WithValue(ctx, manager.ViewerCtx{}, v)
	ctx = context.WithValue(ctx, manager.ViewerSessionCacheCtx{}, &session)
	return ctx, nil
}

func (s *WopiService) RefreshLock(ctx context.Context) error {
	uri, m, _, _, err := s.prepareFs(ctx)
	if err != nil {
		return err
	}

	l := s.dep.Logger()
	file, err := m.Get(ctx, uri, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityUploadFile), dbfs.WithNotRoot())
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}
	// Get lock token from request header
	reqHeader, respHeader, err := utils.HeaderFromContext(ctx)
	if err != nil {
		return err
	}
	lockToken := reqHeader.Get(wopi.LockTokenHeader)
	release, _, err := m.ConfirmLock(ctx, file, file.Uri(false), lockToken)
	if err != nil {
		l.WithContext(ctx).Debugf("WOPI refresh lock, not locked or not match: %w", err)
		respHeader.Set(wopi.LockTokenHeader, "")
		return nil
	}

	release()
	_, err = m.Refresh(ctx, wopi.LockDuration, lockToken)
	if err != nil {
		return err
	}

	respHeader.Set(wopi.LockTokenHeader, lockToken)
	return nil
}

func (s *WopiService) Unlock(ctx context.Context) error {
	_, m, _, _, err := s.prepareFs(ctx)
	if err != nil {
		return err
	}

	l := s.dep.Logger()

	// Get lock token from request header
	reqHeader, respHeader, err := utils.HeaderFromContext(ctx)
	if err != nil {
		return err
	}
	lockToken := reqHeader.Get(wopi.LockTokenHeader)
	if err = m.Unlock(ctx, lockToken); err != nil {
		l.WithContext(ctx).Debugf("WOPI unlock, not locked or not match: %w", err)
		respHeader.Set(wopi.LockTokenHeader, "")
		return nil
	}

	return nil
}
