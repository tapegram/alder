// internal/types/stdlib.go
package types

import (
	"fmt"
	"io/fs"
	"strings"

	"alder/internal/syntax"
	"alder/stdlib"
)

// LoadStd parses every embedded stdlib signature file, keyed by module name.
func LoadStd() (map[string]*syntax.Module, error) {
	out := map[string]*syntax.Module{}
	entries, err := fs.ReadDir(stdlib.FS, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".alder") {
			continue
		}
		src, err := stdlib.FS.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		m, ds := syntax.ParseSignatures("stdlib/"+e.Name(), src)
		if len(ds) > 0 {
			return nil, fmt.Errorf("stdlib %s: %s", e.Name(), ds[0].Message)
		}
		out[m.Name] = m
	}
	return out, nil
}
