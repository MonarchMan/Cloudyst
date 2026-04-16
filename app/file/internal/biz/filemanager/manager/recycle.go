package manager

import (
	pb "api/api/file/common/v1"
	userpb "api/api/user/common/v1"
	"api/external/trans"
	"common/cache"
	"common/serializer"
	"common/util"
	"context"
	"encoding/json"
	"errors"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/crontab"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/data"
	"fmt"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
)

type (
	ExplicitEntityRecycleTask struct {
		*queue.DBTask
	}

	ExplicitEntityRecycleTaskState struct {
		EntityIDs []int            `json:"entity_ids,omitempty"`
		Errors    [][]RecycleError `json:"errors,omitempty"`
	}

	RecycleError struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
)

func init() {
	queue.RegisterResumableTaskFactory(queue.ExplicitEntityRecycleTaskType, NewExplicitEntityRecycleTaskFromModel)
	queue.RegisterResumableTaskFactory(queue.EntityRecycleRoutineTaskType, NewEntityRecycleRoutineTaskFromModel)
	crontab.Register(setting.CronTypeEntityCollect, func(ctx context.Context, q queue.Queue) {
		h := log.NewHelper(log.GetLogger())
		t, err := NewEntityRecycleRoutineTask(ctx)
		if err != nil {
			h.Errorf("Failed to create entity recycle routine task: %s", err)
		}

		if err := q.QueueTask(ctx, t); err != nil {
			h.Errorf("Failed to queue entity recycle routine task: %s", err)
		}
	})
	crontab.Register(setting.CronTypeTrashBinCollect, CronCollectTrashBin)
}

func NewExplicitEntityRecycleTaskFromModel(task *ent.Task) queue.Task {
	return &ExplicitEntityRecycleTask{
		DBTask: &queue.DBTask{
			Task: task,
		},
	}
}

func newExplicitEntityRecycleTask(ctx context.Context, entities []int) (*ExplicitEntityRecycleTask, error) {
	user := trans.FromContext(ctx)
	state := &ExplicitEntityRecycleTaskState{
		EntityIDs: entities,
		Errors:    make([][]RecycleError, 0),
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	t := &ExplicitEntityRecycleTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:         queue.ExplicitEntityRecycleTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState: &pb.TaskPublicState{
					ResumeTime: time.Now().Unix() - 1,
				},
			},
			DirectOwner: user,
		},
	}
	return t, nil
}

func (m *ExplicitEntityRecycleTask) Do(ctx context.Context) (task.Status, error) {
	user := trans.FromContext(ctx)
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	fm := NewFileManager(dep, dbfsDep, user).(*manager)

	// unmarshal state
	state := &ExplicitEntityRecycleTaskState{}
	if err := json.Unmarshal([]byte(m.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// recycle entities
	err := fm.RecycleEntities(ctx, false, state.EntityIDs...)
	if err != nil {
		appendAe(&state.Errors, err)
		privateState, err := json.Marshal(state)
		if err != nil {
			return task.StatusError, fmt.Errorf("failed to marshal state: %w", err)
		}
		m.Task.PrivateState = string(privateState)
		return task.StatusError, err
	}

	return task.StatusCompleted, nil
}

type (
	EntityRecycleRoutineTask struct {
		*queue.DBTask
	}

	EntityRecycleRoutineTaskState struct {
		Errors [][]RecycleError `json:"errors,omitempty"`
	}
)

func NewEntityRecycleRoutineTaskFromModel(task *ent.Task) queue.Task {
	return &EntityRecycleRoutineTask{
		DBTask: &queue.DBTask{
			Task: task,
		},
	}
}

func NewEntityRecycleRoutineTask(ctx context.Context) (queue.Task, error) {
	user := trans.FromContext(ctx)
	state := &EntityRecycleRoutineTaskState{
		Errors: make([][]RecycleError, 0),
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	t := &EntityRecycleRoutineTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:         queue.EntityRecycleRoutineTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState: &pb.TaskPublicState{
					ResumeTime: time.Now().Unix() - 1,
				},
			},
			DirectOwner: user,
		},
	}
	return t, nil
}

func (m *EntityRecycleRoutineTask) Do(ctx context.Context) (task.Status, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	user := trans.FromContext(ctx)
	fm := NewFileManager(dep, dbfsDep, user).(*manager)

	// unmarshal state
	state := &EntityRecycleRoutineTaskState{}
	if err := json.Unmarshal([]byte(m.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// recycle entities
	err := fm.RecycleEntities(ctx, false)
	if err != nil {
		appendAe(&state.Errors, err)

		privateState, err := json.Marshal(state)
		if err != nil {
			return task.StatusError, fmt.Errorf("failed to marshal state: %w", err)
		}
		m.Task.PrivateState = string(privateState)
		return task.StatusError, err
	}

	return task.StatusCompleted, nil
}

// RecycleEntities delete given entities. If the ID list is empty, it will walk through
// all stale entities in DB.
func (m *manager) RecycleEntities(ctx context.Context, force bool, entityIDs ...int) error {
	ae := serializer.NewAggregateError()
	entities, err := m.fs.StaleEntities(ctx, entityIDs...)
	if err != nil {
		return fmt.Errorf("failed to get entities: %w", err)
	}

	// Group entities by policy ID
	entityGroup := lo.GroupBy(entities, func(entity fs.Entity) int {
		return entity.PolicyID()
	})

	// Delete entity in each group in batch
	for _, entities := range entityGroup {
		entityChunk := lo.Chunk(entities, 100)
		m.l.WithContext(ctx).Infof("Recycling %d entities in %d batches", len(entities), len(entityChunk))

		for batch, chunk := range entityChunk {
			m.l.WithContext(ctx).Infof("Start to recycle batch #%d, %d entities", batch, len(chunk))
			mapSrcToId := make(map[string]int, len(chunk))
			_, d, err := m.getEntityPolicyDriver(ctx, chunk[0], nil)
			if err != nil {
				for _, entity := range chunk {
					ae.Add(strconv.Itoa(entity.ID()), err)
				}
				continue
			}

			for _, entity := range chunk {
				mapSrcToId[entity.Source()] = entity.ID()
			}

			toBeDeletedSrc := lo.Map(lo.Filter(chunk, func(item fs.Entity, index int) bool {
				// Only delete entities that are not marked as "unlink only"
				return item.Model().Props == nil || !item.Model().Props.UnlinkOnly
			}), func(entity fs.Entity, index int) string {
				return entity.Source()
			})
			if len(toBeDeletedSrc) > 0 {
				res, err := d.Delete(ctx, toBeDeletedSrc...)
				if err != nil {
					for _, src := range res {
						ae.Add(strconv.Itoa(mapSrcToId[src]), err)
					}
				}
			}

			// Delete upload session if it's still valid
			for _, entity := range chunk {
				sid := entity.UploadSessionID()
				if sid == nil {
					continue
				}

				if session, ok := m.kv.Get(UploadSessionCachePrefix + sid.String()); ok {
					session := session.(fs.UploadSession)
					if err := d.CancelToken(ctx, &session); err != nil {
						m.l.WithContext(ctx).Warnf("Failed to cancel upload session for %q: %s, this is expected if it's remote policy.", session.Props.Uri.String(), err)
					}
					_ = m.kv.Delete(UploadSessionCachePrefix, sid.String())
				}
			}

			// Filtering out entities that are successfully deleted
			rawAe := ae.Raw()
			successEntities := lo.FilterMap(chunk, func(entity fs.Entity, index int) (int, bool) {
				entityIdStr := fmt.Sprintf("%d", entity.ID())
				_, ok := rawAe[entityIdStr]
				if !ok {
					// No error, deleted
					return entity.ID(), true
				}

				if force {
					ae.Remove(entityIdStr)
				}
				return entity.ID(), force
			})

			// Remove entities from DB
			fc, tx, ctx, err := data.WithTx(ctx, m.fc)
			if err != nil {
				return fmt.Errorf("failed to start transaction: %w", err)
			}
			storageReduced, err := fc.RemoveEntitiesByID(ctx, successEntities...)
			if err != nil {
				_ = data.Rollback(tx)
				return fmt.Errorf("failed to remove entities from DB: %w", err)
			}

			tx.AppendStorageDiff(storageReduced)
			if err := data.CommitWithStorageDiff(ctx, tx, m.l, m.uc); err != nil {
				return fmt.Errorf("failed to commit delete change: %w", err)
			}

		}
	}

	return ae.Aggregate()
}

const (
	MinimumTrashCollectBatch = 1000
)

// CronCollectTrashBin walks through all files in trash bin and delete them if they are expired.
func CronCollectTrashBin(ctx context.Context, q queue.Queue) {
	user := trans.FromContext(ctx)
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	l := dep.Logger()

	kv := dep.KV()
	if memKv, ok := kv.(*cache.MemoStore); ok {
		memKv.GarbageCollect(l.Logger())
	}

	fm := NewFileManager(dep, dbfsDep, user).(*manager)
	pageSize := dep.SettingProvider().DBFS(ctx).MaxPageSize
	batch := 0
	expiredFiles := make([]fs.File, 0)
	for {
		res, err := fm.fs.AllFilesInTrashBin(ctx, fs.WithPageSize(pageSize))
		if err != nil {
			l.WithContext(ctx).Errorf("Failed to get files in trash bin: %s", err)
			return
		}

		expired := lo.Filter(res.Files, func(file fs.File, index int) bool {
			if expire, ok := file.Metadata()[dbfs.MetadataExpectedCollectTime]; ok {
				expireUnix, err := strconv.ParseInt(expire, 10, 64)
				if err != nil {
					l.WithContext(ctx).Warnf("Failed to parse expected collect time %q: %s, will treat as expired", expire, err)
				}

				if expireUnix < time.Now().Unix() {
					return true
				}
			}

			return false
		})
		l.WithContext(ctx).Infof("Found %d files in trash bin pending collect, in batch #%d", len(res.Files), batch)

		expiredFiles = append(expiredFiles, expired...)
		if len(expiredFiles) >= MinimumTrashCollectBatch {
			collectTrashBin(ctx, expiredFiles, dep, dbfsDep, l)
			expiredFiles = expiredFiles[:0]
		}

		if res.Pagination.NextPageToken == "" {
			if len(expiredFiles) > 0 {
				collectTrashBin(ctx, expiredFiles, dep, dbfsDep, l)
			}
			break
		}

		batch++
	}
}

func collectTrashBin(ctx context.Context, files []fs.File, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep,
	l *log.Helper) {
	l.WithContext(ctx).Infof("Start to collect %d files in trash bin", len(files))

	// Group files by Owners
	fileGroup := lo.GroupBy(files, func(file fs.File) int {
		return file.OwnerID()
	})

	for uid, expiredFiles := range fileGroup {
		// Create a new file manager for user with only uid to delete files
		fm := NewFileManager(dep, dbfsDep, &userpb.User{Id: int64(uid)}).(*manager)
		if err := fm.Delete(ctx, lo.Map(expiredFiles, func(file fs.File, index int) *fs.URI {
			return file.Uri(false)
		}), fs.WithSkipSoftDelete(true)); err != nil {
			l.WithContext(ctx).Errorf("Failed to delete files for users %d: %s", uid, err)
		}
	}
}

func appendAe(errs *[][]RecycleError, err error) {
	var ae *serializer.AggregateError
	*errs = append(*errs, make([]RecycleError, 0))
	if errors.As(err, &ae) {
		(*errs)[len(*errs)-1] = lo.MapToSlice(ae.Raw(), func(key string, value error) RecycleError {
			return RecycleError{
				ID:    key,
				Error: value.Error(),
			}
		})
	}
}
