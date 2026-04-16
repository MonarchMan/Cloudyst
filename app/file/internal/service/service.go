package service

import (
	"file/internal/service/admin"
	"file/internal/service/webdav"

	"github.com/google/wire"
)

var MasterProviderSet = wire.NewSet(
	NewCallbackService,
	NewFileService,
	NewShareService,
	NewWopiService,
	NewWorkflowService,
	NewSysService,
	admin.NewAdminService,
	webdav.NewWebDAVService,
)

var SalveProviderSet = wire.NewSet(NewSlaveService)
