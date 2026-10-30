// tree-sitter-alder/src/scanner.c
//
// The external scanner: the Go lexer's context-sensitive rules, each ported from the function
// named in its comment (internal/syntax/lexer.go, parser_expr.go). Its one piece of state is
// whether the file is a scratch file (internal/scratch file.go), which changes where a newline
// separates.

#include "tree_sitter/parser.h"

#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

enum TokenType {
  NEWLINE,
  STRING,
  HTML_TEXT,
  RAW_TEXT,
  RAW_TAG_NAME,
  IMPLICIT_END_TAG,
  ERROR_SENTINEL,
  SCRATCH_START,
};

typedef struct {
  bool scratch;  // set by scan_scratch_start: the file is a scratch file
} Scanner;

void *tree_sitter_alder_external_scanner_create(void) { return calloc(1, sizeof(Scanner)); }
void tree_sitter_alder_external_scanner_destroy(void *payload) { free(payload); }
unsigned tree_sitter_alder_external_scanner_serialize(void *payload, char *buffer) {
  buffer[0] = ((Scanner *)payload)->scratch ? 1 : 0;
  return 1;
}
void tree_sitter_alder_external_scanner_deserialize(void *payload, const char *buffer, unsigned length) {
  ((Scanner *)payload)->scratch = length > 0 && buffer[0] == 1;
}

static void advance(TSLexer *lexer) { lexer->advance(lexer, false); }
static void skip(TSLexer *lexer) { lexer->advance(lexer, true); }
static bool is_blank(int32_t c) { return c == ' ' || c == '\t' || c == '\r'; }
static bool is_letter(int32_t c) { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'); }
static bool is_name_char(int32_t c) {
  return is_letter(c) || (c >= '0' && c <= '9') || c == '_' || c == '-';
}

// ---- strings: lexer.go scanString / scanInterp -------------------------------------------

static bool scan_interp(TSLexer *lexer);

// scan_string_body is scanString after the opening quote: a backslash skips the next byte, a
// quote ends the string, `{` opens an interpolation, and a newline or EOF means the string is
// unterminated, which is not a string token (the compiler reports it).
static bool scan_string_body(TSLexer *lexer) {
  for (;;) {
    if (lexer->eof(lexer) || lexer->lookahead == '\n') return false;
    int32_t c = lexer->lookahead;
    if (c == '\\') {
      advance(lexer);
      if (!lexer->eof(lexer) && lexer->lookahead != '\n') advance(lexer);
      continue;
    }
    if (c == '"') {
      advance(lexer);
      return true;
    }
    if (c == '{') {
      advance(lexer);
      if (!scan_interp(lexer)) return false;
      continue;
    }
    advance(lexer);
  }
}

// scan_interp is scanInterp after `{`: nested strings recurse, braces nest, and a newline
// fails the interpolation (and so the string). Backslash is not special here.
static bool scan_interp(TSLexer *lexer) {
  int depth = 1;
  for (;;) {
    if (lexer->eof(lexer) || lexer->lookahead == '\n') return false;
    int32_t c = lexer->lookahead;
    if (c == '"') {
      advance(lexer);
      if (!scan_string_body(lexer)) return false;
      continue;
    }
    if (c == '{') depth++;
    if (c == '}') {
      depth--;
      if (depth == 0) {
        advance(lexer);
        return true;
      }
    }
    advance(lexer);
  }
}

// ---- newlines: parser_expr.go atOpAfterNL / ltIsOperator ---------------------------------

// is_continuation reports whether the first character of a line (the lexer's lookahead) begins
// a binary operator that continues the previous line: |> || ?? && == != < <= > >= + - ++ * / %.
// Also `|` (union variants) and a lone `=` (a type's `=` on the next line), which can never
// begin a declaration. `<` continues only when whitespace follows it; `<-` never does.
static bool is_continuation(TSLexer *lexer) {
  switch (lexer->lookahead) {
    case '|':
    case '&':
    case '=':
    case '>':
    case '+':
    case '*':
    case '/':
    case '%':
      return true;
    case '!':
      advance(lexer);
      return lexer->lookahead == '=';
    case '?':
      advance(lexer);
      return lexer->lookahead == '?';
    case '<':
      advance(lexer);
      if (lexer->lookahead == '=') return true;
      return lexer->lookahead == ' ' || lexer->lookahead == '\t' || lexer->lookahead == '\r' ||
             lexer->lookahead == '\n';
    case '-':
      advance(lexer);
      return lexer->lookahead != '>';  // `--` comment lines were skipped by the caller
    default:
      return false;
  }
}

// is_watch_start reports whether a column-1 line (the lexer's lookahead) starts a scratch
// file's watch: `>` followed by a space, a tab, a newline or EOF (file.go ParseFile).
// `>=` and `>x` still continue the line before.
static bool is_watch_start(TSLexer *lexer) {
  if (lexer->lookahead != '>' || lexer->get_column(lexer) != 0) return false;
  advance(lexer);
  int32_t c = lexer->lookahead;
  return lexer->eof(lexer) || c == ' ' || c == '\t' || c == '\r' || c == '\n';
}

// scan_newline runs at '\n' where the grammar accepts a separator. The token is the newline
// plus any blank lines after it; it stops before a comment line so the comment stays a node.
// It then looks past blank and comment lines: if the next code line starts with an operator
// the newline is not a separator (no token; the newline is whitespace). During error recovery
// it only separates before a line that starts at column 1, which is a declaration boundary.
// In a scratch file a line that starts a watch separates too.
static bool scan_newline(TSLexer *lexer, bool recovering, bool scratch) {
  advance(lexer);
  lexer->mark_end(lexer);
  bool seen_comment = false;
  for (;;) {
    while (is_blank(lexer->lookahead)) advance(lexer);
    if (lexer->eof(lexer)) break;
    if (lexer->lookahead == '\n') {
      advance(lexer);
      if (!seen_comment) lexer->mark_end(lexer);
      continue;
    }
    if (lexer->lookahead == '-') {
      advance(lexer);
      if (lexer->lookahead == '-') {
        seen_comment = true;
        while (!lexer->eof(lexer) && lexer->lookahead != '\n') advance(lexer);
        continue;
      }
      // a line starting with `-` continues: subtraction, or `->` continuing a type
      // (parser.go parseType). This runs only where a separator is valid, and there the
      // grammar takes `->` only after a type, where Go continues too, so no narrower rule is
      // needed. A line-leading `->` in a match arm or after a lambda's parameters (E0001 in Go)
      // is still accepted, but not through here: inside an arm or a lambda head no separator is
      // valid, so the newline is plain whitespace (extras). Rejecting it would take newline
      // tokens inside every match and lambda for code the compiler already reports.
      return false;
    }
    if (recovering && lexer->get_column(lexer) != 0) return false;
    if (scratch && lexer->lookahead == '>') {
      if (is_watch_start(lexer)) break;
      return false;  // `>=` or `>x`: an operator continuing the line before
    }
    if (is_continuation(lexer)) return false;
    break;
  }
  lexer->result_symbol = NEWLINE;
  return true;
}

// ---- HTML: lexer.go ScanText / ScanRawText, parser_html.go addText -----------------------

// scan_html_text reads element content up to `{`, a `<` followed by a letter, `/` or `!`, or
// EOF. It stops before a newline whose next non-blank line starts with `--` (a comment inside
// markup, parser_html.go addText) so the comment is lexed as a comment, at any column: Go's
// ScanText has no column rule. It also stops before a newline whose next line starts in column 1
// with anything but `<`, `{`, `--` or whitespace: element content is indented in practice, and
// stopping there keeps an unclosed tag from turning the rest of the file into text while code is
// being typed (the drift test shows no valid file relies on column-1 text).
//
// Stopping there with no text read, the element was left open: it returns a zero-width
// implicit end tag, once per open element, so the open binding completes and the newline then
// separates the column-1 declaration. Error recovery cannot do this: with the elements left
// open, tree-sitter's error costs prefer skipping a name-led declaration's first tokens
// (`after :`, `after =`) into an ERROR and taking the rest as the binding's body over discarding
// the binding. The Go parser rejects this code (the compiler reports it); the tree accepts it.
static bool scan_html_text(TSLexer *lexer, const bool *valid) {
  bool has_text = false;
  for (;;) {
    if (lexer->eof(lexer)) break;
    int32_t c = lexer->lookahead;
    if (c == '{') break;
    if (c == '<') {
      lexer->mark_end(lexer);
      advance(lexer);
      int32_t d = lexer->lookahead;
      if (is_letter(d) || d == '/' || d == '!') {
        lexer->result_symbol = HTML_TEXT;
        return has_text;
      }
      has_text = true;
      continue;
    }
    if (c == '\n') {
      lexer->mark_end(lexer);
      advance(lexer);
      int32_t first = lexer->lookahead;
      bool column1 = !lexer->eof(lexer) && !is_blank(first) && first != '\n' && first != '<' && first != '{';
      while (is_blank(lexer->lookahead)) advance(lexer);
      if (lexer->lookahead == '-') {
        advance(lexer);
        if (lexer->lookahead == '-') {
          lexer->result_symbol = HTML_TEXT;
          return has_text;
        }
      }
      if (column1) {
        if (has_text) {
          lexer->result_symbol = HTML_TEXT;
          return true;
        }
        if (!valid[IMPLICIT_END_TAG]) return false;
        lexer->result_symbol = IMPLICIT_END_TAG;  // zero width: mark_end is before the newline
        return true;
      }
      has_text = true;
      continue;
    }
    advance(lexer);
    has_text = true;
  }
  lexer->mark_end(lexer);
  lexer->result_symbol = HTML_TEXT;
  return has_text;
}

// scan_raw_text reads a <script>/<style> body up to `</script` or `</style` (ASCII case-
// insensitive) followed by `>`, whitespace, `/` or EOF. Either closer ends either body, which
// accepts a little more than the compiler.
static bool scan_raw_text(TSLexer *lexer) {
  bool has_text = false;
  for (;;) {
    if (lexer->eof(lexer)) break;
    if (lexer->lookahead != '<') {
      advance(lexer);
      has_text = true;
      continue;
    }
    lexer->mark_end(lexer);
    advance(lexer);
    if (lexer->lookahead != '/') {
      has_text = true;
      continue;
    }
    advance(lexer);
    char name[8];
    int n = 0;
    while (n < 7 && is_letter(lexer->lookahead)) {
      int32_t c = lexer->lookahead;
      name[n++] = (char)(c >= 'A' && c <= 'Z' ? c + 32 : c);
      advance(lexer);
    }
    name[n] = 0;
    int32_t after = lexer->lookahead;
    bool closer = (strcmp(name, "script") == 0 || strcmp(name, "style") == 0) &&
                  (lexer->eof(lexer) || after == '>' || after == '/' || is_blank(after) || after == '\n');
    if (closer) {
      lexer->result_symbol = RAW_TEXT;
      return has_text;
    }
    has_text = true;
  }
  lexer->mark_end(lexer);
  lexer->result_symbol = RAW_TEXT;
  return has_text;
}

// scan_raw_tag_name recognises `script` or `style` (exactly, lowercase) right after `<`, the
// tags whose bodies are raw text (parser_html.go).
static bool scan_raw_tag_name(TSLexer *lexer) {
  char name[8];
  int n = 0;
  while (n < 7 && lexer->lookahead >= 'a' && lexer->lookahead <= 'z') {
    name[n++] = (char)lexer->lookahead;
    advance(lexer);
  }
  name[n] = 0;
  if (is_name_char(lexer->lookahead)) return false;
  if (strcmp(name, "script") != 0 && strcmp(name, "style") != 0) return false;
  lexer->mark_end(lexer);
  lexer->result_symbol = RAW_TAG_NAME;
  return true;
}

// ---- scratch files: internal/scratch file.go ParseFile ----------------------------------

// scan_scratch_start runs at the start of the file. It looks past blanks, newlines and `--`
// comments at the first word: a file whose first word is not `module` is a scratch file. The
// token is zero width, so everything it looked at is lexed again.
static bool scan_scratch_start(Scanner *s, TSLexer *lexer) {
  lexer->mark_end(lexer);
  for (;;) {
    while (is_blank(lexer->lookahead) || lexer->lookahead == '\n') advance(lexer);
    if (lexer->lookahead != '-') break;
    advance(lexer);
    if (lexer->lookahead != '-') break;
    while (!lexer->eof(lexer) && lexer->lookahead != '\n') advance(lexer);
  }
  static const char module[] = "module";
  int n = 0;
  while (module[n] && lexer->lookahead == module[n]) {
    advance(lexer);
    n++;
  }
  if (!module[n] && !is_name_char(lexer->lookahead)) return false;
  s->scratch = true;
  lexer->result_symbol = SCRATCH_START;
  return true;
}

bool tree_sitter_alder_external_scanner_scan(void *payload, TSLexer *lexer, const bool *valid) {
  Scanner *s = (Scanner *)payload;
  bool recovering = valid[ERROR_SENTINEL];

  // Only the file's first token may be the scratch start. Looking for it consumes lookahead, so
  // in a module file nothing else is scanned until `module`: the grammar's lexer takes the
  // comments before it, and a newline before it is whitespace.
  if (valid[SCRATCH_START] && !recovering) return scan_scratch_start(s, lexer);

  // Text tokens include their leading whitespace, so they are tried before skipping it, and
  // never during error recovery, where they would swallow code.
  if (!recovering) {
    if (valid[RAW_TEXT]) return scan_raw_text(lexer);
    if (valid[HTML_TEXT]) return scan_html_text(lexer, valid);
    if (valid[RAW_TAG_NAME] && lexer->lookahead >= 'a' && lexer->lookahead <= 'z') {
      return scan_raw_tag_name(lexer);
    }
  }

  while (is_blank(lexer->lookahead)) skip(lexer);

  if (valid[NEWLINE] && lexer->lookahead == '\n') return scan_newline(lexer, recovering, s->scratch);

  if (valid[STRING]) {
    while (is_blank(lexer->lookahead) || lexer->lookahead == '\n') skip(lexer);
    if (lexer->lookahead == '"') {
      advance(lexer);
      if (scan_string_body(lexer)) {
        lexer->result_symbol = STRING;
        return true;
      }
    }
  }
  return false;
}
