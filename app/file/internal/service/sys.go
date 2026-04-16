package service

import (
	"common/cache"
	"context"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/mediameta"
	"file/internal/biz/queue"

	pb "api/api/file/sys/v1"
)

type SysService struct {
	pb.UnimplementedSysServer
	qm             *queue.QueueManager
	d              mime.MimeManager
	em             mediameta.ExtractorStateManager
	kv             cache.Driver
	postprocessors map[string]SettingPostProcessor
}

func NewSysService(qm *queue.QueueManager, d mime.MimeManager, em mediameta.ExtractorStateManager) *SysService {
	return &SysService{
		qm: qm,
		d:  d,
		em: em,
		postprocessors: map[string]SettingPostProcessor{
			"mime_mapping":                               d.Reload,
			"media_meta_exif":                            em.Reload,
			"media_meta_music":                           em.Reload,
			"media_meta_ffprobe":                         em.Reload,
			"queue_media_meta_worker_num":                qm.ReloadMediaMetaQueue,
			"queue_media_meta_max_execution":             qm.ReloadMediaMetaQueue,
			"queue_media_meta_backoff_factor":            qm.ReloadMediaMetaQueue,
			"queue_media_meta_backoff_max_duration":      qm.ReloadMediaMetaQueue,
			"queue_media_meta_max_retry":                 qm.ReloadMediaMetaQueue,
			"queue_media_meta_retry_delay":               qm.ReloadMediaMetaQueue,
			"queue_thumb_worker_num":                     qm.ReloadThumbQueue,
			"queue_thumb_max_execution":                  qm.ReloadThumbQueue,
			"queue_thumb_backoff_factor":                 qm.ReloadThumbQueue,
			"queue_thumb_backoff_max_duration":           qm.ReloadThumbQueue,
			"queue_thumb_max_retry":                      qm.ReloadThumbQueue,
			"queue_thumb_retry_delay":                    qm.ReloadThumbQueue,
			"queue_recycle_worker_num":                   qm.ReloadEntityRecycleQueue,
			"queue_recycle_max_execution":                qm.ReloadEntityRecycleQueue,
			"queue_recycle_backoff_factor":               qm.ReloadEntityRecycleQueue,
			"queue_recycle_backoff_max_duration":         qm.ReloadEntityRecycleQueue,
			"queue_recycle_max_retry":                    qm.ReloadEntityRecycleQueue,
			"queue_recycle_retry_delay":                  qm.ReloadEntityRecycleQueue,
			"queue_io_intense_worker_num":                qm.ReloadIoIntenseQueue,
			"queue_io_intense_max_execution":             qm.ReloadIoIntenseQueue,
			"queue_io_intense_backoff_factor":            qm.ReloadIoIntenseQueue,
			"queue_io_intense_backoff_max_duration":      qm.ReloadIoIntenseQueue,
			"queue_io_intense_max_retry":                 qm.ReloadIoIntenseQueue,
			"queue_io_intense_retry_delay":               qm.ReloadIoIntenseQueue,
			"queue_remote_download_worker_num":           qm.ReloadRemoteDownloadQueue,
			"queue_remote_download_max_execution":        qm.ReloadRemoteDownloadQueue,
			"queue_remote_download_backoff_factor":       qm.ReloadRemoteDownloadQueue,
			"queue_remote_download_backoff_max_duration": qm.ReloadRemoteDownloadQueue,
			"queue_remote_download_max_retry":            qm.ReloadRemoteDownloadQueue,
			"queue_remote_download_retry_delay":          qm.ReloadRemoteDownloadQueue,
		},
	}
}

func (s *SysService) ReloadDependency(ctx context.Context, req *pb.ReloadDependencyRequest) (*pb.ReloadDependencyResponse, error) {
	successList := make([]bool, 0, len(req.Keys))
	for _, key := range req.Keys {
		if postprocessor, ok := s.postprocessors[key]; ok {
			if err := postprocessor(ctx); err == nil {
				successList = append(successList, true)
			}
		} else if key == "secret_key" {
			if err := s.kv.Delete(manager.EntityUrlCacheKeyPrefix); err == nil {
				successList = append(successList, true)
			}

		}
	}
	return &pb.ReloadDependencyResponse{SuccessList: successList}, nil
}

type SettingPostProcessor func(ctx context.Context) error
