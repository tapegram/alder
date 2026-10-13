// Package al is the vendored `alder-ui` design system (scripts/vendor-al.sh) as the runtime
// serves it: rt/web answers /static/al/al.css and /static/al/custom-elements.json from these
// embedded bytes, with a strong ETag computed from them (ETag), so a browser revalidates and a
// compiler upgrade's new stylesheet is seen at once.
package al

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"sync"
)

//go:embed al.css custom-elements.json
var files embed.FS

// Names are the served files, by name under /static/al/.
var Names = []string{"al.css", "custom-elements.json"}

type entry struct {
	data []byte
	etag string
}

var table = sync.OnceValue(func() map[string]entry {
	out := map[string]entry{}
	for _, name := range Names {
		b, err := files.ReadFile(name)
		if err != nil {
			panic("rt/al: " + err.Error())
		}
		sum := sha256.Sum256(b)
		out[name] = entry{b, `"` + hex.EncodeToString(sum[:16]) + `"`}
	}
	return out
})

// File is the embedded file name (one of Names) with its strong ETag, a quoted hash of its
// content; ok is false for any other name.
func File(name string) (data []byte, etag string, ok bool) {
	e, ok := table()[name]
	return e.data, e.etag, ok
}
