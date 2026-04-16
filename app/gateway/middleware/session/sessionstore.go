package session

import (
	"common/cache"
	"net/http"

	"github.com/gorilla/sessions"
)

// Store 会话存储接口
type Store interface {
	Get(r *http.Request, name string) (*sessions.Session, error)
	New(r *http.Request, name string) (*sessions.Session, error)
	Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error
	SetOptions(options *sessions.Options)
	// SaveWithHeader 新增：使用 Header 保存会话的方法
	SaveWithHeader(r *http.Request, header http.Header, session *sessions.Session) error
}

// store 实现Store接口
type store struct {
	*kvStore
}

// NewStore 创建新的会话存储
func NewStore(driver cache.Driver, keyPairs ...[]byte) Store {
	return &store{kvStore: newKvStore("cd_session_", driver, keyPairs...)}
}
