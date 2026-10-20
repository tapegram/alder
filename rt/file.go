package rt

import (
	"net/textproto"
	"strings"
)

// File is a page's answer when its view returns File (chapter 6, "File pages"): written as
// given, with no doctype, head tags or panel. Status 0 means 200; ContentType "" means
// application/octet-stream. Headers keep what the page set, in order; the router sends only
// the names FileHeaderAllowed accepts.
type File struct {
	Status      int
	ContentType string
	Headers     [][2]string
	Body        []byte
}

var fileHeaders = map[string]bool{
	"Content-Disposition": true, "Cache-Control": true, "Etag": true,
	"Last-Modified": true, "Retry-After": true, "Content-Language": true,
}

// FileHeaderAllowed reports whether a file page may set header name, and its canonical form.
// The checker (E0035) and the router share it.
func FileHeaderAllowed(name string) (string, bool) {
	if name == "" || strings.ContainsAny(name, " \t\r\n:") {
		return "", false
	}
	c := textproto.CanonicalMIMEHeaderKey(name)
	return c, fileHeaders[c] || (strings.HasPrefix(c, "X-") && len(c) > 2)
}
