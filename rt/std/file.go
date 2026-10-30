package std

import (
	"unsafe"

	"alder/rt"
	"alder/rt/html"
)

// bodyOf is s's bytes as a File body, without a copy (#37: a large rendered body was held
// twice). Sound because a []byte that Alder code can see is never written to (rt.File.Body);
// the slice's capacity is its length, so an append reallocates. "" gives an empty non-nil body.
func bodyOf(s string) []byte {
	if s == "" {
		return []byte{}
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// FileText is File.text: a 200 answer of contentType with body as UTF-8.
func FileText(r struct {
	ContentType string
	Body        string
}) rt.File {
	return rt.File{Status: 200, ContentType: r.ContentType, Body: bodyOf(r.Body)}
}

// FileBytes is File.bytes.
func FileBytes(r struct {
	ContentType string
	Body        []byte
}) rt.File {
	return rt.File{Status: 200, ContentType: r.ContentType, Body: r.Body}
}

// FileHtml is File.html: the node rendered as view would render it, written as-is.
func FileHtml(n html.Node) rt.File {
	return rt.File{Status: 200, ContentType: "text/html; charset=utf-8", Body: bodyOf(html.Render(n))}
}

// FileJson is File.json: the value's compact encoding, as application/json.
func FileJson(v rt.JsonValue) rt.File {
	return rt.File{Status: 200, ContentType: "application/json", Body: bodyOf(rt.JsonEncode(v))}
}

// FileWithStatus is File.withStatus; a code outside 200..599 is answered as the 500 page by
// the router (a literal one is E0003).
func FileWithStatus(code int64, f rt.File) rt.File { f.Status = int(code); return f }

// FileWithHeader is File.withHeader: appended, never aliasing f's slice.
func FileWithHeader(h struct {
	Name  string
	Value string
}, f rt.File) rt.File {
	hs := make([][2]string, len(f.Headers), len(f.Headers)+1)
	copy(hs, f.Headers)
	f.Headers = append(hs, [2]string{h.Name, h.Value})
	return f
}

// FileStatus is File.status: 200 when the page set none.
func FileStatus(f rt.File) int64 {
	if f.Status == 0 {
		return 200
	}
	return int64(f.Status)
}

// FileContentType is File.contentType: application/octet-stream when the page set none.
func FileContentType(f rt.File) string {
	if f.ContentType == "" {
		return "application/octet-stream"
	}
	return f.ContentType
}

// FileBody is File.body.
func FileBody(f rt.File) []byte { return f.Body }
