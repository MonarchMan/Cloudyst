package webdav

import (
	"bytes"
	"encoding/xml"
	"file/internal/biz/filemanager/lock"
	"file/internal/pkg/webdav"
	"fmt"
	"io"
	"net/http"
	"time"
)

func writeLockInfo(w io.Writer, token string, ld lock.LockDetails) (int, error) {
	depth := "infinity"
	if ld.ZeroDepth {
		depth = "0"
	}
	timeout := ld.Duration / time.Second
	return fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"+
		"<D:prop xmlns:D=\"DAV:\"><D:lockdiscovery><D:activelock>\n"+
		"	<D:locktype><D:write/></D:locktype>\n"+
		"	<D:lockscope><D:exclusive/></D:lockscope>\n"+
		"	<D:depth>%s</D:depth>\n"+
		"	<D:owner>%s</D:owner>\n"+
		"	<D:timeout>Second-%d</D:timeout>\n"+
		"	<D:locktoken><D:href>%s</D:href></D:locktoken>\n"+
		"	<D:lockroot><D:href>/%s</D:href></D:lockroot>\n"+
		"</D:activelock></D:lockdiscovery></D:prop>",
		depth, ld.Owner.Application.InnerXML, timeout, escape(token), escape(ld.Root),
	)
}

func escape(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '&', '\'', '<', '>':
			b := bytes.NewBuffer(nil)
			xml.EscapeText(b, []byte(s))
			return b.String()
		}
	}
	return s
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_set
// http://www.webdav.org/specs/rfc4918.html#ELEMENT_remove
type setRemove struct {
	XMLName xml.Name
	Lang    string                `xml:"xml:lang,attr,omitempty"`
	Prop    webdav.ProppatchProps `xml:"DAV: prop"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propertyupdate
type propertyupdate struct {
	XMLName   xml.Name    `xml:"DAV: propertyupdate"`
	Lang      string      `xml:"xml:lang,attr,omitempty"`
	SetRemove []setRemove `xml:",any"`
}

func readProppatch(r io.Reader) (patches []Proppatch, status int, err error) {
	var pu propertyupdate
	if err = xml.NewDecoder(r).Decode(&pu); err != nil {
		return nil, http.StatusBadRequest, err
	}
	for _, op := range pu.SetRemove {
		remove := false
		switch op.XMLName {
		case xml.Name{Space: "DAV:", Local: "set"}:
			// No-op.
		case xml.Name{Space: "DAV:", Local: "remove"}:
			for _, p := range op.Prop {
				if len(p.InnerXML) > 0 {
					return nil, http.StatusBadRequest, webdav.ErrInvalidProppatch
				}
			}
			remove = true
		default:
			return nil, http.StatusBadRequest, webdav.ErrInvalidProppatch
		}
		patches = append(patches, Proppatch{Remove: remove, Props: op.Prop})
	}
	return patches, 0, nil
}
