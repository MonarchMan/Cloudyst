package credmanager

import (
	pbslave "api/api/file/slave/v1"
	"common/auth"
	"common/cache"
	"common/request"
	"context"
	"encoding/gob"
	"errors"
	"file/internal/biz/cluster"
	"file/internal/biz/cluster/routes"
	"file/internal/conf"
	"file/internal/data"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	// CredManager is a centralized for all Oauth tokens that requires periodic refresh
	// It is primarily used by OneDrive storage policy.
	CredManager interface {
		// Obtain gets a credential from the manager, refresh it if it's expired
		Obtain(ctx context.Context, key string) (Credential, error)
		// Upsert inserts or updates a credential in the manager
		Upsert(ctx context.Context, cred ...Credential) error
		RefreshAll(ctx context.Context)
	}

	Credential interface {
		String() string
		Refresh(ctx context.Context, pc data.StoragePolicyClient, config *conf.Bootstrap, l *log.Helper) (Credential, error)
		Key() string
		Expiry() time.Time
		RefreshedAt() *time.Time
	}
)

func init() {
	gob.Register(CredentialResponse{})
}

func New(kv cache.Driver, pc data.StoragePolicyClient, bs *conf.Bootstrap, l log.Logger) CredManager {
	return &credManager{
		kv:    kv,
		locks: make(map[string]*sync.Mutex),
		pc:    pc,
		bs:    bs,
		l:     log.NewHelper(l, log.WithMessageKey("biz-credManager")),
	}
}

type (
	credManager struct {
		kv cache.Driver
		mu sync.RWMutex
		pc data.StoragePolicyClient
		bs *conf.Bootstrap
		l  *log.Helper

		locks map[string]*sync.Mutex
	}
)

var (
	ErrorNotFound = errors.New("credential not found")
)

func (m *credManager) Upsert(ctx context.Context, cred ...Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	h := log.NewHelper(m.l.Logger())
	for _, c := range cred {
		h.Info("CredManager: Upsert credential for key %q...", c.Key())
		if err := m.kv.Set(c.Key(), c, 0); err != nil {
			return fmt.Errorf("failed to update credential in KV for key %q: %w", c.Key(), err)
		}

		if _, ok := m.locks[c.Key()]; !ok {
			m.locks[c.Key()] = &sync.Mutex{}
		}
	}

	return nil
}

func (m *credManager) Obtain(ctx context.Context, key string) (Credential, error) {
	m.mu.RLock()
	itemRaw, ok := m.kv.Get(key)
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("credential not found for key %q: %w", key, ErrorNotFound)
	}

	h := log.NewHelper(m.l.Logger())

	item := itemRaw.(Credential)
	if _, ok := m.locks[key]; !ok {
		m.locks[key] = &sync.Mutex{}
	}
	m.locks[key].Lock()
	defer m.locks[key].Unlock()
	m.mu.RUnlock()

	if item.Expiry().After(time.Now()) {
		// Credential is still valid
		return item, nil
	}

	// Credential is expired, refresh it
	h.Info("Refreshing credential for key %q...", key)
	newCred, err := item.Refresh(ctx, m.pc, m.bs, m.l)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh credential for key %q: %w", key, err)
	}

	h.Info("New credential for key %q is obtained, expire at %s", key, newCred.Expiry().String())
	if err := m.kv.Set(key, newCred, 0); err != nil {
		return nil, fmt.Errorf("failed to update credential in KV for key %q: %w", key, err)
	}

	return newCred, nil
}

func (m *credManager) RefreshAll(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	h := log.NewHelper(m.l.Logger())
	for key := range m.locks {
		h.Info("Refreshing credential for key %q...", key)
		m.locks[key].Lock()
		defer m.locks[key].Unlock()

		itemRaw, ok := m.kv.Get(key)
		if !ok {
			h.Warn("Credential not found for key %q", key)
			continue
		}

		item := itemRaw.(Credential)
		newCred, err := item.Refresh(ctx, m.pc, m.bs, m.l)
		if err != nil {
			h.Warn("Failed to refresh credential for key %q: %s", key, err)
			continue
		}

		h.Info("New credential for key %q is obtained, expire at %s", key, newCred.Expiry().String())
		if err := m.kv.Set(key, newCred, 0); err != nil {
			h.Warn("Failed to update credential in KV for key %q: %s", key, err)
		}
	}
}

type (
	slaveCredManager struct {
		kv     cache.Driver
		client request.Client
		l      *log.Helper
	}

	CredentialResponse struct {
		*pbslave.GetCredentialResponse
	}
)

func NewSlaveManager(kv cache.Driver, config *conf.Bootstrap, l log.Logger) CredManager {
	return &slaveCredManager{
		kv: kv,
		l:  log.NewHelper(l, log.WithMessageKey("biz-credManager")),
		client: request.NewClient(
			config.Server.Sys.Mode,
			request.WithCredential(auth.HMACAuth{
				[]byte(config.Slave.Secret),
			}, int64(config.Slave.SignatureTtl)),
		),
	}
}

func (c CredentialResponse) String() string {
	return c.Token
}

func (c CredentialResponse) Refresh(ctx context.Context, pc data.StoragePolicyClient, config *conf.Bootstrap, l *log.Helper) (Credential, error) {
	return c, nil
}

func (c CredentialResponse) Key() string {
	return ""
}

func (c CredentialResponse) Expiry() time.Time {
	return c.ExpireAt.AsTime()
}

func (c CredentialResponse) RefreshedAt() *time.Time {
	return nil
}

func (m *slaveCredManager) Upsert(ctx context.Context, cred ...Credential) error {
	return nil
}

func (m *slaveCredManager) Obtain(ctx context.Context, key string) (Credential, error) {
	itemRaw, ok := m.kv.Get(key)
	if !ok {
		return m.requestCredFromMaster(ctx, key)
	}

	return itemRaw.(Credential), nil
}

// No op on slave node
func (m *slaveCredManager) RefreshAll(ctx context.Context) {}

func (m *slaveCredManager) requestCredFromMaster(ctx context.Context, key string) (Credential, error) {
	m.l.Info("SlaveCredManager: Requesting credential for key %q from master...", key)

	requestDst := routes.MasterGetCredentialUrl(cluster.MasterSiteUrlFromContext(ctx), key)
	resp, err := m.client.Request(
		http.MethodGet,
		requestDst.String(),
		nil,
		request.WithContext(ctx),
		request.WithLogger(m.l.Logger()),
		request.WithSlaveMeta(cluster.NodeIdFromContext(ctx)),
		request.WithCorrelationID(),
	).CheckHTTPResponse(http.StatusOK).DecodeResponse()
	if err != nil {
		return nil, fmt.Errorf("failed to request credential from master: %w", err)
	}

	cred := &CredentialResponse{}
	resp.GobDecode(&cred)

	if err := m.kv.Set(key, *cred, max(int(time.Until(cred.Expiry()).Seconds()), 1)); err != nil {
		return nil, fmt.Errorf("failed to update credential in KV for key %q: %w", key, err)
	}

	return cred, nil
}
