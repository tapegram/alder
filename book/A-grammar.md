# A. Grammar

EBNF. `{x}` is zero or more, `[x]` is optional, `|` is alternation.
Newlines are significant only as statement separators inside blocks.
Two further line breaks are allowed: between the attributes of an HTML open
tag (and before its `>` or `/>`), and in a type, where a line may begin with
`->` and the type may begin on the line after the `:` of a signature.

```ebnf
module      = "module" ModName NL {import NL} {decl NL} ;
import      = "import" ModName ["as" uname] ;   (* the canonical qualifier, chapter 3: E0204 otherwise *)

decl        = valueDecl | typeDecl | pageDecl | msgDecl | formDecl
            | tableDecl | capDecl | fakeDecl | sessionDecl | testDecl | loginDecl ;

valueDecl   = lname ":" type NL lname {lname} "=" expr ;
typeDecl    = "type" uname {lname} "=" ( unionBody | recordType | brandType ) ;
unionBody   = ["|"] ctor {"|" ctor} ;
ctor        = uname [atype] ;
brandType   = "brand" uname ["where" expr] ;

pageDecl    = "page" string recordType ["requires" lname ["," "role" uname {"|" uname}]] ;
                                    (* the role clause: after "requires user" only, E0027 *)
msgDecl     = "msg" uname [recordType] ["form" uname] ;
formDecl    = "form" uname "=" "{" {fieldDef ","} [fieldDef] "}" ;
tableDecl   = "table" uname [renamed] "{" {fieldDef {attr} ","} [fieldDef {attr}] "}" ;
fieldDef    = lname ":" type ["=" expr] ;   (* in a table: the default is a literal, E0022 *)
attr        = "@id" | "@ref" uname | "@created" | "@updated" | "@unique"
            | "@cascade" | "@restrict" | "@setNull"    (* a delete rule: after "@ref T" only *)
            | "@owner"                                 (* one per table, on a "@ref" of the user: E0024 *)
            | renamed ;
renamed     = "@renamed" string ;   (* the old name; a plain string, no "{…}" *)
capDecl     = "capability" uname "{" {lname ":" type ","} [lname ":" type] "}" ;
fakeDecl    = "fake" uname "{" {tableDecl | lname {lname} "=" expr} "}" ;   (* beside its capability, E0020 *)
sessionDecl = "session" recordType ;
loginDecl   = "login" "=" ModName ;
testDecl    = "test" string "=" expr ;

type        = atype {atype} ["->" type] ;
atype       = uname | ModName "." uname | lname | recordType | "(" type ")" ;
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
lambda      = "\" lname {lname} "->" expr ;
ifExpr      = "if" expr "then" expr "else" expr ;
matchExpr   = "match" expr "{" {arm ","} [arm] "}" ;
arm         = pattern "->" expr ;
pattern     = "_" | literal | lname | uname [pattern1] | recordPat | listPat ;
pattern1    = "_" | literal | lname | uname | recordPat ;
recordPat   = "{" {lname ":" pattern1 ","} [lname ":" pattern1] "}" ;
listPat     = "[" "]" | "[" lname {"," lname} ["," ".." lname] "]" ;
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

Lexical: `lname` is `[a-z_][A-Za-z0-9_]*` other than `_` alone, `uname` is
`[A-Z][A-Za-z0-9_]*`, and `hname` additionally allows `-`. No `lname` or
`uname` contains two underscores in a row (`a__b` is an error), and no
segment of a `ModName` ends with `_`: the compiler uses `__` to join the
parts of the names it generates (chapter 13). An `hname` (an HTML tag or
attribute name such as `data-x__y`) may contain `__`. Comments run from `--` to end of line.
A `rawtext` is every character up to the closing tag (in any case), kept as
written: no `{ … }`, tags or comments; it may not contain `</script` or
`</style` in any case (chapter 5, "Scripts and styles").
