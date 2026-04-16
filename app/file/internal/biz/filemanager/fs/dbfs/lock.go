package dbfs

import (
	"common/hashid"
	"context"
	"errors"
	"file/ent"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/lock"
	"file/internal/data/types"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
)

type (
	LockSession struct {
		Tokens     map[string]string
		TokenStack [][]string
	}

	LockByPath struct {
		Uri             *fs.URI
		ClosestAncestor *File
		Type            int
		Token           string
	}

	AlwaysIncludeTokenCtx struct{}
)

func (f *DBFS) ConfirmLock(ctx context.Context, ancestor fs.File, uri *fs.URI, token ...string) (func(), fs.LockSession, error) {
	session := LockSessionFromCtx(ctx)
	lockUri := ancestor.RootUri().JoinRaw(uri.PathTrimmed())
	ns, root, lKey := lockTupleFromUri(lockUri, f.user.Id, f.hasher)
	lc := lock.LockInfo{
		Ns:    ns,
		Root:  root,
		Token: token,
	}

	// Skip if already locked in current session
	if _, ok := session.Tokens[lKey]; ok {
		return func() {}, session, nil
	}

	release, tokenHit, err := f.ls.Confirm(time.Now(), lc)
	if err != nil {
		return nil, nil, err
	}

	session.Tokens[lKey] = tokenHit
	stackIndex := len(session.TokenStack) - 1
	session.TokenStack[stackIndex] = append(session.TokenStack[stackIndex], lKey)
	return release, session, nil
}

func (f *DBFS) Lock(ctx context.Context, d time.Duration, requester int, zeroDepth bool, application lock.Application,
	uri *fs.URI, token string) (fs.LockSession, error) {
	// Get navigator
	navigator, err := f.getNavigator(ctx, uri, NavigatorCapabilityLockFile)
	if err != nil {
		return nil, err
	}

	ancestor, err := f.getFileByPath(ctx, navigator, uri)
	if err != nil && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get ancestor: %w", err)
	}

	if ancestor.IsRootFolder() && ancestor.Uri(false).IsSame(uri, hashid.EncodeUserID(f.hasher, int(f.user.Id))) {
		return nil, fs.ErrNotSupportedAction.WithCause(fmt.Errorf("cannot lock root folder"))
	}

	// Lock require create or update permission
	if _, ok := ctx.Value(ByPassOwnerCheckCtxKey{}).(bool); !ok && ancestor.OwnerID() != requester {
		return nil, fs.ErrOwnerOnly
	}

	t := types.FileTypeFile
	if ancestor.Uri(false).IsSame(uri, hashid.EncodeUserID(f.hasher, int(f.user.Id))) {
		t = ancestor.Type()
	}
	lr := &LockByPath{
		Uri:             ancestor.RootUri().JoinRaw(uri.PathTrimmed()),
		ClosestAncestor: ancestor,
		Type:            t,
		Token:           token,
	}
	ls, err := f.acquireByPath(ctx, d, requester, zeroDepth, application, lr)
	if err != nil {
		return nil, err
	}

	return ls, nil
}

func (f *DBFS) Unlock(ctx context.Context, tokens ...string) error {
	return f.ls.Unlock(time.Now(), tokens...)
}

func (f *DBFS) Refresh(ctx context.Context, d time.Duration, token string) (lock.LockDetails, error) {
	return f.ls.Refresh(time.Now(), d, token)
}

func (f *DBFS) acquireByPath(ctx context.Context, duration time.Duration,
	requester int, zeroDepth bool, application lock.Application, locks ...*LockByPath) (*LockSession, error) {
	session := LockSessionFromCtx(ctx)

	// Prepare lock details for each files
	lockDetails := make([]lock.LockDetails, 0, len(locks))
	lockedRequest := make([]*LockByPath, 0, len(locks))
	for _, l := range locks {
		ns, root, lKey := lockTupleFromUri(l.Uri, f.user.Id, f.hasher)
		ld := lock.LockDetails{
			Owner: lock.Owner{
				Application: application,
			},
			Ns:        ns,
			Root:      root,
			ZeroDepth: zeroDepth,
			Duration:  duration,
			Type:      l.Type,
			Token:     l.Token,
		}

		// Skip if already locked in current session
		if _, ok := session.Tokens[lKey]; ok {
			continue
		}

		lockDetails = append(lockDetails, ld)
		lockedRequest = append(lockedRequest, l)
	}

	// Acquire lock
	tokens, err := f.ls.Create(time.Now(), lockDetails...)
	if len(tokens) > 0 {
		for i, token := range tokens {
			key := lockDetails[i].Key()
			session.Tokens[key] = token
			stackIndex := len(session.TokenStack) - 1
			session.TokenStack[stackIndex] = append(session.TokenStack[stackIndex], key)
		}
	}

	if err != nil {
		var conflicts lock.ConflictError
		if errors.As(err, &conflicts) {
			// Conflict with existing lock, generate userId-friendly error message
			conflicts = lo.Map(conflicts, func(c *lock.ConflictDetail, index int) *lock.ConflictDetail {
				lr := lockedRequest[c.Index]
				if lr.ClosestAncestor.Root().Model.OwnerID == int(requester) {
					// Add absolute path for ownerId issued lock request
					c.Path = newMyUri().JoinRaw(c.Path).String()
					return c
				}

				// Hide token for non-ownerId requester
				if v, ok := ctx.Value(AlwaysIncludeTokenCtx{}).(bool); !ok || !v {
					c.Token = ""
				}

				// If conflicted resources still under userId root, expose the relative path
				userRoot := lr.ClosestAncestor.UserRoot()
				userRootPath := userRoot.Uri(true).Path()
				if strings.HasPrefix(c.Path, userRootPath) {
					c.Path = userRoot.
						Uri(false).
						Join(strings.Split(strings.TrimPrefix(c.Path, userRootPath), fs.Separator)...).String()
					return c
				}

				// Hide sensitive information for non-ownerId issued lock request
				c.Path = ""
				return c
			})

			return session, fs.ErrLockConflict.WithCause(conflicts)
		}

		return session, fmt.Errorf("faield to create lock: %w", err)
	}

	// Check if any ancestor is modified during `getFileByPath` and `lock`.
	if err := f.ensureConsistency(
		ctx,
		lo.Map(lockedRequest, func(item *LockByPath, index int) *File {
			return item.ClosestAncestor
		})...,
	); err != nil {
		return session, err
	}

	return session, nil
}

func (f *DBFS) Release(ctx context.Context, session *LockSession) error {
	if session == nil {
		return nil
	}

	stackIndex := len(session.TokenStack) - 1
	err := f.ls.Unlock(time.Now(), lo.Map(session.TokenStack[stackIndex], func(key string, index int) string {
		return session.Tokens[key]
	})...)
	if err == nil {
		for _, key := range session.TokenStack[stackIndex] {
			delete(session.Tokens, key)
		}
		session.TokenStack = session.TokenStack[:len(session.TokenStack)-1]
	}

	return err
}

// ensureConsistency queries database for all given files and its ancestors, make sure there's no modification in
// between. This is to make sure there's no modification between navigator's first query and lock acquisition.
func (f *DBFS) ensureConsistency(ctx context.Context, files ...*File) error {
	if len(files) == 0 {
		return nil
	}

	// Generate a list of unique files (include ancestors) to check
	uniqueFiles := make(map[int]*File)
	for _, file := range files {
		for root := file; root != nil; root = root.Parent {
			if _, ok := uniqueFiles[root.Model.ID]; ok {
				// This files and its ancestors are already included
				break
			}

			uniqueFiles[root.Model.ID] = root
		}
	}

	page := 0
	fileIds := lo.Keys(uniqueFiles)
	for page >= 0 {
		files, next, err := f.fileClient.GetByIDs(ctx, fileIds, page)
		if err != nil {
			return fmt.Errorf("failed to check files consistency: %w", err)
		}

		for _, file := range files {
			latest := uniqueFiles[file.ID].Model
			if file.Name != latest.Name ||
				file.FileParentID != latest.FileParentID ||
				file.OwnerID != latest.OwnerID ||
				file.Type != latest.Type {
				return fs.ErrModified.
					WithCause(fmt.Errorf("files %s has been modified before lock acquisition", file.Name))
			}
		}

		page = next
	}

	return nil
}

// LockSessionFromCtx retrieves lock session from context. If no lock session
// found, a new empty lock session will be returned.
func LockSessionFromCtx(ctx context.Context) *LockSession {
	l, _ := ctx.Value(fs.LockSessionCtxKey{}).(*LockSession)
	if l == nil {
		ls := &LockSession{
			Tokens:     make(map[string]string),
			TokenStack: make([][]string, 0),
		}

		l = ls
	}

	l.TokenStack = append(l.TokenStack, make([]string, 0))
	return l
}

// Exclude removes lock from session, so that it won't be released.
func (l *LockSession) Exclude(lock *LockByPath, uid int64, hasher hashid.Encoder) string {
	_, _, lKey := lockTupleFromUri(lock.Uri, uid, hasher)
	foundInCurrentStack := false
	token, found := l.Tokens[lKey]
	if found {
		stackIndex := len(l.TokenStack) - 1
		l.TokenStack[stackIndex] = lo.Filter(l.TokenStack[stackIndex], func(t string, index int) bool {
			if t == lKey {
				foundInCurrentStack = true
			}
			return t != lKey
		})
		if foundInCurrentStack {
			delete(l.Tokens, lKey)
			return token
		}
	}

	return ""
}

func (l *LockSession) LastToken() string {
	stackIndex := len(l.TokenStack) - 1
	if len(l.TokenStack[stackIndex]) == 0 {
		return ""
	}
	return l.Tokens[l.TokenStack[stackIndex][len(l.TokenStack[stackIndex])-1]]
}

// WithAlwaysIncludeToken returns a new context with a flag to always include token in conflic response.
func WithAlwaysIncludeToken(ctx context.Context) context.Context {
	return context.WithValue(ctx, AlwaysIncludeTokenCtx{}, true)
}

func lockTupleFromUri(uri *fs.URI, uid int64, hasher hashid.Encoder) (string, string, string) {
	userId := int(uid)
	id := uri.ID(hashid.EncodeUserID(hasher, userId))
	if id == "" {
		id = strconv.Itoa(userId)
	}
	ns := fmt.Sprintf(id + "/" + string(uri.FileSystem()))
	root := uri.Path()
	return ns, root, ns + "/" + root
}
