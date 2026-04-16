package service

import (
	commonpb "api/api/common/v1"
	"api/external/trans"
	"context"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"fmt"

	pb "api/api/file/callback/v1"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/types/known/emptypb"
)

type CallbackService struct {
	pb.UnimplementedCallbackServer
	dep     filemanager.ManagerDep
	dbfsDep filemanager.DbfsDep
	l       *log.Helper
}

func NewCallbackService(dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep, logger log.Logger) *CallbackService {
	return &CallbackService{
		dep:     dep,
		dbfsDep: dbfsDep,
		l:       log.NewHelper(logger, log.WithMessageKey("service-callback")),
	}
}

func (s *CallbackService) RemoteCallback(ctx context.Context, req *pb.CallbackRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.processCallback(ctx)
}
func (s *CallbackService) OssCallback(ctx khttp.Context) error {
	var req *pb.OssCallbackRequest
	if err := ctx.BindVars(&req); err != nil {
		return fmt.Errorf("failed to bind request body: %w", err)
	}

	uploadSession := ctx.Value(manager.UploadSessionCtx).(*fs.UploadSession)
	if uploadSession.Props.Size != req.Upload.Size {
		s.l.WithContext(ctx).Errorf("Callback validate failed: size mismatch, expected: %d, actual:%d", uploadSession.Props.Size, req.Upload.Size)
		return commonpb.ErrorForbidden("size mismatch")
	}
	return s.processCallback(ctx)
}
func (s *CallbackService) UpyunCallback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}
func (s *CallbackService) OnedriveCallback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}
func (s *CallbackService) GdriveCallback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}
func (s *CallbackService) CosCallback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}
func (s *CallbackService) AwsS3Callback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}
func (s *CallbackService) Ks3Callback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}
func (s *CallbackService) ObsCallback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}
func (s *CallbackService) QiniuCallback(ctx khttp.Context) error {
	return s.processCallback(ctx)
}

func (s *CallbackService) processCallback(ctx context.Context) error {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	uploadSession := ctx.Value(manager.UploadSessionCtx).(*fs.UploadSession)
	if _, err := m.CompleteUpload(ctx, uploadSession); err != nil {
		return fmt.Errorf("failed to complete upload: %w", err)
	}

	return nil
}
