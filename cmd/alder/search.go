package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"alder/internal/buildkit"
	"alder/internal/search"
	"alder/internal/style"
)

const searchUsage = "usage: alder search <query> [--json] [--limit N]"

type searchJSON struct {
	Query   string          `json:"query"`
	Mode    string          `json:"mode"`
	Note    string          `json:"note"`
	Results []searchRowJSON `json:"results"`
}

type searchRowJSON struct {
	Name      string `json:"name"`
	Module    string `json:"module"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
	Source    string `json:"source"`
	Effect    string `json:"effect,omitempty"`
	Match     string `json:"match,omitempty"`
	Args      *int   `json:"args,omitempty"` // set in produces mode, including 0
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Col       int    `json:"col,omitempty"`
}

// runSearchTo is `alder search <query> [--json] [--limit N]`: the standard library's declarations
// and, inside an app that type-checks, the app's, found by name or by type. A query never fails;
// only a usage error or a type query that does not parse exits 2.
func runSearchTo(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder search: internal error: %v\n", r)
			code = 2
		}
	}()
	var words []string
	jsonOut, limit := false, 0
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--json":
			jsonOut = true
		case a == "--limit":
			i++
			n := 0
			var err error
			if i < len(args) {
				n, err = strconv.Atoi(args[i])
			}
			if i >= len(args) || err != nil || n < 1 {
				fmt.Fprintln(stderr, searchUsage)
				return 2
			}
			limit = n
		case strings.HasPrefix(a, "--"):
			fmt.Fprintln(stderr, searchUsage)
			return 2
		default:
			words = append(words, a)
		}
	}
	if len(words) == 0 {
		fmt.Fprintln(stderr, searchUsage)
		return 2
	}

	ix, err := searchIndex(stderr)
	if err != nil {
		fmt.Fprintf(stderr, "alder search: %v\n", err)
		return 2
	}
	resp := ix.Search(strings.Join(words, " "), search.Options{Limit: limit})
	if resp.Bad {
		fmt.Fprintf(stderr, "alder search: %s\n", resp.Note)
		return 2
	}
	if resp.Note != "" {
		fmt.Fprintf(stderr, "alder search: %s\n", resp.Note)
	} else if len(resp.Results) == 0 {
		fmt.Fprintln(stderr, "alder search: no results")
	}

	if jsonOut {
		out := searchJSON{Query: resp.Query, Mode: resp.Mode, Note: resp.Note, Results: make([]searchRowJSON, 0, len(resp.Results))}
		for _, h := range resp.Results {
			e := h.Entry
			row := searchRowJSON{Name: e.Name, Module: e.Module, Kind: string(e.Kind), Signature: e.Signature, Doc: e.Doc,
				Source: e.Source, Effect: e.Effect, Match: h.Match, File: filepath.ToSlash(e.File), Line: e.Line, Col: e.Col}
			if resp.Mode == "produces" {
				n := h.Args
				row.Args = &n
			}
			out.Results = append(out.Results, row)
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false) // signatures hold `->`; a script reads them as written
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "alder search: %v\n", err)
			return 2
		}
		stdout.Write(b.Bytes())
		return 0
	}
	if len(resp.Results) == 0 {
		return 0
	}
	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTYPE\tFROM\tDOC")
	for _, h := range resp.Results {
		e := h.Entry
		name := e.Module + "." + e.Name
		if e.Module == "Prelude" {
			name = e.Name
		}
		from := e.Source
		if e.Effect != "" {
			from += " " + e.Effect
		}
		doc, _, _ := strings.Cut(e.Doc, "\n")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, e.Signature, from, doc)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(stderr, "alder search: %v\n", err)
		return 2
	}
	lines := strings.Split(strings.TrimRight(table.String(), "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ") // no padding after an empty DOC
	}
	fmt.Fprintf(stdout, "%s\n%s\n", style.For(stdout).Bold(lines[0]), strings.Join(lines[1:], "\n"))
	return 0
}

// searchIndex is the index to search: the standard library plus the app holding the working
// directory, when there is one and it type-checks; otherwise the standard library alone, with a
// note on stderr when an app was found but left out.
func searchIndex(stderr io.Writer) (*search.Index, error) {
	wd, err := os.Getwd()
	if err != nil {
		return search.Std()
	}
	root := findAppRoot(wd)
	if root == "" {
		return search.Std()
	}
	// LoadChecked, not checkedApp: schema drift must not hide the app's declarations.
	prog, res, bag, ioErrs := buildkit.LoadChecked(root, nil)
	if len(ioErrs) > 0 || res == nil || bag.HasErrors() {
		fmt.Fprintln(stderr, "alder search: the app does not type-check, so its declarations are left out; run alder check")
		return search.Std()
	}
	return search.Build(prog, res), nil
}
