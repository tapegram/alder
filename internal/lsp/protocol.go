package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"

	"alder/internal/scratch"
)

// Position is a place in a document: a 0-based line and a 0-based character offset in that line,
// in the negotiated positionEncoding (utf-8 bytes or utf-16 code units; see encoding.go). Inside
// the server a Position's offset is always a byte offset: it is converted where it crosses the wire.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// textDocumentPositionParams are the params of hover and definition: a place in a document.
type textDocumentPositionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position Position `json:"position"`
}

type Diagnostic struct {
	Range    Range           `json:"range"`
	Severity int             `json:"severity"`
	Code     string          `json:"code"`
	Source   string          `json:"source"`
	Message  string          `json:"message"`
	Data     *diagnosticData `json:"data,omitempty"`
}

// diagnosticData carries the compiler's fix for a diagnostic, for the quick-fix code action.
type diagnosticData struct {
	Fix *TextEdit `json:"fix,omitempty"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// scratchParams are the params of alder/scratch, the server's notification of a scratch file's
// run: "types" once the file has checked (before building), then "values" when the run ends.
type scratchParams struct {
	URI     string                `json:"uri"`
	Version int                   `json:"version"`
	Phase   string                `json:"phase"`   // "types" | "values"
	Watches []scratch.WatchResult `json:"watches"` // never null
	Ran     bool                  `json:"ran"`
	Note    string                `json:"note,omitempty"` // FileResult.Note, but not AppErrorsNote: the W0007 warning says that
	Warning string                `json:"warning,omitempty"`
	Error   string                `json:"error,omitempty"` // FileResult.Err
}

type initializeParams struct {
	RootURI      string `json:"rootUri"`
	RootPath     string `json:"rootPath"`
	Capabilities struct {
		Workspace struct {
			InlayHint struct {
				RefreshSupport bool `json:"refreshSupport"`
			} `json:"inlayHint"`
		} `json:"workspace"`
		General struct {
			PositionEncodings []string `json:"positionEncodings"`
		} `json:"general"`
	} `json:"capabilities"`
}

type cancelParams struct {
	ID json.RawMessage `json:"id"`
}

// uriPath is the file path a file:// URI names.
func uriPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("not a file URI: %s", uri)
	}
	return filepath.FromSlash(u.Path), nil
}

func capabilities(enc posEncoding) map[string]any {
	return map[string]any{
		"positionEncoding":           enc.String(),
		"textDocumentSync":           map[string]any{"openClose": true, "change": 1, "save": map[string]any{"includeText": false}},
		"hoverProvider":              true,
		"documentFormattingProvider": true,
		"inlayHintProvider":          true,
		"definitionProvider":         true,
		"referencesProvider":         true,
		"completionProvider":         map[string]any{"triggerCharacters": []string{".", "<", "?"}},
		"codeActionProvider":         map[string]any{"codeActionKinds": []string{"quickfix", "refactor"}},
		"executeCommandProvider":     map[string]any{"commands": []string{"alder.explain"}},
		"experimental":               map[string]any{"alderScratch": true},
	}
}
