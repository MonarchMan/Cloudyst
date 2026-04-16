package webdav

import (
	"bytes"
	"common/constants"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_lockinfo
type LockInfo struct {
	XMLName   xml.Name  `xml:"lockinfo"`
	Exclusive *struct{} `xml:"lockscope>exclusive"`
	Shared    *struct{} `xml:"lockscope>shared"`
	Write     *struct{} `xml:"locktype>write"`
	Owner     owner     `xml:"owner"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_owner
type owner struct {
	InnerXML string `xml:",innerxml"`
}

func ReadLockInfo(r io.Reader) (li LockInfo, status int, err error) {
	c := &countingReader{r: r}
	if err = xml.NewDecoder(c).Decode(&li); err != nil {
		if err == io.EOF {
			if c.n == 0 {
				// An empty body means to refresh the lock.
				// http://www.webdav.org/specs/rfc4918.html#refreshing-locks
				return LockInfo{}, 0, nil
			}
			err = ErrInvalidLockInfo
		}
		return LockInfo{}, http.StatusBadRequest, err
	}
	// We only support exclusive (non-shared) write locks. In practice, these are
	// the only types of locks that seem to matter.
	if li.Exclusive == nil || li.Shared != nil || li.Write == nil {
		return LockInfo{}, http.StatusNotImplemented, ErrUnsupportedLockInfo
	}
	return li, 0, nil
}

type countingReader struct {
	n int
	r io.Reader
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// Next returns the next token, if any, in the XML stream of d.
// RFC 4918 requires to ignore comments, processing instructions
// and directives.
// http://www.webdav.org/specs/rfc4918.html#property_values
// http://www.webdav.org/specs/rfc4918.html#xml-extensibility
func next(d *xml.Decoder) (xml.Token, error) {
	for {
		t, err := d.Token()
		if err != nil {
			return t, err
		}
		switch t.(type) {
		case xml.Comment, xml.Directive, xml.ProcInst:
			continue
		default:
			return t, nil
		}
	}
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_prop (for Propfind)
type propfindProps []xml.Name

// UnmarshalXML appends the property names enclosed within start to pn.
//
// It returns an error if start does not contain any properties or if
// properties contain values. Character data between properties is ignored.
func (pn *propfindProps) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		t, err := next(d)
		if err != nil {
			return err
		}
		switch t.(type) {
		case xml.EndElement:
			if len(*pn) == 0 {
				return fmt.Errorf("%s must not be empty", start.Name.Local)
			}
			return nil
		case xml.StartElement:
			name := t.(xml.StartElement).Name
			t, err = next(d)
			if err != nil {
				return err
			}
			if _, ok := t.(xml.EndElement); !ok {
				return fmt.Errorf("unexpected token %T", t)
			}
			*pn = append(*pn, xml.Name(name))
		}
	}
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propfind
type Propfind struct {
	XMLName  xml.Name      `xml:"DAV: propfind"`
	Allprop  *struct{}     `xml:"DAV: allprop"`
	Propname *struct{}     `xml:"DAV: propname"`
	Prop     propfindProps `xml:"DAV: prop"`
	Include  propfindProps `xml:"DAV: include"`
}

func ReadPropfind(r io.Reader) (pf Propfind, status int, err error) {
	c := countingReader{r: r}
	if err = xml.NewDecoder(&c).Decode(&pf); err != nil {
		if err == io.EOF {
			if c.n == 0 {
				// An empty body means to propfind allprop.
				// http://www.webdav.org/specs/rfc4918.html#METHOD_PROPFIND
				return Propfind{Allprop: new(struct{})}, 0, nil
			}
			err = ErrInvalidPropfind
		}
		return Propfind{}, http.StatusBadRequest, err
	}

	if pf.Allprop == nil && pf.Include != nil {
		return Propfind{}, http.StatusBadRequest, ErrInvalidPropfind
	}
	if pf.Allprop != nil && (pf.Prop != nil || pf.Propname != nil) {
		return Propfind{}, http.StatusBadRequest, ErrInvalidPropfind
	}
	if pf.Prop != nil && pf.Propname != nil {
		return Propfind{}, http.StatusBadRequest, ErrInvalidPropfind
	}
	if pf.Propname == nil && pf.Allprop == nil && pf.Prop == nil {
		return Propfind{}, http.StatusBadRequest, ErrInvalidPropfind
	}
	return pf, 0, nil
}

// Property represents a single DAV resource property as defined in RFC 4918.
// See http://www.webdav.org/specs/rfc4918.html#data.model.for.resource.properties
type Property struct {
	// XMLName is the fully qualified name that identifies this property.
	XMLName xml.Name

	// Lang is an optional xml:lang attribute.
	Lang string `xml:"xml:lang,attr,omitempty"`

	// InnerXML contains the XML representation of the property value.
	// See http://www.webdav.org/specs/rfc4918.html#property_values
	//
	// Property values of complex type or mixed-content must have fully
	// expanded XML namespaces or be self-contained with according
	// XML namespace declarations. They must not rely on any XML
	// namespace declarations within the scope of the XML document,
	// even including the DAV: namespace.
	InnerXML []byte `xml:",innerxml"`
}

// xmlProperty is the same as the Property type except it holds an xml.Name
// instead of an xml.Name.
type xmlProperty struct {
	XMLName  xml.Name
	Lang     string `xml:"xml:lang,attr,omitempty"`
	InnerXML []byte `xml:",innerxml"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_error
// See MultistatusWriter for the "D:" namespace prefix.
type xmlError struct {
	XMLName  xml.Name `xml:"D:error"`
	InnerXML []byte   `xml:",innerxml"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propstat
// See MultistatusWriter for the "D:" namespace prefix.
type propstat struct {
	Prop                []Property `xml:"D:prop>_ignored_"`
	Status              string     `xml:"D:status"`
	Error               *xmlError  `xml:"D:error"`
	ResponseDescription string     `xml:"D:responsedescription,omitempty"`
}

// xmlPropstat is the same as the propstat type except it holds an xml.Name
// instead of an xml.Name.
type xmlPropstat struct {
	Prop                []xmlProperty `xml:"D:prop>_ignored_"`
	Status              string        `xml:"D:status"`
	Error               *xmlError     `xml:"D:error"`
	ResponseDescription string        `xml:"D:responsedescription,omitempty"`
}

// MarshalXML prepends the "D:" namespace prefix on properties in the DAV: namespace
// before encoding. See multistatusWriter.
func (ps propstat) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	// Convert from a propstat to an xmlPropstat.
	xmlPs := xmlPropstat{
		Prop:                make([]xmlProperty, len(ps.Prop)),
		Status:              ps.Status,
		Error:               ps.Error,
		ResponseDescription: ps.ResponseDescription,
	}
	for k, prop := range ps.Prop {
		xmlPs.Prop[k] = xmlProperty{
			XMLName:  xml.Name(prop.XMLName),
			Lang:     prop.Lang,
			InnerXML: prop.InnerXML,
		}
	}

	for k, prop := range xmlPs.Prop {
		if prop.XMLName.Space == "DAV:" {
			prop.XMLName = xml.Name{Space: "", Local: "D:" + prop.XMLName.Local}
			xmlPs.Prop[k] = prop
		}
	}
	// Distinct type to avoid infinite recursion of MarshalXML.
	type newpropstat xmlPropstat
	return e.EncodeElement(newpropstat(xmlPs), start)
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_response
// See MultistatusWriter for the "D:" namespace prefix.
type response struct {
	XMLName             xml.Name   `xml:"D:response"`
	Href                []string   `xml:"D:href"`
	Propstat            []propstat `xml:"D:propstat"`
	Status              string     `xml:"D:status,omitempty"`
	Error               *xmlError  `xml:"D:error"`
	ResponseDescription string     `xml:"D:responsedescription,omitempty"`
}

// MultistatusWriter marshals one or more Responses into a XML
// multistatus response.
// See http://www.webdav.org/specs/rfc4918.html#ELEMENT_multistatus
// TODO(rsto, mpl): As a workaround, the "D:" namespace prefix, defined as
// "DAV:" on this element, is prepended on the nested response, as well as on all
// its nested elements. All property names in the DAV: namespace are prefixed as
// well. This is because some versions of Mini-Redirector (on windows 7) ignore
// elements with a default namespace (no prefixed namespace). A less intrusive fix
// should be possible after golang.org/cl/11074. See https://golang.org/issue/11177
type MultistatusWriter struct {
	// ResponseDescription contains the optional responsedescription
	// of the multistatus XML element. Only the latest content before
	// Close will be emitted. Empty response descriptions are not
	// written.
	responseDescription string

	W   http.ResponseWriter
	enc *xml.Encoder
}

// Write validates and emits a DAV response as part of a multistatus response
// element.
//
// It sets the HTTP status code of its underlying http.ResponseWriter to 207
// (Multi-Status) and populates the Content-Type header. If r is the
// first, valid response to be written, Write prepends the XML representation
// of r with a multistatus tag. Callers must call close after the last response
// has been written.
func (w *MultistatusWriter) Write(r *response) error {
	switch len(r.Href) {
	case 0:
		return ErrInvalidResponse
	case 1:
		if len(r.Propstat) > 0 != (r.Status == "") {
			return ErrInvalidResponse
		}
	default:
		if len(r.Propstat) > 0 || r.Status == "" {
			return ErrInvalidResponse
		}
	}
	err := w.writeHeader()
	if err != nil {
		return err
	}
	return w.enc.Encode(r)
}

// writeHeader writes a XML multistatus start element on W's underlying
// http.ResponseWriter and returns the result of the Write operation.
// After the first Write attempt, writeHeader becomes a no-op.
func (w *MultistatusWriter) writeHeader() error {
	if w.enc != nil {
		return nil
	}
	w.W.Header().Add("Content-Type", "text/xml; charset=utf-8")
	w.W.WriteHeader(constants.StatusMulti)
	_, err := fmt.Fprintf(w.W, `<?xml version="1.0" encoding="UTF-8"?>`)
	if err != nil {
		return err
	}
	w.enc = xml.NewEncoder(w.W)
	return w.enc.EncodeToken(xml.StartElement{
		Name: xml.Name{
			Space: "DAV:",
			Local: "multistatus",
		},
		Attr: []xml.Attr{{
			Name:  xml.Name{Space: "xmlns", Local: "D"},
			Value: "DAV:",
		}},
	})
}

// Close completes the marshalling of the multistatus response. It returns
// an error if the multistatus response could not be completed. If both the
// return value and field enc of W are nil, then no multistatus response has
// been written.
func (w *MultistatusWriter) Close() error {
	if w.enc == nil {
		return nil
	}
	var end []xml.Token
	if w.responseDescription != "" {
		name := xml.Name{Space: "DAV:", Local: "responsedescription"}
		end = append(end,
			xml.StartElement{Name: name},
			xml.CharData(w.responseDescription),
			xml.EndElement{Name: name},
		)
	}
	end = append(end, xml.EndElement{
		Name: xml.Name{Space: "DAV:", Local: "multistatus"},
	})
	for _, t := range end {
		err := w.enc.EncodeToken(t)
		if err != nil {
			return err
		}
	}
	return w.enc.Flush()
}

var xmlLangName = xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "lang"}

func xmlLang(s xml.StartElement, d string) string {
	for _, attr := range s.Attr {
		if attr.Name == xmlLangName {
			return attr.Value
		}
	}
	return d
}

type xmlValue []byte

func (v *xmlValue) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// The XML value of a property can be arbitrary, mixed-content XML.
	// To make sure that the unmarshalled value contains all required
	// namespaces, we encode all the property value XML tokens into a
	// buffer. This forces the encoder to redeclare any used namespaces.
	var b bytes.Buffer
	e := xml.NewEncoder(&b)
	for {
		t, err := next(d)
		if err != nil {
			return err
		}
		if e, ok := t.(xml.EndElement); ok && e.Name == start.Name {
			break
		}
		if err = e.EncodeToken(t); err != nil {
			return err
		}
	}
	err := e.Flush()
	if err != nil {
		return err
	}
	*v = b.Bytes()
	return nil
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_prop (for proppatch)
type ProppatchProps []Property

// UnmarshalXML appends the property names and values enclosed within start
// to ps.
//
// An xml:lang attribute that is defined either on the DAV:prop or property
// name XML element is propagated to the property's Lang field.
//
// UnmarshalXML returns an error if start does not contain any properties or if
// property values contain syntactically incorrect XML.
func (ps *ProppatchProps) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	lang := xmlLang(start, "")
	for {
		t, err := next(d)
		if err != nil {
			return err
		}
		switch elem := t.(type) {
		case xml.EndElement:
			if len(*ps) == 0 {
				return fmt.Errorf("%s must not be empty", start.Name.Local)
			}
			return nil
		case xml.StartElement:
			p := Property{
				XMLName: xml.Name(t.(xml.StartElement).Name),
				Lang:    xmlLang(t.(xml.StartElement), lang),
			}
			err = d.DecodeElement(((*xmlValue)(&p.InnerXML)), &elem)
			if err != nil {
				return err
			}
			*ps = append(*ps, p)
		}
	}
}
