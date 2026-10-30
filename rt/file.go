package rt

import (
	"net/textproto"
	"strings"
)

// File is a page's answer when its view returns File (chapter 6, "File pages"): written as
// given, with no doctype, head tags or panel. Status 0 means 200; ContentType "" means
// application/octet-stream. Headers keep what the page set, in order; the router sends only
// the names FileHeaderAllowed accepts.
//
// Body may share its storage with a Text (File.text, File.json and File.html alias the string's
// bytes rather than copy them, #37), so it must never be written to. That holds for every []byte
// Alder code can see: the Bytes functions build fresh slices, the router only Writes a body,
// and Test.fetch reads a recorded response. TestNoWritesToParameters (rt) pins the rule. A Go
// adapter is the one writer the compiler cannot check, so a capability call hands it rt.Fresh
// copies of its arguments (a File.body among them): adapters may write into their arguments and
// must not rely on sharing.
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
