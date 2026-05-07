package app

import (
	"ai/internal/biz/queue"
	"ai/internal/conf"
	"common/cache"
	"common/constants"
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	Server interface {
		Start() error
		PrintBanner()
		Close()
	}

	server struct {
		logger *log.Helper
		config *conf.Bootstrap
		kv     cache.Driver
		qm     *queue.QueueManager
	}
)

func NewServer(logger log.Logger, conf *conf.Bootstrap, kv cache.Driver, qm *queue.QueueManager) (Server, func()) {
	s := &server{
		logger: log.NewHelper(logger, log.WithMessageKey("app-server")),
		config: conf,
		kv:     kv,
		qm:     qm,
	}
	return s, s.Close
}

func (s *server) Start() error {
	ctx := context.Background()
	s.qm.ReloadIngestQueue(ctx)
	s.qm.IngestQueue().Start()
	s.qm.ReloadReindexQueue(ctx)
	s.qm.ReindexQueue().Start()

	return nil
}

func (s *server) PrintBanner() {
	fmt.Print(`
   	___ _                _                    
  / __\ | ___  _   _  __| |__ __ _____ _____           ____     _____
 / /  | |/ _ \| | | |/ _  | |_| |_  __|_____|         / __ \   |_   _|
/ /___| | (_) | |_| | (_| |     |_\ \_  | |	 ------- / /__\ \   _| |_
\____/|_|\___/ \__,_|\__,_|\_  /|_____| |_|         /_/    \_\ |_____|
                            / /
                           /_/

   V` + constants.BackendVersion + `
================================================

`)
}

func (s *server) Close() {

}
