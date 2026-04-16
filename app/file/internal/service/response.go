package service

import (
	pbfile "api/api/file/files/v1"
	pbshare "api/api/file/share/v1"
	pbexplorer "api/api/file/workflow/v1"
	"common/auth"
	"common/boolset"
	"common/hashid"
	"context"
	"file/ent"
	"file/internal/biz/cluster/routes"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/data"
	"file/internal/pkg/utils"
	"net/url"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func buildListResponse(ctx context.Context, uid int64, parent fs.File, res *fs.ListFileResult, hasher hashid.Encoder,
	settings setting.Provider) *pbfile.ListFileResponse {
	r := &pbfile.ListFileResponse{
		Files: lo.Map(res.Files, func(f fs.File, index int) *pbfile.FileResponse {
			return buildFileResponse(ctx, uid, f, hasher, res.Props.Capability, settings)
		}),
		Pagination: res.Pagination,
		Props: &pbfile.NavigatorProps{
			Capability:            []byte(*res.Props.Capability),
			MaxPageSize:           int32(res.Props.MaxPageSize),
			OrderByOptions:        res.Props.OrderByOptions,
			OrderDirectionOptions: res.Props.OrderDirectionOptions,
		},
		ContextHint:           res.ContextHint.String(),
		RecursionLimitReached: res.RecursionLimitReached,
		MixedType:             res.MixedType,
		SingleFileView:        res.SingleFileView,
		StoragePolicy:         buildStoragePolicy(res.StoragePolicy, hasher),
		View:                  res.View,
	}

	if !res.Parent.IsNil() {
		r.Parent = buildFileResponse(ctx, uid, res.Parent, hasher, res.Props.Capability, settings)
	}

	return r
}

func buildFileResponse(ctx context.Context, uid int64, f fs.File, hasher hashid.Encoder, cap *boolset.BooleanSet,
	settings setting.Provider) *pbfile.FileResponse {
	var owner int64
	if f != nil {
		owner = int64(f.OwnerID())
	}

	if cap == nil {
		cap = f.Capabilities()
	}

	res := &pbfile.FileResponse{
		Type:          int32(f.Type()),
		Id:            hashid.EncodeFileID(hasher, f.ID()),
		Name:          f.DisplayName(),
		CreatedAt:     timestamppb.New(f.CreatedAt()),
		UpdatedAt:     timestamppb.New(f.UpdatedAt()),
		Size:          f.Size(),
		Metadata:      f.Metadata(),
		Path:          f.Uri(false).String(),
		Shared:        f.Shared(),
		Capability:    []byte(*cap),
		Owned:         owner == 0 || owner == uid,
		FolderSummary: f.FolderSummary(),
		ExtendedInfo:  buildExtendedInfo(ctx, uid, f, hasher, settings),
		PrimaryEntity: hashid.EncodeEntityID(hasher, f.PrimaryEntityID()),
	}
	return res
}

func buildExtendedInfo(ctx context.Context, uid int64, f fs.File, hasher hashid.Encoder, settings setting.Provider) *pbfile.ExtendedInfo {
	extendedInfo := f.ExtendedInfo()
	if extendedInfo == nil {
		return nil
	}

	base := settings.SiteURL(ctx)

	ext := &pbfile.ExtendedInfo{
		StoragePolicy: buildStoragePolicy(extendedInfo.StoragePolicy, hasher),
		StorageUsed:   extendedInfo.StorageUsed,
		Entities: lo.Map(f.Entities(), func(e fs.Entity, index int) *pbfile.EntityResponse {
			return buildEntity(extendedInfo, e, hasher)
		}),
		DirectLinks: lo.Map(extendedInfo.DirectLinks, func(d *ent.DirectLink, index int) *pbfile.DirectLink {
			return buildDirectLink(d, hasher, base)
		}),
	}

	if int(uid) == f.OwnerID() {
		// Only owner can see the shares settings.
		ext.Shares = lo.Map(extendedInfo.Shares, func(s *ent.Share, index int) *pbshare.GetShareResponse {
			return buildShare(s, base, hasher, uid, f.Owner().Id, f.DisplayName(), f.Type(), true, false)
		})
		ext.View = utils.ToProtoView(extendedInfo.View)
	}

	return ext
}

func buildShare(s *ent.Share, base *url.URL, hasher hashid.Encoder, requester int64, userId int64, name string, fileType int,
	unlocked bool, expired bool) *pbshare.GetShareResponse {
	res := pbshare.GetShareResponse{
		Name:     name,
		Id:       hashid.EncodeShareID(hasher, s.ID),
		Unlocked: unlocked,
		//OwnerId:           int32(s.OwnerID),
		Expired:           data.IsShareExpired(s) != nil || expired,
		Url:               buildShareLink(s, hasher, base, unlocked),
		CreatedAt:         timestamppb.New(s.CreatedAt),
		Visited:           int32(s.Views),
		SourceType:        int32(fileType),
		PasswordProtected: s.Password != "",
		OwnerInfo:         s.OwnerInfo,
	}
	res.OwnerInfo.Id = hashid.EncodeUserID(hasher, s.OwnerID)

	if unlocked {
		if s.RemainDownloads != nil {
			res.RemainDownloads = int32(*s.RemainDownloads)
		}
		res.Downloaded = int32(s.Downloads)
		if s.Expires != nil {
			res.Expires = timestamppb.New(*s.Expires)
		}
		res.Password = s.Password
		res.ShowReadme = s.Props != nil && s.Props.ShowReadMe
	}

	if requester == userId {
		res.IsPrivate = s.Password != ""
		res.ShareView = s.Props != nil && s.Props.ShareView
	}

	return &res
}

func buildShareLink(s *ent.Share, hasher hashid.Encoder, base *url.URL, unlocked bool) string {
	shareId := hashid.EncodeShareID(hasher, s.ID)
	if unlocked {
		return routes.MasterShareUrl(base, shareId, s.Password).String()
	}
	return routes.MasterShareUrl(base, shareId, "").String()
}

func buildDirectLink(d *ent.DirectLink, hasher hashid.Encoder, base *url.URL) *pbfile.DirectLink {
	return &pbfile.DirectLink{
		Id:         hashid.EncodeSourceLinkID(hasher, d.ID),
		Url:        routes.MasterDirectLink(base, hashid.EncodeSourceLinkID(hasher, d.ID), d.Name).String(),
		Downloaded: int32(d.Downloads),
		CreatedAt:  timestamppb.New(d.CreatedAt),
	}
}

func buildEntity(extendedInfo *fs.FileExtendedInfo, e fs.Entity, hasher hashid.Encoder) *pbfile.EntityResponse {
	return &pbfile.EntityResponse{
		Id:            hashid.EncodeEntityID(hasher, e.ID()),
		Type:          int32(e.Type()),
		CreatedAt:     timestamppb.New(e.CreatedAt()),
		StoragePolicy: buildStoragePolicy(extendedInfo.EntityStoragePolicies[e.PolicyID()], hasher),
		Size:          e.Size(),
		CreateBy:      int32(e.CreatedBy()),
	}
}

func buildStoragePolicy(sp *ent.StoragePolicy, hasher hashid.Encoder) *pbfile.StoragePolicy {
	if sp == nil {
		return nil
	}

	res := &pbfile.StoragePolicy{
		Id:               hashid.EncodePolicyID(hasher, sp.ID),
		Name:             sp.Name,
		Type:             sp.Type,
		MaxSize:          sp.MaxSize,
		Relay:            sp.Settings.Relay,
		ChunkConcurrency: int32(sp.Settings.ChunkConcurrency),
	}

	if sp.Settings.IsFileTypeDenyList {
		res.DeniedSuffix = sp.Settings.FileType
	} else {
		res.AllowedSuffix = sp.Settings.FileType
	}

	if sp.Settings.NameRegexp != "" {
		if sp.Settings.IsNameRegexpDenyList {
			res.DeniedNameRegexp = sp.Settings.NameRegexp
		} else {
			res.AllowedNameRegexp = sp.Settings.NameRegexp
		}
	}

	return res
}

func buildUploadSessionResponse(session *fs.UploadCredential, hasher hashid.Encoder) *pbfile.UploadSessionResponse {
	return &pbfile.UploadSessionResponse{
		SessionId:      session.SessionID,
		ChunkSize:      session.ChunkSize,
		Expires:        session.Expires,
		UploadUrls:     session.UploadURLs,
		Credential:     session.Credential,
		CompleteUrl:    session.CompleteURL,
		Uri:            session.Uri,
		UploadId:       session.UploadID,
		StoragePolicy:  buildStoragePolicy(session.StoragePolicy, hasher),
		CallbackSecret: session.CallbackSecret,
		MimeType:       session.MimeType,
		UploadPolicy:   session.UploadPolicy,
	}
}

func buildDirectLinkResponse(links []manager.DirectLink) *pbfile.GetSourceResponse {
	if len(links) == 0 {
		return nil
	}

	var res []*pbfile.DirectLinkResponse
	for _, link := range links {
		res = append(res, &pbfile.DirectLinkResponse{
			Link:    link.Url,
			FileUrl: link.File.Uri(false).String(),
		})
	}
	return &pbfile.GetSourceResponse{
		Sources: res,
	}
}

func buildListShareResponse(res *data.ListShareResult, hasher hashid.Encoder, base *url.URL, requester int64,
	unlocked bool) *pbshare.ListSharesResponse {
	var infos []*pbshare.GetShareResponse
	for _, share := range res.Shares {
		expired := data.IsValidShare(share) != nil
		shareName := share.Edges.File.Name
		if share.Edges.File.FileParentID == 0 && len(share.Edges.File.Edges.Metadata) >= 0 {
			// 对于垃圾桶里的文件，读取 metadata 中的 name 字段
			restoreUri, found := lo.Find(share.Edges.File.Edges.Metadata, func(item *ent.Metadata) bool {
				return item.Name == dbfs.MetadataRestoreUri
			})
			if found {
				uri, err := fs.NewUriFromString(restoreUri.Value)
				if err == nil {
					shareName = uri.Name()
				}
			}
		}

		infos = append(infos, buildShare(share, base, hasher, requester, int64(share.OwnerID), shareName,
			share.Edges.File.Type, unlocked, expired))
	}

	return &pbshare.ListSharesResponse{
		Shares:     infos,
		Pagination: res.PaginationResults,
	}
}

func buildTaskResponse(task queue.Task, node *ent.Node, hasher hashid.Encoder) *pbexplorer.TaskResponse {
	model := task.Model()
	t := &pbexplorer.TaskResponse{
		Status:    string(task.Status()),
		CreatedAt: timestamppb.New(model.CreatedAt),
		UpdatedAt: timestamppb.New(model.UpdatedAt),
		Id:        hashid.EncodeTaskID(hasher, task.ID()),
		Type:      task.Type(),
		Summary:   task.Summarize(hasher),
		Error:     auth.RedactSensitiveValues(model.PublicState.Error),
		ErrorHistory: lo.Map(model.PublicState.ErrorHistory, func(s string, index int) string {
			return auth.RedactSensitiveValues(s)
		}),
		Duration:   int64(model.PublicState.ExecutedDuration.AsDuration()),
		ResumeTime: model.PublicState.ResumeTime,
		RetryCount: model.PublicState.RetryCount,
	}

	if node != nil {
		t.Node = buildNodeOption(node, hasher)
	}

	return t
}

func buildNodeOption(node *ent.Node, hasher hashid.Encoder) *pbexplorer.NodeOption {
	return &pbexplorer.NodeOption{
		Id:           hashid.EncodeNodeID(hasher, node.ID),
		Name:         node.Name,
		Type:         string(node.Type),
		Capabilities: *node.Capabilities,
	}
}

func buildTaskListResponse(tasks []queue.Task, res *data.ListTaskResult, nodeMap map[int]*ent.Node,
	hasher hashid.Encoder) *pbexplorer.ListTasksResponse {
	return &pbexplorer.ListTasksResponse{
		Pagination: res.PaginationResults,
		Tasks: lo.Map(tasks, func(t queue.Task, index int) *pbexplorer.TaskResponse {
			var (
				node *ent.Node
				s    = t.Summarize(hasher)
			)

			if s.NodeId > 0 {
				node = nodeMap[int(s.NodeId)]
			}
			return buildTaskResponse(t, node, hasher)
		}),
	}
}
