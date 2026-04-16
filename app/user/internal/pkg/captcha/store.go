package captcha

import (
	"common/cache"
	"time"

	"github.com/mojocn/base64Captcha"
)

type CaptchaStore struct {
	store  cache.Driver
	prefix string
	expire time.Duration
}

func NewRedisStore(store cache.Driver, prefix string, expire time.Duration) base64Captcha.Store {
	return &CaptchaStore{
		store:  store,
		prefix: prefix,
		expire: expire,
	}
}

func (s *CaptchaStore) Set(id string, value string) error {
	key := s.prefix + id
	return s.store.Set(key, value, int(s.expire.Seconds()))
}

func (s *CaptchaStore) Get(id string, clear bool) string {
	key := s.prefix + id
	val, ok := s.store.Get(key)
	if !ok {
		return ""
	}
	if clear {
		_ = s.store.Delete(key)
	}
	return val.(string)
}

func (s *CaptchaStore) Verify(id, answer string, clear bool) bool {
	val := s.Get(id, clear)
	return val == answer
}
