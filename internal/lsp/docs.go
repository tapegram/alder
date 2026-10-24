package lsp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"alder/internal/scratch"
)

// doc is a document the client has open: its text replaces the file's on disk.
type doc struct {
	URI     string
	Rel     string // "app/Posts/Index.alder", relative to the real root
	Version int
	Text    string
}

// docs are the open documents, by rel. Notifications change them on the reader goroutine; the
// analysis goroutine and the workers read them.
type docs struct {
	mu    sync.Mutex
	byRel map[string]*doc
}

func (d *docs) put(x *doc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.byRel == nil {
		d.byRel = map[string]*doc{}
	}
	d.byRel[x.Rel] = x
}

func (d *docs) remove(rel string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.byRel, rel)
}

// get is the open document for rel, or nil. A doc is never changed once stored, so the caller
// may read it without the lock.
func (d *docs) get(rel string) *doc {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.byRel[rel]
}

// scratchRels are the rels of the open scratch documents, sorted.
func (d *docs) scratchRels() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var rels []string
	for rel := range d.byRel {
		if scratch.IsScratchFile(rel) {
			rels = append(rels, rel)
		}
	}
	slices.Sort(rels)
	return rels
}

// overlay is the texts of the open documents, keyed by rel, for names.LoadOverlay.
func (d *docs) overlay() map[string][]byte {
	over, _ := d.snapshot()
	return over
}

// snapshot is overlay and the version of each open document, taken together.
func (d *docs) snapshot() (map[string][]byte, map[string]int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	over, vers := make(map[string][]byte, len(d.byRel)), make(map[string]int, len(d.byRel))
	for rel, x := range d.byRel {
		if scratch.IsScratchFile(rel) {
			continue // not valid ordinary Alder: kept out of the analysis until the LSP learns watches
		}
		over[rel], vers[rel] = []byte(x.Text), x.Version
	}
	return over, vers
}

// rel is the path a file:// URI names, relative to the real root and slash-separated
// ("app/Posts/Index.alder"); false when the URI is not a file under the root. Symbolic links
// are resolved, so a file named through a link to the root (or into it) has its real key. A
// file that does not exist yet is resolved through its nearest existing directory.
func (s *server) rel(uri string) (string, bool) {
	if s.rootReal == "" {
		return "", false
	}
	path, err := uriPath(uri)
	if err != nil || !filepath.IsAbs(path) {
		return "", false
	}
	real, err := evalExisting(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	r, err := filepath.Rel(s.rootReal, real)
	if err != nil || r == "." || !filepath.IsLocal(r) {
		return "", false // the key must never leave the root: generate.Materialise writes under it
	}
	return filepath.ToSlash(r), true
}

// evalExisting is filepath.EvalSymlinks of path's longest existing prefix, with the rest
// (names that do not exist yet) joined back on.
func evalExisting(path string) (string, error) {
	rest := ""
	for {
		real, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(real, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		rest = filepath.Join(filepath.Base(path), rest)
		path = parent
	}
}

// uriOf is the URI the client knows rel by: an open document's own URI, else the root as the
// client spelled it joined with rel.
func (s *server) uriOf(rel string) string {
	if x := s.docs.get(rel); x != nil {
		return x.URI
	}
	return "file://" + filepath.ToSlash(filepath.Join(s.root, filepath.FromSlash(rel)))
}

// text is rel's current text: the open document's, else the file's on disk ("" when unreadable).
func (s *server) text(rel string) string {
	if x := s.docs.get(rel); x != nil {
		return x.Text
	}
	b, _ := os.ReadFile(filepath.Join(s.rootReal, filepath.FromSlash(rel)))
	return string(b)
}

type textDocumentItem struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
	Text    string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	ContentChanges []struct {
		Text string `json:"text"`
	} `json:"contentChanges"`
}

type didSaveParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

type didCloseParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

func didOpen(s *server, params json.RawMessage) {
	var p didOpenParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return
	}
	s.docs.put(&doc{URI: p.TextDocument.URI, Rel: rel, Version: p.TextDocument.Version, Text: p.TextDocument.Text})
	s.schedule()
	if scratch.IsScratchFile(rel) {
		s.runScratch(rel)
	}
}

// didChange takes the last change's text: the server asks for full sync, so each change is the
// whole document.
func didChange(s *server, params json.RawMessage) {
	var p didChangeParams
	if json.Unmarshal(params, &p) != nil || len(p.ContentChanges) == 0 {
		return
	}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return
	}
	s.docs.put(&doc{URI: p.TextDocument.URI, Rel: rel, Version: p.TextDocument.Version,
		Text: p.ContentChanges[len(p.ContentChanges)-1].Text})
	s.schedule()
}

// didClose forgets the document, so the file on disk applies again; a scratch document's run
// stops and its diagnostics clear.
func didClose(s *server, params json.RawMessage) {
	var p didCloseParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return
	}
	s.docs.remove(rel)
	s.schedule()
	if scratch.IsScratchFile(rel) {
		s.stopScratch(rel, p.TextDocument.URI)
	}
}

// didSave runs a saved scratch document; a save of any other file under the root reruns every
// open scratch document, which may use what it changed (and the poll that sees the save's
// write does not rerun them again). A change alone never runs one.
func didSave(s *server, params json.RawMessage) {
	var p didSaveParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return
	}
	if scratch.IsScratchFile(rel) {
		if s.docs.get(rel) != nil {
			s.runScratch(rel)
		}
		return
	}
	s.saveRerunScratch()
}
