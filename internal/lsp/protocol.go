package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
)

// Position is a place in a document: a 0-based line and a 0-based byte offset in that line
// (positionEncoding utf-8).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
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

type initializeParams struct {
	RootURI  string `json:"rootUri"`
	RootPath string `json:"rootPath"`
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

func capabilities() map[string]any {
	return map[string]any{
		"positionEncoding":       "utf-8",
		"textDocumentSync":       map[string]any{"openClose": true, "change": 1},
		"hoverProvider":          true,
		"definitionProvider":     true,
		"referencesProvider":     true,
		"completionProvider":     map[string]any{"triggerCharacters": []string{".", "<", "?"}},
		"codeActionProvider":     map[string]any{"codeActionKinds": []string{"quickfix", "refactor"}},
		"executeCommandProvider": map[string]any{"commands": []string{"alder.explain"}},
	}
}
