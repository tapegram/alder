// Package migrate is the migration generator behind `alder migrate`: the canonical schema of an
// app's tables (Schema, written to migrations/schema.json as the snapshot), the deterministic
// diff between two schemas (Diff), the SQLite statements of each step (SQL, including the table
// rebuild), and the migration file (Plan, Render, ParseHeader, NextName).
//
// The ground truth is the runtime's: a Schema's tables are db.TableDescs, its hash is Ensure's
// (the sorted db.CreateStmt texts), and the steps of a migration take a database whose tables
// are exactly `old`'s CREATE TABLE statements to one whose tables are exactly `new`'s — text for
// text, so a migrated database and a freshly created one are indistinguishable.
package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
	"alder/rt/db"
)

// Schema is the canonical description of an app's tables: what migrations are diffed over.
type Schema struct {
	Format string  `json:"format"` // db.Format
	Tables []Table `json:"tables"` // sorted by Name
}

// Table is one `table` declaration as stored.
type Table struct {
	Name        string   `json:"name"` // "Post.Post" (module-qualified, as db.TableDesc.Name)
	ID          string   `json:"id"`
	Created     string   `json:"created,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	Columns     []Column `json:"columns"` // declaration order
	RenamedFrom string   `json:"-"`       // from @renamed, module-qualified ("Article.Article"); never stored
}

// Column is one field of a table as stored. Type is written to JSON by name ("Text", "Int", …)
// and OnDelete as "cascade"/"setNull" (omitted for the default @restrict).
type Column struct {
	Name        string      `json:"name"`
	Type        db.ColType  `json:"type"`
	Nullable    bool        `json:"nullable,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Unique      bool        `json:"unique,omitempty"`
	Ref         string      `json:"ref,omitempty"`
	OnDelete    db.OnDelete `json:"onDelete,omitempty"`
	Default     *string     `json:"default,omitempty"` // SQL literal: "0", "'Reader'", "NULL"
	RenamedFrom string      `json:"-"`
}

var colTypeNames = map[db.ColType]string{db.CText: "Text", db.CInt: "Int", db.CFloat: "Float", db.CBool: "Bool", db.CDate: "Date", db.CTime: "Time", db.CEnum: "Enum"}

var onDeleteNames = map[db.OnDelete]string{db.Restrict: "", db.Cascade: "cascade", db.SetNull: "setNull"}

// columnJSON is Column as written to schema.json.
type columnJSON struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Nullable bool     `json:"nullable,omitempty"`
	Enum     []string `json:"enum,omitempty"`
	Unique   bool     `json:"unique,omitempty"`
	Ref      string   `json:"ref,omitempty"`
	OnDelete string   `json:"onDelete,omitempty"`
	Default  *string  `json:"default,omitempty"`
}

// MarshalJSON writes c with its type and delete rule by name.
func (c Column) MarshalJSON() ([]byte, error) {
	t, ok := colTypeNames[c.Type]
	if !ok {
		return nil, fmt.Errorf("migrate: column %s: unknown column type %d", c.Name, c.Type)
	}
	od, ok := onDeleteNames[c.OnDelete]
	if !ok {
		return nil, fmt.Errorf("migrate: column %s: unknown delete rule %d", c.Name, c.OnDelete)
	}
	return json.Marshal(columnJSON{c.Name, t, c.Nullable, c.Enum, c.Unique, c.Ref, od, c.Default})
}

// UnmarshalJSON reads a column written by MarshalJSON.
func (c *Column) UnmarshalJSON(b []byte) error {
	var j columnJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*c = Column{Name: j.Name, Nullable: j.Nullable, Enum: j.Enum, Unique: j.Unique, Ref: j.Ref, Default: j.Default}
	found := false
	for k, v := range colTypeNames {
		if v == j.Type {
			c.Type, found = k, true
		}
	}
	if !found {
		return fmt.Errorf("column %s: unknown type %q", j.Name, j.Type)
	}
	found = false
	for k, v := range onDeleteNames {
		if v == j.OnDelete {
			c.OnDelete, found = k, true
		}
	}
	if !found {
		return fmt.Errorf("column %s: unknown delete rule %q", j.Name, j.OnDelete)
	}
	return nil
}

// FromProgram builds the Schema of a checked app (module order then declaration order, then
// sorted). A table's RenamedFrom is module-qualified like its Name: a table lives in the module
// named after it, so `table Post @renamed "Article"` was "Article.Article".
func FromProgram(p *ir.Program) Schema {
	s := Schema{Format: db.Format, Tables: []Table{}}
	for _, tb := range p.Tables {
		t := Table{Name: tb.Module + "." + tb.Name, ID: tb.IDField, Created: tb.Created, Updated: tb.Updated, Columns: []Column{}}
		if tb.RenamedFrom != "" {
			t.RenamedFrom = tb.RenamedFrom + "." + tb.RenamedFrom
		}
		for _, f := range tb.Record.Fields {
			c := columnOf(f.Type)
			c.Name = f.Name
			c.Unique = slices.Contains(tb.Unique, f.Name)
			c.Ref = tb.Refs[f.Name]
			switch tb.OnDelete[f.Name] {
			case "cascade":
				c.OnDelete = db.Cascade
			case "setNull":
				c.OnDelete = db.SetNull
			}
			if f.DefaultLit != nil {
				lit := *f.DefaultLit
				c.Default = &lit
			}
			c.RenamedFrom = f.RenamedFrom
			t.Columns = append(t.Columns, c)
		}
		s.Tables = append(s.Tables, t)
	}
	slices.SortFunc(s.Tables, func(a, b Table) int { return strings.Compare(a.Name, b.Name) })
	return s
}

// columnOf is the Type, Nullable and Enum of a column of (erased) type t: the storage mapping
// of rt/db, as internal/gen writes it into the table descriptors.
func columnOf(t types.Type) Column {
	var c Column
	if u, ok := t.(*types.Union); ok && u.Module == "Prelude" && u.Name == "Maybe" && len(u.Args) == 1 {
		c.Nullable, t = true, u.Args[0]
	}
	switch t := t.(type) {
	case *types.Brand:
		base := columnOf(t.Base)
		c.Type, c.Enum = base.Type, base.Enum
		return c
	case *types.Union:
		if t.Module != "Prelude" {
			c.Type = db.CEnum
			for _, ct := range t.Constructors() {
				c.Enum = append(c.Enum, ct.Name)
			}
			return c
		}
	case *types.Con:
		k, ok := map[string]db.ColType{"Text": db.CText, "Int": db.CInt, "Float": db.CFloat, "Bool": db.CBool, "Date": db.CDate, "Time": db.CTime}[t.Name]
		if ok {
			c.Type = k
			return c
		}
	case *types.App:
		if t.Con == "Id" || t.Con == "Column" {
			c.Type = db.CText
			return c
		}
	}
	panic(fmt.Sprintf("migrate: type %s cannot be stored in a table", types.String(t)))
}

// Desc is t as the runtime's table descriptor.
func (t Table) Desc() db.TableDesc {
	cols := make([]db.Column, len(t.Columns))
	for i, c := range t.Columns {
		cols[i] = db.Column{Name: c.Name, Type: c.Type, Nullable: c.Nullable, Enum: c.Enum, Unique: c.Unique, Ref: c.Ref, OnDelete: c.OnDelete}
	}
	return db.TableDesc{Name: t.Name, Columns: cols, ID: t.ID, Created: t.Created, Updated: t.Updated}
}

// byName is s's tables as descriptors by name, for db.CreateStmt's @ref lookups.
func (s Schema) byName() map[string]db.TableDesc {
	m := map[string]db.TableDesc{}
	for _, t := range s.Tables {
		m[t.Name] = t.Desc()
	}
	return m
}

// CreateStmts are s's CREATE TABLE statements (db.CreateStmt), in s's table order.
func (s Schema) CreateStmts() ([]string, error) {
	by := s.byName()
	out := make([]string, len(s.Tables))
	for i, t := range s.Tables {
		st, err := db.CreateStmt(t.Desc(), by)
		if err != nil {
			return nil, err
		}
		out[i] = st
	}
	return out, nil
}

// Hash is Ensure's hash of the schema (the sorted CREATE TABLE statements), so a snapshot and a
// live database can be compared.
func (s Schema) Hash() (string, error) {
	stmts, err := s.CreateStmts()
	if err != nil {
		return "", err
	}
	slices.Sort(stmts)
	sum := sha256.Sum256([]byte(strings.Join(stmts, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

// table is s's table named name, or nil.
func (s Schema) table(name string) *Table {
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	return nil
}

// ReadSnapshot reads a schema.json written by WriteSnapshot.
func ReadSnapshot(path string) (Schema, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Schema{}, err
	}
	var s Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return Schema{}, fmt.Errorf("%s: %w", path, err)
	}
	if s.Tables == nil {
		s.Tables = []Table{}
	}
	return s, nil
}

// WriteSnapshot writes s as indented JSON, its tables sorted by name, ending in a newline.
func WriteSnapshot(path string, s Schema) error {
	s.Tables = slices.Clone(s.Tables)
	if s.Tables == nil {
		s.Tables = []Table{}
	}
	slices.SortFunc(s.Tables, func(a, b Table) int { return strings.Compare(a.Name, b.Name) })
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
