package data

import (
	"errors"
	"strings"
)

// The reasons Guard refuses a statement.
var (
	errNotRead    = errors.New("only select, with and explain run here (the database is opened read-only)")
	errStatements = errors.New("one statement at a time")
)

// readOnlyLeads are the first words of the statements Guard lets through.
var readOnlyLeads = []string{"select", "with", "explain"}

// identifierRune reports whether b may be part of a SQL keyword or bare identifier.
func identifierRune(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// Guard checks that stmt is one statement that reads: past leading whitespace and comments (`--`
// to the end of the line, `/* */`), its first word, in any case, is select, with or explain, and
// a `;` outside a string, a quoted identifier or a comment is followed by nothing but whitespace
// and comments. It is a courtesy that gives the reason up front: the database is opened
// read-only with query_only set, so SQLite refuses a write that got past it.
func Guard(stmt string) error {
	i := skipSpace(stmt, 0)
	j := i
	for j < len(stmt) && identifierRune(stmt[j]) {
		j++
	}
	word := strings.ToLower(stmt[i:j])
	ok := false
	for _, w := range readOnlyLeads {
		ok = ok || word == w
	}
	if !ok {
		return errNotRead
	}
	for k := j; k < len(stmt); {
		switch c := stmt[k]; {
		case c == ';':
			if skipSpace(stmt, k+1) < len(stmt) {
				return errStatements
			}
			return nil
		case c == '\'' || c == '"' || c == '`':
			k = skipQuoted(stmt, k, c)
		case c == '[':
			k = skipQuoted(stmt, k, ']')
		case strings.HasPrefix(stmt[k:], "--") || strings.HasPrefix(stmt[k:], "/*"):
			k = skipSpace(stmt, k)
		default:
			k++
		}
	}
	return nil
}

// skipSpace is the index of the first byte of s at or after i that is neither whitespace nor
// in a comment; an unterminated comment runs to the end.
func skipSpace(s string, i int) int {
	for i < len(s) {
		switch {
		case s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\f' || s[i] == '\v':
			i++
		case strings.HasPrefix(s[i:], "--"):
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				return len(s)
			}
			i += end + 1
		case strings.HasPrefix(s[i:], "/*"):
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return len(s)
			}
			i += 2 + end + 2
		default:
			return i
		}
	}
	return i
}

// skipQuoted is the index just past the quoted text that starts at s[i] and ends at the closing
// byte end; a doubled closing byte inside is part of the text. An unterminated one runs to the
// end.
func skipQuoted(s string, i int, end byte) int {
	for k := i + 1; k < len(s); k++ {
		if s[k] != end {
			continue
		}
		if end != ']' && k+1 < len(s) && s[k+1] == end {
			k++
			continue
		}
		return k + 1
	}
	return len(s)
}
