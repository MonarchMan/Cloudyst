package interrupt

import (
	"common/cache"
	"context"
	"fmt"
)

type CheckPointStore struct {
	store cache.Driver
	ttl   int
}

const (
	CheckPointKeyPrefix = "check_point:"
	CheckPointTTL       = 86400
)

func NewCheckPointStore(store cache.Driver) *CheckPointStore {
	return &CheckPointStore{
		store: store,
		ttl:   CheckPointTTL,
	}
}

func (s *CheckPointStore) Set(ctx context.Context, key string, value []byte) (err error) {
	return s.store.Set(CheckPointKeyPrefix+key, value, s.ttl)
}

func (s *CheckPointStore) Get(ctx context.Context, key string) (value []byte, existed bool, err error) {
	val, existed := s.store.Get(CheckPointKeyPrefix + key)
	if !existed {
		return nil, false, nil
	}

	data, ok := val.([]byte)
	if !ok {
		return nil, true, fmt.Errorf("checkpoint %q has unexpected type %T", key, val)
	}
	return data, true, nil
}
