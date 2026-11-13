# Search plan: interface facts

Existing signatures the search plan (`2026-11-12-search.md`) builds on, collected 2026-11-12 at
8e3c55f0 by reading the code. Line numbers drift; names do not.
All paths relative to the repo root. "V" = verified by reading; "I" = inferred, not run.

## A. internal/types

types/types.go:11  `type Type interface{ isType() }`  (sealed: only types in this package implement it)
types.go:15  `type Con struct{ Name, Module string }`  builtin; Module "" for Prelude. Opaque no-param stdlib types are Con{Name,Module:"Text"} (Text.Builder). Prelude singletons: types.Int/Float/Bool/Text/Unit/Date/Time/Never/Error/Route/Response/File/Bytes (types.go:81). types.Err (poison, prints "_") at :99, `IsErr(t)` :102.
types.go:16  `type Var struct { ID int; Name string }`  unification variable (Name "" when made by Fresh/Instantiate).
types.go:20  `type Brand struct { Module, Name string; Base Type; HasCheck bool }`
types.go:25  `type Field struct { Name string; Type Type; HasDefault bool; Default syntax.Expr }`
types.go:38  `type Record struct { Module, Name string; Fields []Field; Anonymous bool; Params []string; Args []Type; Def *Record; Flex *Var }`  nominal when !Anonymous; instance has Args+Def.
types.go:51  `type Ctor struct { Name string; Arg Type }`
types.go:55  `type Union struct { Module, Name string; Args []Type; Ctors []Ctor; Def *Union; Params []string }`  Maybe/Result/Bool are Unions with Module "Prelude". `(u *Union) Constructors() []Ctor` :246, `WithArgs(args []Type) *Union` :254.
types.go:62  `type App struct { Con string; Args []Type }`  opaque stdlib generics with params: List a, Dict k v, Query a, Task a, Html (checker's Html is `App{Con:"Html",Args:[Route]}`, prints "Html"). NO module on App.
types.go:66  `type Fun struct { Params []Type; Result Type }`  n-ary, flattened (a -> b -> c is Params [a,b], Result c). A signature `A -> (B -> C)` parenthesised keeps a nested Fun param.
types.go:70  `type Param struct{ Name string }`  a signature's type variable (rigid, named). Instantiate turns Params into fresh Vars.

Printing (types.go):
  :104 `func String(t Type) string`  (modules qualified by full path)
  :107 `func StringIn(t Type, mod string) string`  (mod's own types unqualified)
  :114 `func StringQ(t Type, mod string, q func(module string) string) string`  (hover uses `types.StringQ(t, m.Name, a.prog.Qualify(m.Name))`)
  Unresolved Var: Var with Name "" prints "_" (types.go:~158); Var with a Name prints the Name; Param prints its Name; Err prints "_". Union in Prelude prints bare ("Maybe Int"); Html prints "Html".

types/unify.go:
  :6  `type Subst map[int]Type`
  :25 `type Unifier struct { Subst Subst; next int; tables map[string]*TableInfo }`
  NO constructor func. Checker builds one as `&types.Unifier{Subst: types.Subst{}}` (check/check.go:~132). A fresh one is fine for search only if candidate types contain no tables; prefer res.U.Clone().
  :48 `func (u *Unifier) Clone() *Unifier`  copies the whole Subst map (O(len Subst), the app's entire inference substitution) and the tables map (shallow); `next` carries on. Nothing else copied (types are shared, immutable). V.
  :62 `func (u *Unifier) Fresh() *Var`
  :83 `func (u *Unifier) Apply(t Type) Type`  deep substitute; normalises Id/Insert/InsertScoped over known tables.
  :190 `func (u *Unifier) Unify(a, b Type) error`  exact
  :193 `func (u *Unifier) Check(actual, expected Type) error`  like Unify but brand widens to base, Never fits anywhere (widen=true).
  :360 `func (u *Unifier) Instantiate(t Type) Type`  Param -> fresh Var, consistently per call. Instantiates Params nested in Union/Record/App/Fun.
  MUTATION: Unify/Check write `u.Subst[...] = ...` directly, even on failure partway (no rollback). So every candidate needs its own clone (hole completion does `u := cx.a.res.U.Clone(); u.Check(u.Instantiate(t), want)`, complete.go:~835). Clone cost is O(|Subst|) per candidate: for 2000+ stdlib candidates with a big app that is likely the dominant cost; a search engine should instead build ONE small fresh unifier per query (or per candidate) over query-and-candidate types only and avoid res.U unless the hole type has unresolved Vars bound in res.U (call res.U.Apply(hole type) first so the query type is Var-free, then use a tiny fresh Unifier). Registered tables matter only for Id/Insert normalisation. (I for the recommendation, V for the facts.)
  Note Check(actual, expected) argument order: candidate (instantiated) is `actual`, the wanted type is `expected`. For a rigid query type variable use *types.Param in the query type: Unify of Param with a Var binds the Var to the Param; Param vs Con mismatches (V from reading unify structure; not run).

types/stdlib.go:14 `func LoadStd() (map[string]*syntax.Module, error)`  parses every stdlib/*.alder via syntax.ParseSignatures("stdlib/<file>", src); keyed by module name ("Prelude","Text","List",...). Modules' .Lines hold source for DocOf. Stdlib dir: /Users/tavishpegram/projects/alder/stdlib (embedded, stdlib/embed.go). Stdlib has no parameter names; opaque `type X a` is a TypeDecl with no Union/Record/Brand.

## B. internal/check

check/result.go:9 `type Result struct` (fields relevant):
  Types  map[syntax.Expr]types.Type        // every expression's type (after Apply)  <- hole type is res.Types[*syntax.HoleExpr]
  Binds  map[any]types.Type
  Values map[string]types.Type             // "Module.name" -> signature  (includes stdlib modules: declareValues runs for allModules())
  Decls  map[string]types.Type             // "Module.Type" -> declared type (records, unions, brands, tables, forms; stdlib opaque types too)
  Tables map[string]*types.TableInfo; Forms map[string]*types.Record
  Ctors  map[string]*CtorInfo              // "Module.Ctor" -> {Union, Arg}   e.g. "Prelude.Just"
  Holes  int                               // count of W0002
  U      *types.Unifier
  Routes map[string][]*RouteInfo, Caps ..., TestOnly map[string]string ("Module.name" -> why test helper)
  `type CtorInfo struct { Union *types.Union; Arg types.Type }` (result.go ~:97)
  No list of declarations with modules beyond these maps (maps, unordered): declaration ORDER and docs must come from syntax modules (prog.Std[...].Decls, prog.Modules[...].Decls) - iterate those and look up Values/Ctors/Decls by key. Ctor key == c.key(mod, name) = mod+"."+name (check.go:~295 `func (c *Checker) key(mod, name string) string`; decls.go:293 registers ctors).
  Note: a route's members are values named "Index.load" (ValueDecl.Route != ""); skip/handle them. `vd.FullName()` gives "Index.load".
  Values has no entry for a ValueDecl with Sig == nil in an app module that failed (E0007).

Entry points (check/check.go):
  :124-ish `func Program(prog *names.Program, bag *diag.Bag) *Result`
  `func ProgramWith(prog *names.Program, bag *diag.Bag, o Options) *Result`
  `type Options struct { InferModule string }`
  Full pipeline (buildkit/buildkit.go:67 loadChecked): types.LoadStd -> names.LoadOverlay(dir,std,overlay,extra...) -> if !prog.ParseFailed { prog.Resolve(bag); prog.MarkFailed(bag); res = check.ProgramWith(prog,bag,o) } ; bag.Sort(). res nil only on parse failure. Holes do NOT stop it (W0002 warnings).

EMPTY PROGRAM (stdlib-only index) - V by reading, NOT run:
  names.Build(mods []*syntax.Module, std map[string]*syntax.Module) (*Program, *diag.Bag) (names/graph.go:165) accepts nil mods: Program{Modules:{}, Order:nil, Std:std}. check.Program then runs declareStd, declareTypeNames over allModules() (= every prog.Std module, map order, then prog.Order), defineTypes, declareValues for all of them, so Result.Values/Decls/Ctors include the stdlib. Every App-specific step is guarded: `if app := prog.Modules["App"]; app != nil` (check.go ~:152, ~:261); loops over prog.Order are empty. typeOf for the `Session` type checks `c.prog.Modules["App"] == nil && c.prog.Std[mc.m.Name] == nil` so stdlib mentioning Session is fine (decls.go:140).
  Suggested stdlib-only recipe: std, _ := types.LoadStd(); prog, _ := names.Build(nil, std); then mirror buildkit.loadChecked: bag := &diag.Bag{}; prog.Resolve(bag); prog.MarkFailed(bag); res := check.Program(prog, bag). Resolve (names/resolve.go:155) indexes the scopes of p.Std modules too, so do call it.
  NO existing test or command checks a zero-module program (grep for names.Build(nil / empty module lists: none; all names.Build tests pass modules; cmd `alder routes`/`check` refuse "no .alder files under <dir>/app"; scratch.Check also errors "no .alder files" when len(prog.Modules)==len(mods)). The builder must verify with a quick test that it does not panic/produce diagnostics. Cost: checking the whole stdlib signatures each time `alder search` runs outside an app (signature-only, bodies nil) - probably small but unmeasured.
  Alternative that avoids the checker for stdlib: none exported; typeOf is unexported (below).

Signature -> types.Type:
  check/decls.go:119 `func (mc *modCtx) typeOf(t syntax.Type, params map[string]bool) types.Type`  UNEXPORTED method on unexported modCtx; NOT reachable from outside package. It depends on t.Ref (syntax.Ref filled by names.Resolve: Kind RefType, Module, Name) for every *syntax.TypeName; unresolved (Ref.Kind != RefType) -> types.Err. Type vars: params[name] -> &types.Param{Name}, adds unknown lowercase to params; RecordType -> anonymous Record; FunType -> Fun; TypeName -> looks up c.types[c.key(Ref.Module, name)], checks arity (typeArity), applies args via applyArgs; Html -> App{Html,[Route]}.
  Consequence: a query parser output (syntax.Type with empty Refs) cannot be fed to typeOf. The search package must have its own syntax.Type -> types.Type converter: resolve TypeName by looking in res.Decls["Module.Name"] (key = c.key(module, name)); Prelude types are keyed "Prelude.X" (Int, Text, List, Maybe, Result, Bool...). Then build args itself: Union -> d.WithArgs(args), generic Record -> d.WithArgs(args), App -> &types.App{Con: d.Con, Args: args}, else d. (copy of check.applyArgs, decls.go:~245, unexported) plus Html special case, plus arity check. An alternative is exporting a thin helper in check (e.g. `func (r *Result) TypeFromSyntax(...)`); neither exists.
  Type-name resolution for real source: names.Resolve (names/resolve.go:155) sets TypeName.Ref per module scope (imports, qualifiers `prog.Qualifiers(module)`); needs the module context. Not usable for a query.
  Unresolved note: `_` lexes as token kind Underscore (lexer.go:206), NOT LIdent, so parseAType errors on `_` today ("expected a type, got ...").

## C. internal/syntax

syntax/ast.go:
  :9  `type Node struct{ Span diag.Span }`; `func (n *Node) GetSpan() diag.Span`
  :28 `type Ref struct { Kind RefKind; Module string; Name string }`  (RefType, RefValue, RefStd, RefCtor, ... ast.go:12-24)
  :36 `type Module struct { Node; Name string; File string; Imports []*Import; Decls []Decl; Lines []string; Comments []Comment }`
  :56 `type Import struct { Node; Module string; Alias string; AliasSpan diag.Span }`
  :61 `type Decl interface{ GetSpan() diag.Span }`
  :64 `type ValueDecl struct { Node; Route string; RouteSpan diag.Span; Name string; Sig Type; Params []*Ident; Body Expr }`  `(d *ValueDecl) FullName() string`; stdlib: Body nil; Sig set.
  :88 `type TypeDecl struct { Node; Name string; Params []string; Union []*Ctor; Record *RecordType; Brand *BrandType }`  all nil = opaque.
  :96 `type Ctor struct { Node; Name string; Arg Type }`  Arg nil = no payload (ONE payload type; `Just a`, `Rendered Route`).
  :101 `type BrandType struct{ Node; Base string; Check Expr }`
  :272 `type Type interface{ GetSpan() diag.Span }`; :274 `type TypeName struct { Node; Module string; Name string; Args []Type; Ref Ref }`  (`Text.Builder` parses as Module "Text", Name "Builder"; `A.B.C` -> Module "A.B"); :281 `type TypeVar struct { Node; Name string }`; :285 `type RecordType struct{ Node; Fields []*Field }`; :289 `type FunType struct { Node; Params []Type; Result Type }`
  :403 `type HoleExpr struct { Node; Name string }`
  Other decl kinds: RouteDecl, TableDecl, FormDecl, CapDecl, FakeDecl, TestDecl, ...
Position/span: diag/diag.go:9 `type Pos struct{ Line, Col int }`; :10 `type Span struct { File string; Start, End Pos }`. Line and Col are 1-BASED (lexer starts line:1,col:1; advance increments col per BYTE), End is exclusive. LSP positions are 0-based (lspRange converts).
Parser:
  syntax/parser.go:44 `func ParseFile(file string, src []byte) (*Module, []diag.Diagnostic)`
  :124 `func ParseSignatures(file string, src []byte) (*Module, []diag.Diagnostic)`
  :73 `func ParseExpr(file string, src []byte) (e Expr, diags []diag.Diagnostic, more string, at diag.Pos)`
  :35 `func newParser(file string, src []byte, sigMode bool, trace *[]Token) *Parser`  (calls p.advance() to prime the first token; `p.lines`, `p.lx` set)
  :1105 `func (p *Parser) parseType() Type`  grammar: atype {atype} ["->" type]; flattens arrows; calls p.in("a type"), spanFrom.
  :1137 `func (p *Parser) parseAType() Type`  UIdent(.UIdent)* -> TypeName; LIdent -> TypeVar; `{`->record; `(`..`)` ; `()` -> TypeName Unit; else errorAt "expected a type, got ...".
  A new exported ParseType would be: `p := newParser("query", []byte(src), false, nil); p.skipNL(); t := p.parseType(); p.skipNL(); more/at as ParseExpr does if !p.at(EOF); return t, p.finish()`  (mirror ParseExpr at parser.go:73-82). For `_`: add a Parser flag (like p.session, p.pageParams, parser.go:20-33) so parseAType accepts token Underscore and returns e.g. &TypeVar{Name:"_"}; source files unaffected while flag false. `p.at(Kind)`, `p.advance()`, `p.errorAt(tok, msg)` exist. Note: parseType's application loop (`for p.at(UIdent)||p.at(LIdent)...`) would also need to treat Underscore as an arg start only under the flag.
  Hole token: lexer.go:249 `?name` -> Hole token; HoleExpr built at parser_expr.go:274. 
Doc & walk (syntax/walk.go):
  :94 `func DocOf(lines []string, line int) string`  line = 1-based line of the declaration (d.GetSpan().Start.Line); returns the `--` block directly above, "" if none (NOT blank-line tolerant: stops at first non-`--` line).
  :72 `func NodeAt(m *Module, pos diag.Pos) []Spanned`  path from m to innermost node (index 0 is m); `func Contains(sp diag.Span, pos diag.Pos) bool` :67 half-open; `func Children(n Spanned) []Spanned` :19 (reflection); `type Spanned interface{ GetSpan() diag.Span }` walk.go:12.
  Decl span for a ValueDecl in a stdlib file starts at the signature line; for a Ctor, Ctor.Span.Start.Line is the ctor's own line (a ctor in a multi-line union written `| Rendered Route` on its own line gets that line, so DocOf above it works only for those).

## D. Program loading

buildkit/buildkit.go:
  :46 `func LoadChecked(dir string, extra []*syntax.Module) (prog *names.Program, res *check.Result, bag *diag.Bag, ioErrs []error)`
  :57 `func LoadCheckedOverlay(dir string, overlay map[string][]byte) (prog *names.Program, res *check.Result, bag *diag.Bag, ioErrs []error)`  overlay keys = slash paths relative to dir ("app/Posts/Index.alder"); unsaved buffers.
  :63 `LoadCheckedOverlayWith(dir, overlay, o check.Options)`;  :51 `LoadCheckedWith(dir, extra, o)`
  :86 `func Sources(prog *names.Program) map[string][]string`  module file -> lines (for diag.RenderText); includes prog.Refused modules.
  buildkit/routes.go:15 `type Route struct { Kind, Method, Path, Module, Name, File string; Line, Col int; EndLine, EndCol int }`; :24 `func Routes(prog *names.Program, res *check.Result) []Route` (calls ir.Lower: panics/needs a hole-free clean check -> lsp/routes.go recovers).
  Also `buildkit.AddSchemaDrift(dir, prog, res, bag)` (adds E0019); `check`/`routes`/lsp call it after loading. res may be nil.
names/graph.go:17 `type Program struct { Modules map[string]*syntax.Module; Order []string; Std map[string]*syntax.Module; Failed map[string]bool; Refused []*syntax.Module; ParseFailed bool; Unparsed []string; ElementsManifest []byte; ... }`  module file path = Module.File (relative to project root, slash, e.g. "app/Posts/Show.alder"; stdlib "stdlib/Text.alder"). `prog.Qualify(module) func(string) string`, `prog.Qualifiers(module) map[string]string`, `prog.QualifierFor(module, path)` (qualify.go:118-137) give the import qualifier for printing.
  `names.CanonicalQualifiers(self string, paths []string, taken func(string) bool) (quals map[string]string, bad []string)`, `names.Taken(m *syntax.Module, isStd func(string) bool) func(string) bool`, `names.NeedsAlias(path, q string) bool`.
cmd/alder/build.go:234 `func checkedApp(dir string) (*names.Program, *check.Result, *diag.Bag, []error)` = buildkit.LoadChecked(dir, nil) then, if no ioErrs, buildkit.AddSchemaDrift.
  Directory with no app/: names.LoadOverlay's WalkDir callback gets err for the missing root and appends it to ioErrs -> checkedApp returns ioErrs -> `reportLoadErrs(cmd, ioErrs, stderr)` prints "<cmd>: <err>" -> exit 2 (TestRoutes_Errors asserts code 2 for t.TempDir()). A dir with app/ but no .alder files: prog.Modules empty -> routes prints "no .alder files under DIR/app", exit 2.
  "Inside an app" helpers (cmd/alder/eval.go:347,361): `func dirHasApp(dir string) bool` (dir/app contains an .alder file) and `func findAppRoot(start string) string` (nearest ancestor with an app, "" if none). `alder routes`/`check` do NOT walk up; they take [dir] default ".". `alder eval` uses dirHasApp/findAppRoot. For `alder search` use findAppRoot(cwd) (or dirHasApp(".")) to choose app vs stdlib-only.
  `alder fmt` (fmt.go:254 fmtAppRoot, :185) never builds the app: it parses/formats files and only uses findAppRoot to judge scratch files, falling back to cwd; it loads std itself via types.LoadStd. `alder explain` (main.go:~150 explainTo) touches nothing: a diag.Explain map lookup. Both work with no app.

## E. CLI (cmd/alder)

main.go:69 (inside `switch os.Args[1]` in main(); before it `wantsHelpJSON` and `helpTo(os.Args[1:], os.Stdout)` handle --help):
```
	case "routes":
		os.Exit(runRoutesTo(os.Args[2:], os.Stdout, os.Stderr))
```
  (Commands with stdin: `case "fill": os.Exit(runFillTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))`). Default branch: usage(); exit 2.
routes.go:15,28: `const routesUsage = "usage: alder routes [dir] [--json]"`; `func runRoutesTo(args []string, stdout, stderr io.Writer) (code int)` with a recover -> "alder routes: internal error: %v" code 2; flag loop switch; `checkedApp`; `reportLoadErrs("alder routes", ioErrs, stderr)`; table via text/tabwriter with header bold via style.For(stdout).Bold; JSON via json.Marshal + fmt.Fprintln. Style helpers: internal/style (style.For(w), .Bold, .BoldRed; errTag(w, tag) in main.go).
help.go:11-42 `var commandUsages = map[string]string{ ... "routes":   routesUsage, ...}`  (gofmt-aligned map; keys check, fmt, build, run, tui, routes, dump, ...). Add `"search": searchUsage,`.
helpjson.go:14 `type flagSpec struct { Name string; Value string; Multi bool; Summary string }` (json tags name,value,multi,summary); :24 `type cmdSpec struct { Name, Summary, Usage string; Args []string; Required []bool; Flags []flagSpec; Subcommands []cmdSpec }`.
helpjson.go:121 in `var commandTree = []cmdSpec{`:
```
	{Name: "routes", Summary: "list the app's routes, with their methods and paths", Usage: commandUsages["routes"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--json", Summary: "a JSON array of routes with file, line and col"}}},
```
  Value-taking flag example (eval): `{Name: "--in", Value: "Name", Summary: "..."}`.
Registration tests that fail if a command is missing/inconsistent (cmd/alder/helpjson_test.go:54 TestHelpJSON_MatchesUsage): every commandTree entry must be in commandUsages with identical Usage, every commandUsages key must have a commandTree entry, every node needs a Summary and every flag a Summary, and `usageFlags(c.Usage)` (flags parsed out of the usage string) must equal nodeFlags(c) (flags in the tree), so a `--limit N` in the usage text needs `{Name:"--limit", Value:"N"}`. ALSO cmd/alder/help_style_test.go:55 TestHelp_BookCommandsBlock requires book/11-tooling.md to contain the "Commands" fenced block equal to usageText(style.Plain) (rows = commandTree order: name, Summary, column-aligned) - adding a command REQUIRES updating that block in book/11-tooling.md (search for the fenced block that begins "usage: alder <command> [arguments]"). Column width is set by the longest name (so adding a name longer than "rollback"/"scratch" re-aligns the whole block; "search" is 6 chars, fine).
  Possibly also cmd/alder/style_colour_test.go:27 (list of commands with styled output) - optional.
Tests of CLI commands: cmd/alder/routes_test.go: helper
```
func runRoutes(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runRoutesTo(args, &out, &errb)
	return code, out.String(), errb.String()
}
```
  runs against fixture apps by relative path: examples/blog (`"../../examples/blog"`), testdata/corpus/<name> (e.g. "../../testdata/corpus/routes-ok", "../../testdata/corpus/E0002-own-module" for a type error), or t.TempDir() for no-app. Tests call t.Parallel(). Tests with typed holes: look in testdata/corpus for a hole fixture (not checked by me) or write files into t.TempDir()/app/App.alder.

## F. internal/lsp

server.go:37 `type handler func(s *server, params json.RawMessage) (any, error)`
server.go:~47-65 registries (a package-level `var ( notifications; fastMethods; slowMethods )`):
```
	fastMethods = map[string]handler{
		...
		"textDocument/codeAction":           codeActionReq,
		...
		"alder/generators":                  generatorsReq,
		"alder/routes":                      routesReq,
	}
	slowMethods = map[string]handler{
		"alder/preview":            previewReq,
		"alder/plan":               planReq,
		"workspace/executeCommand": executeCommandReq,
	}
```
  Unknown method -> -32601. Fast methods run in order on one worker; handler panics are recovered by s.run into an internal error.
server.go server struct fields used: `enc posEncoding; root, rootReal string; docs docs; cur, good atomic.Pointer[analysis]; applyEdit, workDoneProgress, showMessageRequest bool; inlayRefresh bool`.
analysis.go:17 `type analysis struct { prog *names.Program; res *check.Result; diags []diag.Diagnostic; versions map[string]int; lines map[string][]string }`  immutable snapshot.
analysis.go:~131 `func (s *server) current() *analysis` (latest; res nil after parse failure); `func (s *server) lastGood() *analysis` (latest with res != nil; INCLUDES builds with type errors and holes; nil before first build).
analysis.go `freshLines(a *analysis, rel string, now []string, line int, inclusive bool) bool`; `(s *server) fresh(a, rel, line, inclusive)`.
docs.go:91 `func (s *server) rel(uri string) (string, bool)` (URI -> root-relative slash path; false outside root); docs.go:133 `func (s *server) uriOf(rel string) string`; docs.go:141 `func (s *server) text(rel string) string` (open doc text else disk).
diagnostics.go:76 `func lspRange(line, col, endLine, endCol int) Range` (1-based compiler span -> 0-based LSP range, byte columns).
encoding.go: `func (e posEncoding) wireRange(lines []string, r Range) Range` (bytes -> wire units; lines = the lines the columns refer to); `wireEdit(lines, TextEdit) TextEdit`; `toWire(line string, col int) int`; INVERSE: `func (e posEncoding) fromWire(line string, ch int) int` (wire char -> byte col), `func (s *server) byteAt(rel string, p Position) Position` (:~92, reads current text) and `byteAtLines(lines []string, p Position) Position`. Compiler position = diag.Pos{Line: p.Line+1, Col: byteCol+1}.
protocol.go: `type Position struct { Line int; Character int }` (0-based), `type Range struct{ Start, End Position }`, `type Location struct{ URI string; Range Range }`, `type textDocumentPositionParams struct { TextDocument struct{ URI string }; Position Position }`, `type TextEdit struct { Range Range; NewText string }` (json range,newText).
actions.go:27-: 
```
type codeAction struct { Title string; Kind string; IsPreferred bool `json:"isPreferred,omitempty"`; Diagnostics []Diagnostic; Edit *workspaceEdit `json:"edit,omitempty"`; Data *generatorHere; Command *command }
type workspaceEdit struct { DocumentChanges []textDocumentEdit `json:"documentChanges"` }
type textDocumentEdit struct { TextDocument versionedDocument `json:"textDocument"`; Edits []TextEdit `json:"edits"` }
type versionedDocument struct { URI string `json:"uri"`; Version *int `json:"version"` }   // Version nil when file not open
type command struct { Title string; Command string; Arguments []any }
```
  Version for an open doc: `a.versions[rel]`; in the edit, edits are given in wire units (convert with s.enc.wireEdit against the CURRENT text lines).
complete.go:77 `type completion struct { a *analysis; enc posEncoding; m *syntax.Module; lines []string; line int; col int; prefix string; pos diag.Pos; fresh bool; same bool; locals []binder; hasLocals bool }`
  complete.go:114 `func (s *server) completionAt(rel string, p Position) *completion` builds it (reads text once, converts wire col to byte col, a = s.lastGood(), m = module of rel, fresh/same from freshLines). Reusable to get the hole under a cursor.
holeAt (complete.go:789) in full:
```
// holeAt is the build's hole the cursor is in or just after, when the cursor's line is
// unchanged since the build; nil otherwise.
func (cx *completion) holeAt() *syntax.HoleExpr {
	if !cx.same {
		return nil
	}
	var found *syntax.HoleExpr
	var walk func(n syntax.Spanned)
	walk = func(n syntax.Spanned) {
		for _, c := range syntax.Children(n) {
			if found != nil {
				return
			}
			if !within(c.GetSpan(), cx.pos) {
				continue
			}
			if h, ok := c.(*syntax.HoleExpr); ok {
				found = h
				return
			}
			walk(c)
		}
	}
	walk(cx.m)
	return found
}
```
`func within(sp diag.Span, pos diag.Pos) bool` (inclusive of the end).
fits (complete.go:823) in full:
```
// fits reports whether the name (a local in scope, else a value of the module) has a type that
// fits want. It unifies on a clone of the build's unifier, which the build keeps unchanged.
func (cx *completion) fits(name string, want types.Type) bool {
	var t types.Type
	if b := innermost(cx.scope(), name); b != nil {
		t = cx.a.res.Binds[b.Key]
	} else if v, ok := cx.a.res.Values[cx.m.Name+"."+name]; ok {
		t = v
	} else {
		t = cx.ctorType(cx.m.Name + "." + name)
	}
	if t == nil {
		return false
	}
	u := cx.a.res.U.Clone()
	return u.Check(u.Instantiate(t), want) == nil
}
```
hole() (complete.go:758): `g := reHole.FindStringSubmatch(cx.prefix); h := cx.holeAt(); ...; if h != nil { want = cx.a.res.Types[h] }` and requires cx.fresh. The hole's type = `cx.a.res.Types[h]` (already Applied; may still hold Vars bound in res.U -> use res.U.Apply). Hole type string for the editor: `types.StringQ(t, m.Name, a.prog.Qualify(m.Name))` (as hover does, hover.go:66) - prints unresolved Vars as "_", which is the spec's rule. W0002's message text uses mc.ts (same thing, module's qualifiers).
Import line: complete.go:190 `func (cx *completion) importText(path string) string` (METHOD on *completion; needs cx.a.prog and cx.m): returns path, or "path as Q" via names.CanonicalQualifiers(cx.m.Name, paths, names.Taken(cx.m, isStd)). It only serves `import ` completion (items inserted at the `import ` prefix). Completion does NOT add imports as an additional edit anywhere: there is no AdditionalTextEdits in internal/lsp (grep: none). `completionItem` (complete.go:35) = {Label, Kind, Detail, SortText, TextEdit TextEdit, Documentation *markupContent}. The search pick edit has to compute the import insertion itself (position after the last import or after `module X` line from m.Imports[i].Span / m.Span; qualifier from a.prog.QualifierFor(module, path) or names.CanonicalQualifiers). Qualifier used when writing a name from module M in file F: prog.Qualify(F.Name)(M) (names/qualify.go:128) - for a module F does not import yet use importText's CanonicalQualifiers logic.
Hole code action (actions.go, inside codeActionReq, ~:137-146):
```
	a := s.lastGood()
	if !freshLines(a, rel, now, start.Line+1, true) {
		return out, nil // the line or one above it has changed since the last good build
	}
	pos := diag.Pos{Line: start.Line + 1, Col: start.Character + 1}
	ctx := hereCtx(a, rel, pos)
	if ctx.Hole != "" && s.applyEdit { // the edit comes back as workspace/applyEdit: nothing else carries it
		if act, ok := s.fillAction(a, rel, ctx, pos); ok {
			out = append(out, act)
		}
	}
```
fill.go:29-49 fillAction (quote):
```
func (s *server) fillAction(a *analysis, rel string, ctx generate.HereCtx, pos diag.Pos) (codeAction, bool) {
	for _, h := range generate.HoleIndex(a.prog, a.diags) {
		if h.File != rel || h.Name != ctx.Hole || !within(h.Span, pos) {
			continue
		}
		var ds []Diagnostic
		for _, d := range a.diags {
			if d.Code == "W0002" && d.Span == h.Span {
				ds = append(ds, s.enc.wireDiagnostic(a.lines[rel], lspDiagnostic(d)))
			}
		}
		arg := fillArgs{URI: s.uriOf(rel), Address: h.Address(), Hole: h.Name, Type: h.Type}
		if v, open := a.versions[rel]; open {
			arg.Version = &v
		}
		title := fmt.Sprintf("Fill ?%s with AI (%s)", h.Name, h.Type)
		return codeAction{Title: title, Kind: "quickfix", Diagnostics: ds,
			Command: &command{Title: title, Command: "alder.fill", Arguments: []any{arg}}}, true
	}
	return codeAction{}, false
}
```
  generate.HoleIndex(prog, diags) []generate.Hole (generate/holes.go:55): Hole{Module, Name, Type (the printed type string parsed out of W0002's message by regexp `hole \?(\w+) has type (.*)$`), File, Line, Col, Span, Decl, Arm}. hereCtx (actions.go:372) -> generate.HereCtx{Module, IsRoute, Decl, DeclKind, Hole}. Gate gotcha: the Fill action is gated on `s.applyEdit` (initializeParams capabilities.workspace.applyEdit, protocol.go:~75, read in server.go initialize: `s.applyEdit = p.Capabilities.Workspace.ApplyEdit`). The Fill action is kind "quickfix"; code action kinds advertised: ["quickfix","refactor"].
  The code action for search needs a NEW client-capability gate: initializeParams (protocol.go:76) only decodes workspace.applyEdit, workspace.inlayHint.refreshSupport, window.workDoneProgress, window.showMessage, general.positionEncodings. It does NOT decode `capabilities.experimental` or `initializationOptions`. Add a field to that struct (e.g. `Experimental struct{ AlderSearch bool ... } \`json:"experimental"\``, or top-level `InitializationOptions`), a bool on `server` set in initialize() (server.go ~:322-332, "set by initialize, before any worker reads them").
Server capabilities declared in protocol.go:~108 `func capabilities(enc posEncoding) map[string]any` (keys: positionEncoding, textDocumentSync, hoverProvider, documentFormattingProvider, inlayHintProvider, colorProvider, definitionProvider, referencesProvider, completionProvider{triggerCharacters . < ?}, codeActionProvider{codeActionKinds quickfix, refactor}, executeCommandProvider{commands: ["alder.explain","alder.fill"]}, semanticTokensProvider, foldingRangeProvider, selectionRangeProvider, experimental{alderScratch:true}). Add `"workspaceSymbolProvider": true` there and a new command name to executeCommandProvider if a server-run command is added (executeCommandReq actions.go:421 switch handles alder.fill, alder.explain; unknown -> codeInvalidParams). Client-run commands (like alder.generate) need not be listed.
alder/routes handler (lsp/routes.go), the model for alder/search:
```
func routesReq(s *server, _ json.RawMessage) (out any, err error) {
	items := []routeItem{}
	defer func() { if r := recover(); r != nil { out, err = items, nil } }()
	a := s.lastGood()
	if a == nil || a.res == nil || a.res.Holes > 0 { return items, nil }
	for _, r := range buildkit.Routes(a.prog, a.res) {
		items = append(items, routeItem{ ..., URI: s.uriOf(r.File),
			Range: s.enc.wireRange(a.lines[r.File], lspRange(r.Line, r.Col, r.EndLine, r.EndCol)) })
	}
	return items, nil
}
```
  Param decode errors: `return nil, &rpcError{codeInvalidParams, err.Error()}`. rpcError{code, message}.
hover.go:230 `func declDoc(a *analysis, ref syntax.Ref) string`  = findDecl(a, ref) (scope.go:448; modulePath(a, ref.Module) looks in a.prog.Modules then a.prog.Std) then `syntax.DocOf(mod.Lines, d.GetSpan().Start.Line)`. Needs: an analysis and a syntax.Ref{Kind: RefValue|RefStd|RefType|RefCtor, Module, Name}. For stdlib values use Kind RefStd (or RefValue; findDecl accepts RefValue/RefStd/RefComponent for ValueDecl, matching d.FullName()==ref.Name); type -> RefType; ctor -> RefCtor (returns the *syntax.Ctor; its span line). Simplest for search: walk modulePath(...).Decls yourself and call syntax.DocOf(mod.Lines, d.GetSpan().Start.Line).
Tests (internal/lsp): TestRoutes_List (routes_test.go:13):
```
	root := blogCopy(t)            // copyExample(t,"blog"): copies examples/blog to t.TempDir()
	c := start(t, root)            // client_test.go:49: server over io.Pipe, initialize+initialized (startWith / startOffering / startCaps(t, root, o Options, caps map[string]any) :67 for custom client capabilities)
	openClean(t, c, "app/Posts/Show.alder")   // hover_test.go:24: c.open + wait for diagnostics, returns text
	var out []routeItem
	if e := c.call("alder/routes", map[string]any{}, &out); e != nil { t.Fatalf(...) }
```
  client helpers: c.call(method, params, out) *rpcError, c.open(rel,text), c.change(rel,text), c.at(rel, needle, off) Position, c.uri(rel), c.disk(rel), c.diags(rel), c.save(rel). Custom client caps example: fill_test.go:72 fillClient + `fullCaps`; routesCopy (routes_test.go:~38) uses testdata/corpus/routes-ok. Hole fixtures: fill_test.go fillRoot(t)/fillRel (fill_test.go, app with `?out`); TestCodeAction_FillHole fill_test.go:202 and `actionsAt(t, c, rel, pos)` helper.

## G. Docs and conventions
Book (book/*.md; chapters 01-16, A-C): CLI + editor docs are in book/11-tooling.md:
  - "### `alder routes`" at line ~527 (under the CLI commands section; heading text is exactly "### `alder routes`"; code fence "```text" with a `$ alder routes examples/blog` transcript).
  - "## Editor support" line 2890 with "### What you get", "### Routes in the editor" (~3035; documents alder/routes request, params {}, uri/range), "### Generators in the editor", "### Scratch files in the editor", "### Setting up Neovim".
  - "## Holes" line 3250 (typed holes: W0002, `alder fill`, hover/completion/Fill-with-AI sentence); also "#### W0002 — typed hole (`?name`) with its expected type" at ~2838; "## Asking questions of the program" ~3270 lists `alder routes`, `alder eval`, `alder dump`, `alder explain` (good place to add `alder search`). Chapter 15 has "## Holes, and filling them: `alder fill`" (:691); chapter 13 ("Fitting a hole" :7246, "Filling a hole with a model" :7510, LSP transcripts section 10). Chapter 9 (09-stdlib.md) is the stdlib reference.
  - docs/markdown-style.md: wrap prose ~80 cols, never rename/remove headings (add new ###/#### instead), name headings after the thing.
  - The Commands block in 11-tooling.md must equal usageText(style.Plain) (TestHelp_BookCommandsBlock, see E).
Book snippet tests: book/book_test.go (package book, builds the real alder binary): TestBook (:838) checks every ```alder block (Module/Decls/Signatures `alder check`ed in a copy of examples/todos; Expr blocks parsed, or evaluated with `eval=`/`error=`); TestBook_Commands (:1215) runs ```text command=<state>``` blocks (see book 11-tooling.md:1957 "#### ```text command=<state>```" for the fence syntax and :1472 example `text command=guestbook`). Run: `go test ./book -run 'TestBook' -count=1` (builds binary; slow) - e.g. `go test ./book -run 'TestBook_Commands|TestBook$'`. A plain "```text" transcript (like alder routes) with no command= is not executed.
CHANGELOG.md top: "# Changelog" intro, then `## v0.2.0 — 2026-11-11` (em dash, ISO date), `### Breaking`, `### Added` (bullets of "- `alder lsp`: ...", wrapped ~96 cols, issue refs "(#172)"), `### Fixed`. There is no "Unreleased" section yet; a new release section above v0.2.0 would be needed. scripts/release-notes.sh extracts the section whose heading starts "## vX.Y.Z" up to the next "## ", so an "## Unreleased" heading is harmless but must be renamed to "## vX.Y.Z — date" at release.
Tests: no Makefile. `scripts/test.sh [go test flags]` runs everything in parallel (`scripts/test.sh -short` = "the quick local loop"; ALDER_TEST_SHARDS=4 default; cmd/alder in shards via .github/shard.sh, rest via `go test $(go list ./... | grep -v '^alder/cmd/alder$') -count=1 -timeout 30m`, REPL tests last). Scoped runs: `go test ./internal/search/... ./internal/lsp/... -count=1` and `go test ./cmd/alder -run 'TestSearch|TestHelp' -count=1` (cmd/alder tests build things and are slow). Mutation gates only with ALDER_MUTATION_GATES=1; formatter timing tests ALDER_SPEED_TESTS=1.
Lint before commit (CI `lint` job, .github/workflows/ci.yml:152-176): `gofmt -l $(git ls-files '*.go')` must print nothing; `go vet ./...`; plus Python tests (not relevant). No golangci-lint, no CLAUDE.md/CONTRIBUTING in the repo root (checked).
