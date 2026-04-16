package manager

import (
	pb "api/api/file/common/v1"
	pbfile "api/api/file/files/v1"
	pbslave "api/api/file/slave/v1"
	userpb "api/api/user/common/v1"
	"api/external/trans"
	"common/util"
	"context"
	"encoding/json"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/driver"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/queue"
	"file/internal/data/types"
	"fmt"

	"github.com/samber/lo"
)

type (
	MediaMetaTask struct {
		*queue.DBTask
	}

	MediaMetaTaskState struct {
		Uri      *fs.URI `json:"uri"`
		EntityID int     `json:"entity_id"`
	}
)

func init() {
	queue.RegisterResumableTaskFactory(queue.MediaMetaTaskType, NewMediaMetaTaskFromModel)
}

// NewMediaMetaTask creates a new MediaMetaTask to
func NewMediaMetaTask(ctx context.Context, uri *fs.URI, entityID int, creator *userpb.User) (*MediaMetaTask, error) {
	state := &MediaMetaTaskState{
		Uri:      uri,
		EntityID: entityID,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &MediaMetaTask{
		DBTask: &queue.DBTask{
			DirectOwner: creator,
			Task: &ent.Task{
				Type:         queue.MediaMetaTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &pb.TaskPublicState{},
			},
		},
	}, nil
}

func NewMediaMetaTaskFromModel(task *ent.Task) queue.Task {
	return &MediaMetaTask{
		DBTask: &queue.DBTask{
			Task: task,
		},
	}
}

func (m *MediaMetaTask) Do(ctx context.Context) (task.Status, error) {
	user := trans.FromContext(ctx)
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	fm := NewFileManager(dep, dbfsDep, user).(*manager)

	// unmarshal state
	var state MediaMetaTaskState
	if err := json.Unmarshal([]byte(m.State()), &state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, queue.CriticalErr)
	}

	err := fm.ExtractAndSaveMediaMeta(ctx, state.Uri, state.EntityID)
	if err != nil {
		return task.StatusError, err
	}

	return task.StatusCompleted, nil
}

func (m *manager) ExtractAndSaveMediaMeta(ctx context.Context, uri *fs.URI, entityID int) error {
	// 1. retrieve files info
	file, err := m.fs.Get(ctx, uri, dbfs.WithFileEntities())
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}

	versions := lo.Filter(file.Entities(), func(i fs.Entity, index int) bool {
		return i.Type() == types.EntityTypeVersion
	})
	targetVersion, versionIndex, found := lo.FindIndexOf(versions, func(i fs.Entity) bool {
		return i.ID() == entityID
	})
	if !found {
		return fmt.Errorf("failed to find version: %s (%w)", err, queue.CriticalErr)
	}

	if versionIndex != 0 {
		m.l.WithContext(ctx).Debugf("Skip media meta task for non-latest version.")
		return nil
	}

	var (
		metas []pbslave.MediaMeta
	)
	// 2. try using native driver
	_, d, err := m.getEntityPolicyDriver(ctx, targetVersion, nil)
	if err != nil {
		return fmt.Errorf("failed to get storage driver: %s (%w)", err, queue.CriticalErr)
	}
	driverCaps := d.Capabilities()
	if util.IsInExtensionList(driverCaps.MediaMetaSupportedExts, file.Name()) {
		m.l.WithContext(ctx).Debugf("Using native driver to generate media meta.")
		metas, err = d.MediaMeta(ctx, targetVersion.Source(), file.Ext())
		if err != nil {
			return fmt.Errorf("failed to get media meta using native driver: %w", err)
		}
	} else if driverCaps.MediaMetaProxy && util.IsInExtensionList(m.extractor.GetMediaMetaExtractor().Exts(), file.Name()) {
		m.l.WithContext(ctx).Debugf("Using local extractor to generate media meta.")
		extractor := m.extractor.GetMediaMetaExtractor()
		source, err := m.GetEntitySource(ctx, targetVersion.ID())
		defer source.Close()
		if err != nil {
			return fmt.Errorf("failed to get entity source: %w", err)
		}

		metas, err = extractor.Extract(ctx, file.Ext(), source)
		if err != nil {
			return fmt.Errorf("failed to extract media meta using local extractor: %w", err)
		}

	} else {
		m.l.WithContext(ctx).Debugf("No available generator for media meta.")
		return nil
	}

	m.l.WithContext(ctx).Debugf("%d media meta generated.", len(metas))
	m.l.WithContext(ctx).Debugf("Media meta: %v", metas)
	patches := make([]*pbfile.MetadataPatch, len(metas))
	for i := range metas {
		patches[i] = &pbfile.MetadataPatch{
			Key:   fmt.Sprintf("%s:%s", metas[i].Type, metas[i].Key),
			Value: metas[i].Value,
		}
	}
	// 3. save meta
	if len(metas) > 0 {
		if err := m.fs.PatchMetadata(ctx, []*fs.URI{uri}, patches...); err != nil {
			return fmt.Errorf("failed to save media meta: %s (%w)", err, queue.CriticalErr)
		}
	}

	return nil
}

func (m *manager) shouldGenerateMediaMeta(ctx context.Context, d driver.Handler, fileName string) bool {
	driverCaps := d.Capabilities()
	if util.IsInExtensionList(driverCaps.MediaMetaSupportedExts, fileName) {
		// Handler support it natively
		return true
	}

	if driverCaps.MediaMetaProxy && util.IsInExtensionList(m.extractor.GetMediaMetaExtractor().Exts(), fileName) {
		// Handler does not support. but proxy is enabled.
		return true
	}

	return false
}

func (m *manager) mediaMetaForNewEntity(ctx context.Context, session *fs.UploadSession, d driver.Handler) {
	if session.Props.EntityType == nil || *session.Props.EntityType == types.EntityTypeVersion {
		if !m.shouldGenerateMediaMeta(ctx, d, session.Props.Uri.Name()) {
			return
		}

		mediaMetaTask, err := NewMediaMetaTask(ctx, session.Props.Uri, session.EntityID, m.user)
		if err != nil {
			m.l.WithContext(ctx).Warnf("Failed to create media meta task: %s", err)
			return
		}
		if err := m.qm.GetMediaMetaQueue().QueueTask(ctx, mediaMetaTask); err != nil {
			m.l.WithContext(ctx).Warnf("Failed to queue media meta task: %s", err)
		}
		return
	}
}
