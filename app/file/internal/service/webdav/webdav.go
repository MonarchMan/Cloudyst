package webdav

import (
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"api/external/trans"
	"common/boolset"
	"common/constants"
	"common/hashid"
	"common/request"
	"common/serializer"
	"common/util"
	"context"
	"errors"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/filemanager/lock"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"file/internal/data/types"
	"file/internal/pkg/webdav"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"user/ent"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
	"golang.org/x/tools/container/intsets"
)

type WebDAVService struct {
	dep      filemanager.ManagerDep
	dbfsDep  filemanager.DbfsDep
	uc       pbuser.UserClient
	l        *log.Helper
	settings setting.Provider
	hasher   hashid.Encoder
	mime     mime.MimeManager
}

func NewWebDAVService(dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep, uc pbuser.UserClient, l log.Logger,
	settings setting.Provider, hasher hashid.Encoder, mm mime.MimeManager) *WebDAVService {
	return &WebDAVService{
		dep:      dep,
		dbfsDep:  dbfsDep,
		uc:       uc,
		l:        log.NewHelper(l, log.WithMessageKey("service-webdav")),
		settings: settings,
		hasher:   hasher,
		mime:     mm,
	}
}

func (s *WebDAVService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u := trans.FromContext(r.Context())
	fm := manager.NewFileManager(s.dep, s.dbfsDep, u)
	defer fm.Recycle()

	status, err := http.StatusBadRequest, webdav.ErrUnsupportedMethod

	switch r.Method {
	case "OPTIONS":
		status, err = s.handleOptions(w, r, u, fm)
	case "GET", "HEAD", "POST":
		status, err = s.handleGetHeadPost(w, r, u, fm)
	case "DELETE":
		status, err = s.handleDelete(w, r, u, fm)
	case "PUT":
		status, err = s.handlePut(w, r, u, fm)
	case "MKCOL":
		status, err = s.handleMkcol(w, r, u, fm)
	case "COPY", "MOVE":
		status, err = s.handleCopyMove(w, r, u, fm)
	case "LOCK":
		status, err = s.handleLock(w, r, u, fm)
	case "UNLOCK":
		status, err = s.handleUnlock(w, r, u, fm)
	case "PROPFIND":
		status, err = s.handlePropfind(w, r, u, fm)
	case "PROPPATCH":
		status, err = s.handleProppatch(w, r, u, fm)
	}
	if status != 0 {
		w.WriteHeader(status)
		if status != http.StatusNoContent {
			w.Write([]byte(webdav.StatusText(status)))
		}
	}
	if err != nil {
		s.l.Debugf("WebDAV request failed with error: %s", err)
	}
}

func (s *WebDAVService) handleGetHeadPost(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	_, reqPath, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}
	target, _, err := fm.SharedAddressTranslation(r.Context(), reqPath)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	if target.Type() != types.FileTypeFile {
		return http.StatusMethodNotAllowed, nil
	}

	es, err := fm.GetEntitySource(r.Context(), target.PrimaryEntityID())
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}
	defer es.Close()

	es.Apply(entitysource.WithSpeedLimit(user.Group.SpeedLimit.GetValue()))
	options := boolset.BooleanSet(user.DavAccounts[0].Options)
	permissions := boolset.BooleanSet(user.Group.Permissions)
	if es.ShouldInternalProxy() || ((&options).Enabled(constants.DavAccountProxy) &&
		(&permissions).Enabled(int(types.GroupPermissionWebDAVProxy))) {
		es.Serve(w, r)
	} else {
		expire := time.Now().Add(s.settings.EntityUrlValidDuration(r.Context()))
		src, err := es.Url(r.Context(), entitysource.WithExpire(&expire))
		if err != nil {
			return purposeStatusCodeFromError(err), err
		}
		http.Redirect(w, r, src.Url, http.StatusNotFound)
	}

	return 0, nil
}

func (s *WebDAVService) handleOptions(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	allow := []string{"OPTIONS", "LOCK", "PUT", "MKCOL"}

	if user != nil {
		_, reqPath, status, err := stripPrefix(r.URL.Path, user)
		if err != nil {
			return status, err
		}
		if target, _, err := fm.SharedAddressTranslation(r.Context(), reqPath); err == nil {
			allow = allow[:1]
			read, update, del, create := true, true, true, true
			if target.OwnerID() != int(user.Id) {
				update = false
				del = false
				create = false
			}
			if del {
				allow = append(allow, "DELETE", "MOVE")
			}
			if read {
				allow = append(allow, "COPY", "PROPFIND")
				if target.Type() == types.FileTypeFile {
					allow = append(allow, "GET", "HEAD", "POST")
				}
			}
			if update || create {
				allow = append(allow, "LOCK", "UNLOCK")
			}
			if update {
				allow = append(allow, "PROPPATCH")
				if target.Type() == types.FileTypeFile {
					allow = append(allow, "PUT")
				}
			}
		} else {
			s.l.WithContext(r.Context()).Debugf("Handle options failed to get target: %s", err)
		}
	}
	w.Header().Set("Allow", strings.Join(allow, ", "))
	w.Header().Set("DAV", "1, 2")
	w.Header().Set("MS-Author-Via", "DAV")
	return 0, nil
}

func (s *WebDAVService) auth(w http.ResponseWriter, r *http.Request) bool {
	username, password, ok := r.BasicAuth()
	if !ok {
		if r.Method == http.MethodOptions {
			return true
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="cloudyst`)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	expectedUser, err := s.uc.GetActiveUserByDavAccount(r.Context(), &pbuser.GetActiveUserByDavAccountRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		//if username != "" {
		//	if u, err := s.uc.GetUserByEmail(r.Context(), &pbuser.GetUserByEmailRequest{Email: username}); err == nil {
		//
		//	}
		//}
		s.l.Debugf("WebDAVAuth: failed to get user %q with provided credential: %s", username, err)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	// validate dav account
	accounts := expectedUser.DavAccounts
	if len(accounts) == 0 {
		s.l.Debugf("WebDAVAuth: failed to get user dav accounts %q with provided credential: %s", username, err)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	// check whether group uses WebDAV
	group := expectedUser.Group
	if group == nil {
		s.l.Debugf("WebDAVAuth: user group not found: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	// validate group permission to access WebDAV
	permissions := boolset.BooleanSet(group.Permissions)
	if !(&permissions).Enabled(int(types.GroupPermissionWebDAV)) {
		s.l.Debugf("WebDAVAuth: user %q does not have WebDAV permission.", expectedUser.Email)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	// check read-only
	permissions = expectedUser.DavAccounts[0].Options
	if !(&permissions).Enabled(int(constants.DavAccountReadOnly)) {
		switch r.Method {
		case http.MethodDelete, http.MethodPut, "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK":
			w.WriteHeader(http.StatusForbidden)
			return false
		}
	}

	context.WithValue(r.Context(), trans.UserCtx{}, expectedUser)
	return true
}

func (s *WebDAVService) handleDelete(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	_, reqPath, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}

	ancestor, uri, err := fm.SharedAddressTranslation(r.Context(), reqPath)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	release, ls, status, err := s.confirmLock(r, fm, user, ancestor, nil, uri, nil)
	if err != nil {
		return status, err
	}
	defer release()

	ctx := fs.LockSessionToContext(r.Context(), ls)

	// TODO: return MultiStatus where appropriate.

	if err := fm.Delete(ctx, []*fs.URI{uri}); err != nil {
		return purposeStatusCodeFromError(err), err
	}

	return http.StatusNoContent, nil
}

func (s *WebDAVService) confirmLock(r *http.Request, fm manager.FileManager, user *userpb.User, srcAnc, dstAnc fs.File,
	src, dst *fs.URI) (func(), fs.LockSession, int, error) {
	hdr := r.Header.Get("If")
	if hdr == "" {
		// An empty If header means that the client hasn't previously created locks.
		// Even if this client doesn't care about locks, we still need to check that
		// the resources aren't locked by another client, so we create temporary
		// locks that would conflict with another client's locks. These temporary
		// locks are unlocked at the end of the HTTP request.
		srcToken, dstToken := "", ""
		ap := fs.LockApp(fs.ApplicationDAV)
		var (
			ctx context.Context = r.Context()
			ls  fs.LockSession
			err error
		)
		if src != nil {
			ls, err = fm.Lock(ctx, -1, int(user.Id), true, ap, src, "")
			if err != nil {
				return nil, nil, purposeStatusCodeFromError(err), err
			}
			srcToken = ls.LastToken()
			ctx = fs.LockSessionToContext(ctx, ls)
		}

		if dst != nil {
			ls, err = fm.Lock(ctx, -1, int(user.Id), true, ap, dst, "")
			if err != nil {
				if src != nil {
					_ = fm.Unlock(ctx, srcToken)
				}
				return nil, nil, purposeStatusCodeFromError(err), err
			}
			dstToken = ls.LastToken()
			ctx = fs.LockSessionToContext(ctx, ls)
		}

		return func() {
			if dstToken != "" {
				_ = fm.Unlock(ctx, dstToken)
			}
			if srcToken != "" {
				_ = fm.Unlock(ctx, srcToken)
			}
		}, ls, 0, nil
	}

	ih, ok := webdav.ParseIfHeader(hdr)
	if !ok {
		return nil, nil, http.StatusBadRequest, webdav.ErrInvalidIfHeader
	}
	// ih is a disjunction (OR) of ifLists, so any ifList will do.
	for _, l := range ih.Lists {
		var (
			releaseSrc = func() {}
			releaseDst = func() {}
			ls         fs.LockSession
			err        error
		)
		if src != nil {
			releaseSrc, ls, err = fm.ConfirmLock(r.Context(), srcAnc, src, lo.Map(l.Conditions, func(c webdav.Condition, index int) string {
				return c.Token
			})...)
			if errors.Is(err, lock.ErrConfirmationFailed) {
				continue
			}
			if err != nil {
				return nil, nil, purposeStatusCodeFromError(err), err
			}
		}

		if dst != nil {
			releaseDst, ls, err = fm.ConfirmLock(r.Context(), dstAnc, dst, lo.Map(l.Conditions, func(c webdav.Condition, index int) string {
				return c.Token
			})...)
			if errors.Is(err, lock.ErrConfirmationFailed) {
				continue
			}
			if err != nil {
				return nil, nil, purposeStatusCodeFromError(err), err
			}
		}

		return func() {
			releaseDst()
			releaseSrc()
		}, ls, 0, nil
	}
	// Section 10.4.1 says that "If this header is evaluated and all state lists
	// fail, then the request must fail with a 412 (Precondition Failed) status."
	// We follow the spec even though the cond_put_corrupt_token test case from
	// the litmus test warns on seeing a 412 instead of a 423 (Locked).
	return nil, nil, http.StatusPreconditionFailed, webdav.ErrLocked
}

func (s *WebDAVService) handlePut(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	_, reqPath, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}

	ancestor, uri, err := fm.SharedAddressTranslation(r.Context(), reqPath)
	if err != nil && !ent.IsNotFound(err) {
		return purposeStatusCodeFromError(err), err
	}

	if options := boolset.BooleanSet(user.DavAccounts[0].Options); (&options).Enabled(constants.DavAccountDisableSysFiles) {
		if strings.HasPrefix(reqPath.Name(), ".") {
			return http.StatusMethodNotAllowed, nil
		}
	}

	release, ls, status, err := s.confirmLock(r, fm, user, ancestor, nil, uri, nil)
	if err != nil {
		return status, err
	}
	defer release()

	ctx := fs.LockSessionToContext(r.Context(), ls)
	// TODO(rost): Support the If-Match, If-None-Match headers? See bradfitz'

	rc, fileSize, err := request.SniffContentLength(r.Context())
	if err != nil {
		return http.StatusBadRequest, err
	}

	fileData := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:  uri,
			Size: fileSize,
		},
		File: rc,
		Mode: fs.ModeOverwrite,
	}

	//m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	//defer m.Recycle()

	// Update file
	res, err := fm.Update(ctx, fileData)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	etag, err := s.findETag(ctx, fm, res)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("ETag", etag)
	return http.StatusCreated, nil
}

func (s *WebDAVService) handleMkcol(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	_, reqPath, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}

	ancestor, uri, err := fm.SharedAddressTranslation(r.Context(), reqPath)
	if err != nil && !ent.IsNotFound(err) {
		return purposeStatusCodeFromError(err), err
	}

	release, ls, status, err := s.confirmLock(r, fm, user, ancestor, nil, uri, nil)
	if err != nil {
		return status, err
	}
	defer release()

	ctx := fs.LockSessionToContext(r.Context(), ls)

	if r.ContentLength > 0 {
		return http.StatusUnsupportedMediaType, nil
	}

	_, err = fm.Create(ctx, uri, types.FileTypeFolder, dbfs.WithNoChainedCreation(), dbfs.WithErrorOnConflict())
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	return http.StatusCreated, nil
}

func (s *WebDAVService) handleCopyMove(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	header := r.Header.Get("Destination")
	if header == "" {
		return http.StatusBadRequest, webdav.ErrInvalidDestination
	}
	u, err := url.Parse(header)
	if err != nil {
		return http.StatusBadRequest, webdav.ErrInvalidDestination
	}
	if u.Host != "" && u.Host != r.Host {
		return http.StatusBadGateway, webdav.ErrInvalidDestination
	}

	_, src, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}

	srcTarget, srcUri, err := fm.SharedAddressTranslation(r.Context(), src)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	_, dst, status, err := stripPrefix(u.Path, user)
	if err != nil {
		return status, err
	}

	dstTarget, dstUri, err := fm.SharedAddressTranslation(r.Context(), dst)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	_, dstFolderUri, err := fm.SharedAddressTranslation(r.Context(), dst.DirUri())
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	if srcUri.IsSame(dstUri, hashid.EncodeUserID(s.hasher, int(user.Id))) {
		return http.StatusForbidden, webdav.ErrDestinationEqualsSource
	}

	if r.Method == "COPY" {
		// Section 7.5.1 says that a COPY only needs to lock the destination,
		// not both destination and source. Strictly speaking, this is racy,
		// even though a COPY doesn't modify the source, if a concurrent
		// operation modifies the source. However, the litmus test explicitly
		// checks that COPYing a locked-by-another source is OK.
		release, ls, status, err := s.confirmLock(r, fm, user, dstTarget, nil, dstUri, nil)
		if err != nil {
			return status, err
		}
		defer release()
		ctx := fs.LockSessionToContext(r.Context(), ls)

		// Section 9.8.3 says that "The COPY method on a collection without a Depth
		// header must act as if a Depth header with value "infinity" was included".
		depth := infiniteDepth
		if hdr := r.Header.Get("Depth"); hdr != "" {
			depth = parseDepth(hdr)
			if depth != 0 && depth != infiniteDepth {
				// Section 9.8.3 says that "A client may submit a Depth header on a
				// COPY on a collection with a value of "0" or "infinity"."
				return http.StatusBadRequest, webdav.ErrInvalidDepth
			}
		}

		if err := fm.MoveOrCopy(ctx, []*fs.URI{srcUri}, dstFolderUri, true); err != nil {
			return purposeStatusCodeFromError(err), err
		}
	}

	release, ls, status, err := s.confirmLock(r, fm, user, srcTarget, dstTarget, srcUri, dstUri)
	if err != nil {
		return status, err
	}
	defer release()
	ctx := fs.LockSessionToContext(r.Context(), ls)

	// Section 9.9.2 says that "The MOVE method on a collection must act as if
	// a "Depth: infinity" header was used on it. A client must not submit a
	// Depth header on a MOVE on a collection with any value but "infinity"."
	if hdr := r.Header.Get("Depth"); hdr != "" {
		if parseDepth(hdr) != infiniteDepth {
			return http.StatusBadRequest, webdav.ErrInvalidDepth
		}
	}
	if err := fm.MoveOrCopy(ctx, []*fs.URI{srcUri}, dstFolderUri, false); err != nil {
		return purposeStatusCodeFromError(err), err
	}

	if dstUri.Name() != srcUri.Name() {
		if _, err := fm.Rename(ctx, dstFolderUri.Join(srcUri.Name()), dstUri.Name()); err != nil {
			return purposeStatusCodeFromError(err), err
		}
	}

	return http.StatusNoContent, nil
}

func (s *WebDAVService) handleLock(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (retStatus int, retErr error) {
	duration, err := webdav.ParseTimeout(r.Header.Get("Timeout"))
	if err != nil {
		return http.StatusBadRequest, err
	}
	li, status, err := webdav.ReadLockInfo(r.Body)
	if err != nil {
		return status, err
	}

	href, reqPath, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}

	token, ld, created := "", lock.LockDetails{}, false
	if li == (webdav.LockInfo{}) {
		// An empty lockInfo means to refresh the lock.
		ih, ok := webdav.ParseIfHeader(r.Header.Get("If"))
		if !ok {
			return http.StatusBadRequest, webdav.ErrInvalidIfHeader
		}
		if len(ih.Lists) == 1 && len(ih.Lists[0].Conditions) == 1 {
			token = ih.Lists[0].Conditions[0].Token
		}
		if token == "" {
			return http.StatusBadRequest, webdav.ErrInvalidLockToken
		}
		ld, err = fm.Refresh(r.Context(), duration, token)
		if err != nil {
			if errors.Is(err, lock.ErrNoSuchLock) {
				return http.StatusPreconditionFailed, err
			}
			return http.StatusInternalServerError, err
		}
		ld.Root = href
	} else {
		// Section 9.10.3 says that "If no Depth header is submitted on a LOCK request,
		// then the request MUST act as if a "Depth:infinity" had been submitted."
		depth := infiniteDepth
		if hdr := r.Header.Get("Depth"); hdr != "" {
			depth = parseDepth(hdr)
			if depth != 0 && depth != infiniteDepth {
				// Section 9.10.3 says that "Values other than 0 or infinity must not be
				// used with the Depth header on a LOCK method".
				return http.StatusBadRequest, webdav.ErrInvalidDepth
			}
		}

		ancestor, uri, err := fm.SharedAddressTranslation(r.Context(), reqPath)
		if err != nil && !ent.IsNotFound(err) {
			return purposeStatusCodeFromError(err), err
		}

		ld = lock.LockDetails{
			Root:      href,
			Duration:  duration,
			Owner:     lock.Owner{Application: lock.Application{InnerXML: li.Owner.InnerXML}},
			ZeroDepth: depth == 0,
		}
		app := lock.Application{
			Type:     string(fs.ApplicationDAV),
			InnerXML: li.Owner.InnerXML,
		}
		ls, err := fm.Lock(r.Context(), duration, int(user.Id), depth == 0, app, uri, "")
		if err != nil {
			if errors.Is(err, lock.ErrLocked) {
				return http.StatusLocked, err
			}
			return http.StatusInternalServerError, err
		}
		token = ls.LastToken()
		ctx := fs.LockSessionToContext(r.Context(), ls)
		defer func() {
			if retErr != nil {
				_ = fm.Unlock(r.Context(), token)
			}
		}()

		// Create the resource if it didn't previously exist.
		if !ancestor.Uri(false).IsSame(uri, hashid.EncodeUserID(s.hasher, int(user.Id))) {
			if _, err = fm.Create(ctx, uri, types.FileTypeFile); err != nil {
				return purposeStatusCodeFromError(err), err
			}

			created = true
		}

		// http://www.webdav.org/specs/rfc4918.html#HEADER_Lock-Token says that the
		// Lock-Token value is a Coded-URL. We add angle brackets.
		w.Header().Set("Lock-Token", "<"+token+">")
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if created {
		// This is not "return http.StatusCreated, nil" because we write our own (XML) response to w
		// and Handler.ServeHTTP would otherwise write "Created".
		w.WriteHeader(http.StatusCreated)
	}
	writeLockInfo(w, token, ld)
	return 0, nil
}

func (s *WebDAVService) handleUnlock(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	// http://www.webdav.org/specs/rfc4918.html#HEADER_Lock-Token says that the
	// Lock-Token value is a Coded-URL. We strip its angle brackets.
	t := r.Header.Get("Lock-Token")
	if len(t) < 2 || t[0] != '<' || t[len(t)-1] != '>' {
		return http.StatusBadRequest, webdav.ErrInvalidLockToken
	}
	t = t[1 : len(t)-1]
	err := fm.Unlock(r.Context(), t)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	return http.StatusNoContent, err
}

func (s *WebDAVService) handlePropfind(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	href, reqPath, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}

	_, targetPath, err := fm.SharedAddressTranslation(r.Context(), reqPath)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	depth := infiniteDepth
	if header := r.Header.Get("Depth"); header != "" {
		depth = parseDepth(header)
		if depth == invalidDepth {
			return http.StatusBadRequest, webdav.ErrInvalidDepth
		}
	}
	pf, status, err := webdav.ReadPropfind(r.Body)
	if err != nil {
		return status, err
	}

	mw := webdav.MultistatusWriter{
		W: w,
	}
	walkFn := func(f fs.File, level int) error {
		var pstats []webdav.Propstat
		if pf.Propname != nil {
			pnames, err := s.propNames(r.Context(), f, fm)
			if err != nil {
				return err
			}
			pstat := webdav.Propstat{Status: http.StatusOK}
			for _, xmlname := range pnames {
				pstat.Props = append(pstat.Props, webdav.Property{XMLName: xmlname})
			}
			pstats = append(pstats, pstat)
		} else if pf.Allprop != nil {
			pstats, err = s.allProps(r.Context(), f, fm, pf.Prop)
		} else {
			pstats, err = s.Props(r.Context(), f, fm, pf.Prop)
		}
		if err != nil {
			return err
		}

		p := path.Join(constants.DavPrefix, href)
		elements := f.Uri(false).Elements()
		for i := 0; i < level; i++ {
			p = path.Join(p, elements[len(elements)-level+i])
		}
		if f.Type() == types.FileTypeFolder {
			p = util.FillSlash(p)
		}

		return mw.Write(webdav.MakePropstatResponse(p, pstats))
	}

	if err := fm.Walk(r.Context(), targetPath, depth, walkFn, dbfs.WithFilePublicMetadata()); err != nil {
		return purposeStatusCodeFromError(err), err
	}

	closeErr := mw.Close()
	if closeErr != nil {
		return http.StatusInternalServerError, closeErr
	}
	return 0, nil
}

func (s *WebDAVService) handleProppatch(w http.ResponseWriter, r *http.Request, user *userpb.User, fm manager.FileManager) (int, error) {
	_, reqPath, status, err := stripPrefix(r.URL.Path, user)
	if err != nil {
		return status, err
	}

	ancestor, uri, err := fm.SharedAddressTranslation(r.Context(), reqPath)
	if err != nil {
		return purposeStatusCodeFromError(err), err
	}

	release, ls, status, err := s.confirmLock(r, fm, user, ancestor, ancestor, uri, uri)
	if err != nil {
		return status, err
	}
	defer release()
	ctx := fs.LockSessionToContext(r.Context(), ls)

	patches, status, err := readProppatch(r.Body)
	if err != nil {
		return status, err
	}
	pstats, err := s.Patch(ctx, ancestor, fm, patches)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	mw := webdav.MultistatusWriter{W: w}
	writeErr := mw.Write(webdav.MakePropstatResponse(r.URL.Path, pstats))
	if writeErr != nil {
		return http.StatusInternalServerError, writeErr
	}
	closeErr := mw.Close()
	if closeErr != nil {
		return http.StatusInternalServerError, closeErr
	}
	return 0, nil
}

func purposeStatusCodeFromError(err error) int {
	if ent.IsNotFound(err) {
		return http.StatusNotFound
	}

	if errors.Is(err, lock.ErrNoSuchLock) {
		return http.StatusConflict
	}

	var ae *serializer.AggregateError
	if errors.As(err, &ae) && len(ae.Raw()) > 0 {
		for _, e := range ae.Raw() {
			return purposeStatusCodeFromError(e)
		}
	}

	var appErr serializer.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case serializer.CodeNotFound, serializer.CodeParentNotExist, serializer.CodeEntityNotExist:
			return http.StatusNotFound
		case serializer.CodeNoPermissionErr:
			return http.StatusForbidden
		case serializer.CodeLockConflict:
			return http.StatusLocked
		case serializer.CodeObjectExist:
			return http.StatusMethodNotAllowed
		}
	}

	return http.StatusInternalServerError
}

func stripPrefix(p string, u *userpb.User) (string, *fs.URI, int, error) {
	base, err := fs.NewUriFromString(u.DavAccounts[0].Uri)
	if err != nil {
		return "", nil, http.StatusInternalServerError, err
	}

	prefix := constants.DavPrefix
	if r := strings.TrimPrefix(p, prefix); len(r) < len(p) {
		r = strings.TrimPrefix(r, fs.Separator)
		return r, base.JoinRaw(util.RemoveSlash(r)), http.StatusOK, nil
	}
	return "", nil, http.StatusNotFound, webdav.ErrPrefixMismatch
}

const (
	infiniteDepth = intsets.MaxInt
	invalidDepth  = -2
)

// parseDepth maps the strings "0", "1" and "infinity" to 0, 1 and
// infiniteDepth. Parsing any other string returns invalidDepth.
//
// Different WebDAV methods have further constraints on valid depths:
//   - PROPFIND has no further restrictions, as per section 9.1.
//   - COPY accepts only "0" or "infinity", as per section 9.8.3.
//   - MOVE accepts only "infinity", as per section 9.9.2.
//   - LOCK accepts only "0" or "infinity", as per section 9.10.3.
//
// These constraints are enforced by the handleXxx methods.
func parseDepth(s string) int {
	switch s {
	case "0":
		return 0
	case "1":
		return 1
	case "infinity":
		return infiniteDepth
	}
	return invalidDepth
}
