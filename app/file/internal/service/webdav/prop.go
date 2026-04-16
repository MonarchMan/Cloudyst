package webdav

import (
	pbfile "api/api/file/files/v1"
	"api/external/trans"
	"bytes"
	"common/constants"
	"common/hashid"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/data/types"
	"file/internal/pkg/webdav"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	DeadPropsMetadataPrefix = "dav:"
	SpaceNameSeparator      = "|"
)

type (
	DeadPropsStore struct {
		Lang     string `json:"l,omitempty"`
		InnerXML []byte `json:"i,omitempty"`
	}
)

// Proppatch describes a property update instruction as defined in RFC 4918.
// See http://www.webdav.org/specs/rfc4918.html#METHOD_PROPPATCH
type Proppatch struct {
	// Remove specifies whether this patch removes properties. If it does not
	// remove them, it sets them.
	Remove bool
	// Props contains the properties to be set or removed.
	Props []webdav.Property
}

// DeadPropsHolder holds the dead properties of a resource.
//
// Dead properties are those properties that are explicitly defined. In
// comparison, live properties, such as DAV:getcontentlength, are implicitly
// defined by the underlying resource, and cannot be explicitly overridden or
// removed. See the Terminology section of
// http://www.webdav.org/specs/rfc4918.html#rfc.section.3
//
// There is a whitelist of the names of live properties. This package handles
// all live properties, and will only pass non-whitelisted names to the Patch
// method of DeadPropsHolder implementations.
type DeadPropsHolder interface {
	// DeadProps returns a copy of the dead properties held.
	DeadProps() (map[xml.Name]webdav.Property, error)

	// Patch patches the dead properties held.
	//
	// Patching is atomic; either all or no patches succeed. It returns (nil,
	// non-nil) if an internal server error occurred, otherwise the Propstats
	// collectively contain one webdav.Property for each proposed patch webdav.Property. If
	// all patches succeed, Patch returns a slice of length one and a Propstat
	// element with a 200 OK HTTP status code. If none succeed, for reasons
	// other than an internal server error, no Propstat has status 200 OK.
	//
	// For more details on when various HTTP status codes apply, see
	// http://www.webdav.org/specs/rfc4918.html#PROPPATCH-status
	Patch(context.Context, []Proppatch) ([]webdav.Propstat, error)
}

// LivePropFinder 动态属性查找函数类型
type LivePropFinder func(ctx context.Context, fm manager.FileManager, file fs.File) (string, error)

// LivePropDef 动态属性定义
type LivePropDef struct {
	FindFn LivePropFinder
	Dir    bool // 是否适用于目录
}

func (s *WebDAVService) GetLiveProps() map[xml.Name]LivePropDef {
	return map[xml.Name]LivePropDef{
		{Space: "DAV:", Local: "resourcetype"}: {
			FindFn: s.findResourceType,
			Dir:    true,
		},
		{Space: "DAV:", Local: "displayname"}: {
			FindFn: s.findDisplayName,
			Dir:    true,
		},
		{Space: "DAV:", Local: "getcontentlength"}: {
			FindFn: s.findContentLength,
			Dir:    false,
		},
		{Space: "DAV:", Local: "getlastmodified"}: {
			FindFn: s.findLastModified,
			Dir:    true,
		},
		{Space: "DAV:", Local: "creationdate"}: {
			FindFn: s.findCreationDate,
			Dir:    true,
		},
		{Space: "DAV:", Local: "getcontenttype"}: {
			FindFn: s.findContentType,
			Dir:    false,
		},
		{Space: "DAV:", Local: "getetag"}: {
			FindFn: s.findETag,
			Dir:    false,
		},
		{Space: "DAV:", Local: "supportedlock"}: {
			FindFn: s.findSupportedLock,
			Dir:    true,
		},
		{Space: "DAV:", Local: "quota-used-bytes"}: {
			FindFn: s.findQuotaUsedBytes,
			Dir:    true,
		},
		{Space: "DAV:", Local: "quota-available-bytes"}: {
			FindFn: s.findQuotaAvailableBytes,
			Dir:    true,
		},
	}
}

func (s *WebDAVService) Props(ctx context.Context, file fs.File, fm manager.FileManager, pnames []xml.Name) ([]webdav.Propstat, error) {
	isDir := file.Type() == types.FileTypeFolder

	// 获取 dead props
	deadProps, err := s.getDeadProps(file)
	if err != nil {
		return nil, err
	}

	liveProps := s.GetLiveProps()

	pstatOK := webdav.Propstat{Status: http.StatusOK}
	pstatNotFound := webdav.Propstat{Status: http.StatusNotFound}

	for _, pn := range pnames {
		// 检查 dead properties
		if dp, ok := deadProps[pn]; ok {
			pstatOK.Props = append(pstatOK.Props, dp)
			continue
		}

		// 检查 live properties
		if prop, ok := liveProps[pn]; ok && prop.FindFn != nil && (prop.Dir || !isDir) {
			innerXML, err := prop.FindFn(ctx, fm, file)
			if err != nil {
				if errors.Is(err, ErrNotImplemented) {
					pstatNotFound.Props = append(pstatNotFound.Props, webdav.Property{
						XMLName: pn,
					})
					continue
				}
				return nil, err
			}
			pstatOK.Props = append(pstatOK.Props, webdav.Property{
				XMLName:  pn,
				InnerXML: []byte(innerXML),
			})
		} else {
			pstatNotFound.Props = append(pstatNotFound.Props, webdav.Property{
				XMLName: pn,
			})
		}
	}

	return webdav.MakePropstats(pstatOK, pstatNotFound), nil
}

func (s *WebDAVService) getDeadProps(file fs.File) (map[xml.Name]webdav.Property, error) {
	meta := file.Metadata()
	res := make(map[xml.Name]webdav.Property)

	for k, v := range meta {
		if !strings.HasPrefix(k, DeadPropsMetadataPrefix) {
			continue
		}

		spaceLocal := strings.SplitN(
			strings.TrimPrefix(k, DeadPropsMetadataPrefix),
			SpaceNameSeparator,
			2,
		)
		if len(spaceLocal) != 2 {
			continue
		}

		name := xml.Name{Space: spaceLocal[0], Local: spaceLocal[1]}
		propsStore := &DeadPropsStore{}
		if err := json.Unmarshal([]byte(v), propsStore); err != nil {
			return nil, err
		}

		res[name] = webdav.Property{
			XMLName:  name,
			InnerXML: propsStore.InnerXML,
			Lang:     propsStore.Lang,
		}
	}

	return res, nil
}

// Allprop returns the properties defined for resource name and the properties
// named in include.
//
// Note that RFC 4918 defines 'allProps' to return the DAV: properties defined
// within the RFC plus dead properties. Other live properties should only be
// returned if they are named in 'include'.
//
// See http://www.webdav.org/specs/rfc4918.html#METHOD_PROPFIND
func (s *WebDAVService) allProps(ctx context.Context, file fs.File, fm manager.FileManager, include []xml.Name) ([]webdav.Propstat, error) {
	pnames, err := s.propNames(ctx, file, fm)
	if err != nil {
		return nil, err
	}
	// Add names from include if they are not already covered in pnames.
	nameset := make(map[xml.Name]bool)
	for _, pn := range pnames {
		nameset[pn] = true
	}
	for _, pn := range include {
		if !nameset[pn] {
			pnames = append(pnames, pn)
		}
	}
	return s.Props(ctx, file, fm, pnames)
}

// Patch 修补属性
func (s *WebDAVService) Patch(ctx context.Context, file fs.File, fm manager.FileManager, patches []Proppatch) ([]webdav.Propstat, error) {
	// 检查是否修改受保护的属性
	liveProps := s.GetLiveProps()
	conflict := false

loop:
	for _, patch := range patches {
		for _, p := range patch.Props {
			if _, ok := liveProps[p.XMLName]; ok {
				conflict = true
				break loop
			}
		}
	}

	if conflict {
		pstatForbidden := webdav.Propstat{
			Status:   http.StatusForbidden,
			XMLError: `<D:cannot-modify-protected-property xmlns:D="DAV:"/>`,
		}
		pstatFailedDep := webdav.Propstat{
			Status: constants.StatusFailedDependency,
		}

		for _, patch := range patches {
			for _, p := range patch.Props {
				if _, ok := liveProps[p.XMLName]; ok {
					pstatForbidden.Props = append(pstatForbidden.Props, webdav.Property{XMLName: p.XMLName})
				} else {
					pstatFailedDep.Props = append(pstatFailedDep.Props, webdav.Property{XMLName: p.XMLName})
				}
			}
		}

		return webdav.MakePropstats(pstatForbidden, pstatFailedDep), nil
	}

	// 应用 dead props 修补
	return s.patchDeadProps(ctx, fm, file, patches)
}

// patchDeadProps 修补 dead properties
func (s *WebDAVService) patchDeadProps(ctx context.Context, fm manager.FileManager, file fs.File, patches []Proppatch) ([]webdav.Propstat, error) {
	metadataPatches := make([]*pbfile.MetadataPatch, 0, len(patches))
	pstat := webdav.Propstat{Status: http.StatusOK}

	for _, patch := range patches {
		for _, prop := range patch.Props {
			// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propstat says that
			// "The contents of the prop XML element must only list the names of
			// properties to which the result in the status element applies."
			pstat.Props = append(pstat.Props, webdav.Property{XMLName: prop.XMLName})

			key := DeadPropsMetadataPrefix + prop.XMLName.Space + SpaceNameSeparator + prop.XMLName.Local

			if patch.Remove {
				metadataPatches = append(metadataPatches, &pbfile.MetadataPatch{
					Key:    key,
					Remove: true,
				})
			} else {
				val, err := json.Marshal(&DeadPropsStore{
					Lang:     prop.Lang,
					InnerXML: prop.InnerXML,
				})
				if err != nil {
					return nil, err
				}
				metadataPatches = append(metadataPatches, &pbfile.MetadataPatch{
					Key:   key,
					Value: string(val),
				})
			}
		}
	}

	if err := fm.PatchMedata(ctx, []*fs.URI{file.Uri(false)}, metadataPatches...); err != nil {
		return nil, err
	}

	return []webdav.Propstat{pstat}, nil
}

func (s *WebDAVService) propNames(ctx context.Context, file fs.File, fm manager.FileManager) ([]xml.Name, error) {
	var deadProps map[xml.Name]webdav.Property
	deadProps, err := s.getDeadProps(file)
	if err != nil {
		return nil, err
	}

	isDir := file.Type() == types.FileTypeFolder
	liveProps := s.GetLiveProps()
	pnames := make([]xml.Name, 0, len(liveProps)+len(deadProps))
	for pn, prop := range liveProps {
		if prop.FindFn != nil && (prop.Dir || !isDir) {
			pnames = append(pnames, pn)
		}
	}
	for pn := range deadProps {
		pnames = append(pnames, pn)
	}
	return pnames, nil
}

func escapeXML(s string) string {
	for i := 0; i < len(s); i++ {
		// As an optimization, if s contains only ASCII letters, digits or a
		// few special characters, the escaped value is s itself and we don't
		// need to allocate a buffer and convert between string and []byte.
		switch c := s[i]; {
		case c == ' ' || c == '_' ||
			('+' <= c && c <= '9') || // Digits as well as + , - . and /
			('A' <= c && c <= 'Z') ||
			('a' <= c && c <= 'z'):
			continue
		}
		// Otherwise, go through the full escaping process.
		var buf bytes.Buffer
		xml.EscapeText(&buf, []byte(s))
		return buf.String()
	}
	return s
}

// ErrNotImplemented should be returned by optional interfaces if they
// want the original implementation to be used.
var ErrNotImplemented = errors.New("not implemented")

func (s *WebDAVService) findResourceType(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	if file.Type() == types.FileTypeFolder {
		return `<D:collection xmlns:D="DAV:"/>`, nil
	}
	return "", nil
}

func (s *WebDAVService) findDisplayName(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	return escapeXML(file.DisplayName()), nil
}

func (s *WebDAVService) findContentLength(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	return strconv.FormatInt(file.Size(), 10), nil
}

func (s *WebDAVService) findLastModified(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	return file.UpdatedAt().UTC().Format(http.TimeFormat), nil
}

func (s *WebDAVService) findCreationDate(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	return file.CreatedAt().UTC().Format(http.TimeFormat), nil
}

func (s *WebDAVService) findContentType(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	return s.mime.MimeDetector().TypeByName(file.DisplayName()), nil
}

func (s *WebDAVService) findETag(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	return fmt.Sprintf(`"%s"`, hashid.EncodeEntityID(s.hasher, file.PrimaryEntityID())), nil
}

func (s *WebDAVService) findQuotaUsedBytes(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	requester := trans.FromContext(ctx)
	if file.Owner().Id != requester.Id {
		return "", ErrNotImplemented
	}
	capacity, err := fm.Capacity(ctx)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(capacity.Used, 10), nil
}

func (s *WebDAVService) findQuotaAvailableBytes(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	requester := trans.FromContext(ctx)
	if file.Owner().Id != requester.Id {
		return "", ErrNotImplemented
	}
	capacity, err := fm.Capacity(ctx)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(capacity.Total-capacity.Used, 10), nil
}

func (s *WebDAVService) findSupportedLock(ctx context.Context, fm manager.FileManager, file fs.File) (string, error) {
	return `` +
		`<D:lockentry xmlns:D="DAV:">` +
		`<D:lockscope><D:exclusive/></D:lockscope>` +
		`<D:locktype><D:write/></D:locktype>` +
		`</D:lockentry>`, nil
}
