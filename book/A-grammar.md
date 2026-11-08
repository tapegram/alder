# A. Grammar

EBNF. `{x}` is zero or more, `[x]` is optional, `|` is alternation.
Newlines are significant only as statement separators inside blocks.
Two further line breaks are allowed: between the attributes of an HTML open
tag (and before its `>` or `/>`), and in a type, where a line may begin with
`->` and the type may begin on the line after the `:` of a signature.

```ebnf
module      = "module" ModName NL {import NL} {decl NL} ;
import      = "import" ModName ["as" uname] ;   (* the canonical qualifier, chapter 3: E0204 otherwise *)

decl        = valueDecl | typeDecl | routeDecl | formDecl
            | tableDecl | capDecl | fakeDecl | sessionDecl | testDecl | loginDecl ;

valueDecl   = declName ":" type NL declName {lname} "=" expr ;
declName    = lname | uname "." lname ;         (* Route.name: a member of route Route, chapter 6;
                                                   the signature and the definition write the same name *)
typeDecl    = "type" uname {lname} "=" ( unionBody | recordType | brandType ) ;
unionBody   = ["|"] ctor {"|" ctor} ;
ctor        = uname [atype] ;
brandType   = "brand" uname ["where" expr] ;

routeDecl   = "route" uname "=" method string recordType ["form" uname] [guard] ;
method      = "GET" | "POST" | "PUT" | "PATCH" | "DELETE" ;
                                    (* "form F": a write route only, E0043;
                                       the string is the path, "{name}" segments;
                                       the record is the route's parameters: a field may
                                       have a default and "as" (a query key) *)
guard       = "requires" lname {"," lname} ["," "role" uname {"|" uname}] ["," "silo" lname] ;
                                    (* each lname a session field; the role clause: after
                                       "requires user" only, E0027 *)
formDecl    = "form" uname "=" "{" {fieldDef ","} [fieldDef] "}" ;
tableDecl   = "table" uname [renamed] "{" {fieldDef {attr} ","} [fieldDef {attr}] "}" ;
fieldDef    = lname ":" type ["=" expr] ["as" string] ;
                                    (* in a table: the default is a literal, E0022;
                                       "as": a route's query key, route parameters only;
                                       it may start the next line; in the default, "as"
                                       is a name inside ( ), [ ], a record's, an update's
                                       or a block's { }; elsewhere it ends the default *)
attr        = "@id" | "@ref" uname | "@created" | "@updated" | "@unique"
            | "@cascade" | "@restrict" | "@setNull"    (* a delete rule: after "@ref T" only *)
            | "@owner"                                 (* one per table, on a "@ref" of the user: E0024 *)
            | renamed ;
renamed     = "@renamed" string ;   (* the old name; a plain string, no "{…}" *)
capDecl     = "capability" uname "{" {lname ":" type ","} [lname ":" type] "}" ;
fakeDecl    = "fake" uname "{" {tableDecl | lname {lname} "=" expr} "}" ;   (* beside its capability, E0020 *)
sessionDecl = "session" recordType ;
loginDecl   = "login" "=" ModName ;   (* a route: Auth.Login.Login, the module and then the route *)
testDecl    = "test" string "=" expr ;

type        = atype {atype} ["->" type] ;
atype       = uname | ModName "." uname |   (* Index.Params, Row.Session: a route's types *)
             lname | recordType | "(" type ")" ;
recordType  = "{" {fieldDef ","} [fieldDef] "}" ;

expr        = pipeExpr ;
pipeExpr    = orExpr {"|>" orExpr} ;
orExpr      = defExpr {"||" defExpr} ;              (* see precedence table in ch. 3 *)
defExpr     = andExpr ["??" andExpr] ;
andExpr     = cmpExpr {"&&" cmpExpr} ;
cmpExpr     = addExpr [("==" | "!=" | "<" | "<=" | ">" | ">=") addExpr] ;
addExpr     = mulExpr {("+" | "-" | "++") mulExpr} ;
mulExpr     = appExpr {("*" | "/" | "%") appExpr} ;
appExpr     = primary {primary} ;
primary     = literal | lname | qname | uname | qname
            | "(" expr ")" | list | record | recordUpdate
            | lambda | ifExpr | matchExpr | block | html
            | primary "." lname ;

qname       = ModName "." (lname | uname) ;
literal     = int | float | string | "True" | "False" | "()" ;
list        = "[" {expr ","} [expr] "]" ;
record      = "{" {lname ":" expr ","} [lname ":" expr] "}" ;
recordUpdate= "{" expr "|" lname ":" expr {"," lname ":" expr} "}" ;
lambda      = "\" lparam {lparam} "->" expr ;
lparam      = lname | "(" lname ":" type ")" ;
ifExpr      = "if" expr "then" expr "else" expr ;
matchExpr   = "match" expr "{" {arm ","} [arm] "}" ;
arm         = pattern "->" expr ;
pattern     = "_" | literal | lname | uname [pattern1] | recordPat | listPat | "(" pattern ")" ;
pattern1    = "_" | literal | lname | uname | recordPat | listPat | "(" pattern ")" ;
recordPat   = "{" {lname ":" pattern ","} [lname ":" pattern] "}" ;
listPat     = "[" [pattern {"," pattern}] ["," ".." lname] "]" | "[" ".." lname "]" ;
block       = "{" {stmt NL} expr "}" ;
stmt        = "let" lname [":" type] ("=" | "<-") expr ;

html        = element | rawElement | selfClosing ;
element     = "<" tag {hattr} ">" {hchild} "</" tag ">" ;
rawElement  = "<" ("script" | "style") {hattr} ">" rawtext "</" ("script" | "style") ">" ;
selfClosing = "<" tag {hattr} "/>" ;
tag         = lname | uname | ModName "." uname ;
hattr       = hname "=" (string | "{" expr "}") | hname ;
hchild      = htext | html | "{" hexpr "}" ;
hexpr       = expr
            | "for" lname "in" expr "->" html
            | "if" expr "then" html "else" html
            | "match" expr "{" {pattern "->" html ","} "}" ;

string      = '"' {char | "{" expr "}"} '"' ;
ModName     = uname {"." uname} ;
```

## Routes and qualified names

A route's members are `Route.name` declarations (`Index.load`, `Index.view`,
`Toggle.update`); what a route module may declare, and the signatures, are in
chapter 6. A dotted name is read as the longest module path the program knows,
then a route of that module, then a member: `Todos.Index.Index {}` is the module
`Todos.Index`, its route `Index`, and the route's parameters.

## The old form

`page "…" { … }` and `msg M { … }` are the old form of a route and its writes. The
parser still accepts them, beside `route` (a module has one form or the other,
E0043), until they are removed; `alder fix --routes` rewrites them. They are not
part of the grammar above, and `page` and `msg` stay reserved words meanwhile.

## Lexical rules

- `lname` is `[a-z_][A-Za-z0-9_]*` other than `_` alone.
- `uname` is `[A-Z][A-Za-z0-9_]*`.
- `hname` additionally allows `-`.
- Comments run from `--` to end of line.

No `lname` or `uname` contains two underscores in a row (`a__b` is an
error), and no segment of a `ModName` ends with `_`: the compiler uses `__`
to join the parts of the names it generates (chapter 13).

> **`__` in an `hname`.** An `hname` (an HTML tag or attribute name such as
> `data-x__y`) may contain `__`.

A `rawtext` is every character up to the closing tag (in any case), kept as
written: no `{ … }`, tags or comments. It may not contain `</script` or
`</style` in any case (chapter 5, "Scripts and styles").
