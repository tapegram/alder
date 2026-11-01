package gen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// decodeDevMap lowers the app in srcs (file name to source) and decodes its dev map.
func decodeDevMap(t *testing.T, srcs map[string]string) devMap {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range srcs {
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var m devMap
	if err := json.Unmarshal([]byte(devMapJSON(lowerDir(t, dir))), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDevMapJSON_SQLNames(t *testing.T) {
	m := decodeDevMap(t, map[string]string{"Book.alder": `module Book

type BookId = brand Text

table Book {
  id: BookId @id,
  title: Text,
  pages: Int,
  rating: Maybe Float,
}
`})
	if len(m.Tables) != 1 {
		t.Fatalf("tables = %+v", m.Tables)
	}
	tb := m.Tables[0]
	if tb.Name != "Book.Book" || tb.SQL != "Book__Book" || tb.Silo != "" {
		t.Errorf("table = %+v", tb)
	}
	want := []devCol{
		{Name: "id", Type: "Text", SQL: "id", SQLType: "TEXT"},
		{Name: "title", Type: "Text", SQL: "title", SQLType: "TEXT"},
		{Name: "pages", Type: "Int", SQL: "pages", SQLType: "INTEGER"},
		{Name: "rating", Type: "Maybe Float", SQL: "rating", SQLType: "REAL"},
	}
	if len(tb.Cols) != len(want) {
		t.Fatalf("cols = %+v", tb.Cols)
	}
	for i, w := range want {
		if tb.Cols[i] != w {
			t.Errorf("cols[%d] = %+v, want %+v", i, tb.Cols[i], w)
		}
	}
}

func TestDevMapJSON_Silos(t *testing.T) {
	dir := "../../testdata/golden/gen_silos"
	var m devMap
	if err := json.Unmarshal([]byte(devMapJSON(lowerDir(t, dir))), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Silos) != 1 {
		t.Fatalf("silos = %+v", m.Silos)
	}
	s := m.Silos[0]
	if s.Key != "org" || s.Ref != "Org.Org" || len(s.Tables) != 1 || s.Tables[0] != "Project.Project" || s.Path != "silos/org/{value}.db" {
		t.Errorf("silo = %+v", s)
	}
	for _, tb := range m.Tables {
		want := ""
		if tb.Name == "Project.Project" {
			want = "org"
		}
		if tb.Silo != want {
			t.Errorf("%s: silo = %q, want %q", tb.Name, tb.Silo, want)
		}
	}
}
