package session

import (
	"bytes"
	"common/cache"
	"encoding/base32"
	"encoding/gob"
	"net/http"
	"strings"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

type kvStore struct {
	Codecs        []securecookie.Codec
	Options       *sessions.Options
	DefaultMaxAge int

	prefix     string
	serializer SessionSerializer
	store      cache.Driver
}

func newKvStore(prefix string, store cache.Driver, keyPairs ...[]byte) *kvStore {
	return &kvStore{
		prefix:        prefix,
		store:         store,
		DefaultMaxAge: 60 * 20,
		serializer:    GobSerializer{},
		Codecs:        securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: 86400 * 30,
		},
	}
}

// Get returns a session for the given name after adding it to the registry.
func (s *kvStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

// New returns a session for the given name without adding it to the registry.
func (s *kvStore) New(r *http.Request, name string) (*sessions.Session, error) {
	var (
		err error
	)
	session := sessions.NewSession(s, name)
	// make a copy
	options := *s.Options
	session.Options = &options
	session.IsNew = true
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if err == nil {
			res, ok := s.store.Get(s.prefix + session.ID)
			if ok {
				err = s.serializer.Deserialize(res.([]byte), session)
			}

			session.IsNew = !(err == nil && ok)
		}
	}
	return session, err
}

// Save 保存会话到 ResponseWriter（保留原方法以兼容）
func (s *kvStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	// Marked for deletion.
	if session.Options.MaxAge <= 0 {
		if err := s.store.Delete(s.prefix, session.ID); err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
	} else {
		// Build an alphanumeric key for the cache store.
		if session.ID == "" {
			session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
		}

		b, err := s.serializer.Serialize(session)
		if err != nil {
			return err
		}

		age := session.Options.MaxAge
		if age == 0 {
			age = s.DefaultMaxAge
		}

		if err := s.store.Set(s.prefix+session.ID, b, age); err != nil {
			return err
		}

		encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
		if err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	}
	return nil
}

// SaveWithHeader 使用 Header 保存会话（新方法）
func (s *kvStore) SaveWithHeader(r *http.Request, header http.Header, session *sessions.Session) error {
	// Marked for deletion.
	if session.Options.MaxAge <= 0 {
		if err := s.store.Delete(s.prefix, session.ID); err != nil {
			return err
		}
		// 直接设置 Set-Cookie header 来删除 cookie
		cookie := sessions.NewCookie(session.Name(), "", session.Options)
		addCookieToHeader(header, cookie)
	} else {
		// Build an alphanumeric key for the store.
		if session.ID == "" {
			session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
		}

		b, err := s.serializer.Serialize(session)
		if err != nil {
			return err
		}

		age := session.Options.MaxAge
		if age == 0 {
			age = s.DefaultMaxAge
		}

		if err := s.store.Set(s.prefix+session.ID, b, age); err != nil {
			return err
		}

		encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
		if err != nil {
			return err
		}

		// 直接设置 Set-Cookie header
		cookie := sessions.NewCookie(session.Name(), encoded, session.Options)
		addCookieToHeader(header, cookie)
	}
	return nil
}

func (s *kvStore) SetOptions(options *sessions.Options) {
	s.Options = options
}

// addCookieToHeader 将 Cookie 添加到 Header 中
// 这个函数模拟了 http.SetCookie 的行为
func addCookieToHeader(header http.Header, cookie *http.Cookie) {
	if v := cookie.String(); v != "" {
		header.Add("Set-Cookie", v)
	}
}

// SessionSerializer provides an interface hook for alternative serializers
type SessionSerializer interface {
	Deserialize(d []byte, ss *sessions.Session) error
	Serialize(ss *sessions.Session) ([]byte, error)
}

// GobSerializer uses gob package to encode the session map
type GobSerializer struct{}

// Serialize using gob
func (s GobSerializer) Serialize(ss *sessions.Session) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(ss.Values)
	if err == nil {
		return buf.Bytes(), nil
	}
	return nil, err
}

// Deserialize back to map[interface{}]interface{}
func (s GobSerializer) Deserialize(d []byte, ss *sessions.Session) error {
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	return dec.Decode(&ss.Values)
}
