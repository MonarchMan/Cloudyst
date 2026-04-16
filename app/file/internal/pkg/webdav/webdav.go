package webdav

import (
	"errors"

	"golang.org/x/tools/container/intsets"
)

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

var (
	ErrDestinationEqualsSource = errors.New("webdav: destination equals source")
	ErrDirectoryNotEmpty       = errors.New("webdav: directory not empty")
	ErrInvalidDepth            = errors.New("webdav: invalid depth")
	ErrInvalidDestination      = errors.New("webdav: invalid destination")
	ErrInvalidIfHeader         = errors.New("webdav: invalid If header")
	ErrInvalidLockInfo         = errors.New("webdav: invalid lock info")
	ErrInvalidLockToken        = errors.New("webdav: invalid lock token")
	ErrInvalidPropfind         = errors.New("webdav: invalid propfind")
	ErrInvalidProppatch        = errors.New("webdav: invalid proppatch")
	ErrInvalidResponse         = errors.New("webdav: invalid response")
	ErrInvalidTimeout          = errors.New("webdav: invalid timeout")
	ErrNoFileSystem            = errors.New("webdav: no file system")
	ErrNoLockSystem            = errors.New("webdav: no lock system")
	ErrNotADirectory           = errors.New("webdav: not a directory")
	ErrPrefixMismatch          = errors.New("webdav: prefix mismatch")
	ErrRecursionTooDeep        = errors.New("webdav: recursion too deep")
	ErrUnsupportedLockInfo     = errors.New("webdav: unsupported lock info")
	ErrUnsupportedMethod       = errors.New("webdav: unsupported method")
)
