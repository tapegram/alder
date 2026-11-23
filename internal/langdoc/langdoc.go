// Package langdoc documents what no `.alder` file declares: the operators, `<-` in an effect
// block, the typed hole `?name`, the keywords, and the words that are keywords only in a job's
// line (`every`, `cron`, `retries`, `timeout`, `silo`, `priority`). Each entry's doc is in the doc comment
// format (internal/doc), so the stdlib's tests, renderer and hover take it as they take a
// declaration's.
package langdoc

import (
	"strings"

	"alder/internal/doc"
)

type Entry struct {
	Token     string // "|>", "match", "<-", "?" for a typed hole
	Signature string // "(|>) : a -> (a -> b) -> b"; "" for a keyword
	Doc       string // the doc comment format, without the `-- ` prefixes
}

// Lookup is the entry of token, as the lexer writes it ("|>", "match"; "?" for a hole).
func Lookup(token string) (Entry, bool) {
	i, ok := byToken[token]
	if !ok {
		return Entry{}, false
	}
	return entries[i], true
}

// All is every entry in a fixed order: operators, then `<-` and `?`, then keywords.
func All() []Entry { return append([]Entry(nil), entries...) }

// Markdown is the signature in a fenced block when there is one, then the rendered doc.
func (e Entry) Markdown() string {
	md := doc.Parse(e.Doc).Markdown("")
	if e.Signature == "" {
		return md
	}
	return "```alder\n" + e.Signature + "\n```\n\n" + md
}

var byToken = map[string]int{}

func init() {
	for i := range entries {
		// A raw string cannot hold a backtick, so the docs below write one as ˋ (U+02CB).
		entries[i].Doc = strings.ReplaceAll(strings.TrimPrefix(entries[i].Doc, "\n"), "ˋ", "`")
		byToken[entries[i].Token] = i
	}
}

const (
	operators = `Book: chapter 3, "Operators"`
	numberDef = "ˋnumberˋ stands for ˋIntˋ or ˋFloatˋ, and both sides must be the same one: ˋ1 + 1.5ˋ is an error. Convert an ˋIntˋ first with ˋInt.toFloatˋ. Two values of one numeric brand give that brand; a brand with its base, or two brands of one base, give the base."
	orderDef  = "ˋcomparableˋ stands for the types that have an order: ˋIntˋ, ˋFloatˋ, ˋTextˋ, ˋDateˋ, ˋTimeˋ, their brands, and unions whose constructors carry nothing. Both sides have the same type, except that a brand may be compared with its base, and two brands of one numeric base compare as that base. Text is ordered by its bytes, so every capital letter comes before every small one, and constructors are ordered as their union declares them."
	noChain   = "Comparisons do not chain: ˋa < b < cˋ is an error, so join two of them with ˋ&&ˋ. A comparison binds more loosely than arithmetic and ˋ++ˋ and more tightly than ˋ&&ˋ, so put it in parentheses to compare its result with ˋ==ˋ."
)

var entries = []Entry{
	{"|>", "(|>) : a -> (a -> b) -> b", `
Pass a value to a function, so that a chain of steps reads from left to right.

ˋx |> fˋ is ˋf xˋ. When the function on the right already has arguments, the value goes in as
the last one: ˋxs |> List.map fˋ is ˋList.map f xsˋ.

ˋ|>ˋ binds more loosely than every other operator, so ˋx |> f == yˋ reads as ˋx |> (f == y)ˋ.
Put the pipeline in parentheses to compare its result.

    ([1, 2, 3] |> List.map (\n -> n * 2)) == [2, 4, 6]
    ("  hi  " |> Text.trim |> Text.length) == 2

` + operators},
	{"??", "(??) : Maybe a -> a -> a", `
Take the value out of a ˋMaybeˋ, or use a default when there is none.

ˋm ?? dˋ is the value inside ˋmˋ when it is ˋJustˋ, and ˋdˋ when it is ˋNothingˋ. A chain of
them tries each ˋMaybeˋ in turn: ˋa ?? b ?? 0ˋ reads as ˋa ?? (b ?? 0)ˋ.

ˋ??ˋ binds more loosely than ˋ&&ˋ, ˋ||ˋ and the comparisons, so ˋready && m ?? Trueˋ reads as
ˋ(ready && m) ?? Trueˋ. Write ˋready && (m ?? True)ˋ when the default belongs to ˋmˋ alone.

    (Just 3 ?? 0) == 3
    (Nothing ?? 0) == 0
    (Nothing ?? Just 2 ?? 0) == 2

` + operators},
	{"&&", "(&&) : Bool -> Bool -> Bool", `
True when both sides are true.

The right side is only worked out when the left side is ˋTrueˋ. ˋ&&ˋ binds more tightly than
ˋ||ˋ, so ˋa || b && cˋ reads as ˋa || (b && c)ˋ.

    (True && False) == False
    (1 < 2 && 2 < 3) == True

` + operators},
	{"||", "(||) : Bool -> Bool -> Bool", `
True when either side is true, or both are.

The right side is only worked out when the left side is ˋFalseˋ.

    (False || True) == True
    (False || 2 < 1) == False

` + operators},
	{"==", "(==) : a -> a -> Bool", `
True when the two sides are the same value.

Both sides must have exactly the same type, so a brand is not equal to its base type: comparing
a ˋCentsˋ with an ˋIntˋ is an error even though a ˋCentsˋ may be used as an ˋIntˋ elsewhere.
Lists, records and union values are equal when everything inside them is equal.

Functions cannot be compared, and neither can effects such as ˋQueryˋ and ˋTaskˋ, nor a record,
list or union that holds one: the checker rejects them (E0003).

` + noChain + `

    [1, 2] == [1, 2]
    (Just "a" == Nothing) == False

` + operators},
	{"!=", "(!=) : a -> a -> Bool", `
True when the two sides are different values.

It is the opposite of ˋ==ˋ and follows the same rules: both sides have the same type, and
functions and effects cannot be compared.

    (1 != 2) == True
    ("a" != "a") == False

` + operators},
	{"<", "(<) : comparable -> comparable -> Bool", `
True when the left side comes before the right side.

` + orderDef + `

` + noChain + `

    (1 < 2) == True
    ("Zebra" < "apple") == True

` + operators},
	{"<=", "(<=) : comparable -> comparable -> Bool", `
True when the left side comes before the right side or is equal to it.

` + orderDef + `

    (2 <= 2) == True
    (3 <= 2) == False

` + operators},
	{">", "(>) : comparable -> comparable -> Bool", `
True when the left side comes after the right side.

` + orderDef + `

    (3 > 2) == True
    ("a" > "b") == False

` + operators},
	{">=", "(>=) : comparable -> comparable -> Bool", `
True when the left side comes after the right side or is equal to it.

` + orderDef + `

    (2 >= 2) == True
    (1.5 >= 2.0) == False

` + operators},
	{"+", "(+) : number -> number -> number", `
Add two numbers.

` + numberDef + ` To join ˋTextˋ, use ˋ++ˋ.

ˋIntˋ arithmetic that goes past the largest ˋIntˋ wraps around to the smallest.

    2 + 3 == 5
    0.5 + 0.25 == 0.75

` + operators},
	{"-", "(-) : number -> number -> number", `
Subtract the right number from the left one.

` + numberDef + `

A ˋ-ˋ written straight before a number makes a negative number, ˋ-3ˋ. There is no ˋ-xˋ for a
name: write ˋ0 - xˋ.

    10 - 4 == 6
    1.5 - 2.0 == -0.5

` + operators},
	{"*", "(*) : number -> number -> number", `
Multiply two numbers.

` + numberDef + `

ˋ*ˋ, ˋ/ˋ and ˋ%ˋ bind more tightly than ˋ+ˋ and ˋ-ˋ, so ˋ1 + 2 * 3ˋ is ˋ7ˋ.

    6 * 7 == 42
    1 + 2 * 3 == 7

` + operators},
	{"/", "(/) : number -> number -> number", `
Divide the left number by the right one.

` + numberDef + `

On ˋIntˋ the result is cut toward zero: ˋ7 / 2ˋ is ˋ3ˋ and ˋ-7 / 2ˋ is ˋ-3ˋ. Dividing an ˋIntˋ
by zero is a runtime failure, not a value. Inside a request the request gets the error page and
the server goes on; inside a test that test fails; in ˋalder evalˋ the evaluation stops with the
error. A top-level constant is worked out when the program starts, so one that divides by zero
stops the program before it serves or tests anything (chapter 4, "Int"). On ˋFloatˋ, dividing by
zero gives infinity or NaN instead.

    7 / 2 == 3
    -7 / 2 == -3
    7.0 / 2.0 == 3.5

` + operators},
	{"%", "(%) : number -> number -> number", `
The remainder left after dividing the left number by the right one.

` + numberDef + `

The result has the sign of the left side: ˋ-7 % 2ˋ is ˋ-1ˋ. The remainder of an ˋIntˋ by zero
is a runtime failure, as with ˋ/ˋ: chapter 4, "Int", says what it stops.

    7 % 3 == 1
    -7 % 2 == -1
    7.5 % 2.0 == 1.5

` + operators},
	{"++", "(++) : appendable -> appendable -> appendable", `
Join two pieces of text, or two lists, into one.

ˋappendableˋ stands for ˋTextˋ or a ˋListˋ: two texts, or two lists of the same element type.
To put one element in front of a list, make it a list first: ˋ[x] ++ xsˋ.

    "ab" ++ "cd" == "abcd"
    [1] ++ [2, 3] == [1, 2, 3]

` + operators},
	{"<-", "", `
Run an effect inside an effect block and name what it produces.

An effect block is a block whose last expression is a ˋQueryˋ or a ˋTaskˋ. In it,
ˋlet x <- eˋ runs ˋeˋ and binds its result to ˋxˋ for the lines below; ˋlet _ <- eˋ runs ˋeˋ
for its effect alone. The last expression is the result of the whole block.

In a ˋQueryˋ block the right side must be a ˋQueryˋ; in a ˋTaskˋ block it may be a ˋTaskˋ or a
ˋQueryˋ. A ˋTaskˋ in a ˋQueryˋ block is an error (E0401), and so is ˋ<-ˋ in a block that is not
an effect block (E0402).

    \(extra : Int) -> {
        let n <- Db.count (Db.from Todo)
        Query.succeed (n + extra)
      }

Book: chapter 7, "Building effects"`},
	{"?", "", `
A typed hole: a placeholder for an expression you have not written yet.

The program still compiles with holes in it, and ˋalder checkˋ reports each one as warning
W0002 with the type the missing expression must have.

ˋalder fill --listˋ lists every hole, and ˋalder fillˋ fills one with an expression you give
it or, with ˋ--aiˋ, one a model writes. In an editor, completion on a hole lists the names in
scope, those whose type fits first, and the code action ˋFill ?name with AIˋ fills it.

Book: chapter 11, "Holes"`},
	{"brand", "", `
Make a new type that is stored like a built-in type but kept apart from it by the checker.

ˋtype Cents = brand Intˋ may be used wherever an ˋIntˋ is expected, but an ˋIntˋ becomes a
ˋCentsˋ only through the constructor ˋCentsˋ, which exists only in the module that declares the
brand. Other modules get ˋCentsˋ values from that module, or decoded from a route, a form or a
database row. ˋbrandˋ is allowed only as the whole body of a ˋtypeˋ declaration.

Book: chapter 4, "Brands"`},
	{"capability", "", `
Declare an interface to the outside world, which the host implements in Go.

Each entry of ˋcapability Email { send : … }ˋ is a function whose result is a ˋQueryˋ or a
ˋTaskˋ, used like any other effect: ˋEmail.send { … }ˋ. The app does not build or start until
every capability is implemented and wired in. A test can use a ˋfakeˋ in its place.

Book: chapter 7, "Capabilities"`},
	{"cron", "", `
In a job's declaration, run the job at the times a cron expression matches.

ˋjob Nightly = {} cron "0 3 * * *"ˋ runs ˋNightly.runˋ at 03:00 every day. The expression has
five fields: minute (0 to 59), hour (0 to 23), day of month (1 to 31), month (1 to 12) and day
of week (0 to 6, Sunday is 0). Each field is ˋ*ˋ, a number or a range ˋa-bˋ, any of them with a
step (ˋ*/15ˋ), or a list of those joined by commas. There are no names, no seconds and no ˋLˋ,
ˋWˋ or ˋ#ˋ, and every time is UTC. When neither day field begins with ˋ*ˋ, a day matches if
either does; when one does (ˋ*ˋ, ˋ*/2ˋ), a day must match both. An expression that cannot be
read is E0050, and so is a restricted day of month with a ˋ*ˋ day of week when none of the listed
days exists in the listed months (ˋ0 0 30 2 *ˋ); ˋ0 0 31 2 1ˋ is accepted, and runs on Mondays in
February. Parameters on a scheduled job, and a job with both ˋcronˋ and ˋeveryˋ, are E0050 too.

Book: chapter 17, "Schedules"`},
	{"else", "", `
Start the branch an ˋifˋ takes when its condition is ˋFalseˋ.

Every ˋifˋ has an ˋelseˋ, and both branches have the same type. In markup, write
ˋelse Html.noneˋ to show nothing.

    (if 1 > 2 then "yes" else "no") == "no"

Book: chapter 3, "Conditionals"`},
	{"every", "", `
In a job's declaration, run the job at a fixed interval.

ˋjob Hourly = {} every "1h"ˋ runs ˋHourly.runˋ every hour. The interval is a whole number and a
unit, ˋmsˋ, ˋsˋ, ˋmˋ, ˋhˋ or ˋdˋ, and at least ˋ1sˋ. An interval that divides a day puts the runs
on its grid from midnight UTC: ˋevery "1h"ˋ runs on the hour. A scheduled job takes no
parameters, and a job has ˋeveryˋ or ˋcronˋ, not both (E0050).

Book: chapter 17, "Schedules"`},
	{"fake", "", `
Implement a capability in Alder, for tests.

ˋfake Stripe { … }ˋ defines every entry of ˋcapability Stripeˋ, each written
ˋname params = bodyˋ with no signature, and may declare tables of its own. It must be in the
module that declares the capability (E0020). A test installs it with ˋTest.fake Stripeˋ, and a
production build contains no fakes.

Book: chapter 10, "8. Faking a capability"`},
	{"for", "", `
Repeat a piece of markup once for each element of a list.

ˋ{for todo in todos -> <li>{todo.title}</li>}ˋ is written inside the braces of markup and makes
a ˋList Htmlˋ. There is no index; use ˋList.indexedˋ first when you need one.

    <ul>{for n in [1, 2] -> <li>{Text.fromInt n}</li>}</ul>

Book: chapter 5, "Control flow in HTML"`},
	{"form", "", `
Declare the fields a form posts, and their types.

ˋform NewTodo = { title: Text.NonEmpty, due: Maybe Date }ˋ lists them. A write route that names
it, ˋroute Create = POST "/todos" { } form NewTodoˋ, gets the decoded form as ˋparams.formˋ. A
field with a default is optional in the request.

Book: chapter 6, "ˋformˋ"`},
	{"if", "", `
Choose between two expressions with a ˋBoolˋ.

ˋif c then a else bˋ is ˋaˋ when ˋcˋ is ˋTrueˋ and ˋbˋ when it is ˋFalseˋ. There is always an
ˋelseˋ, and both branches have the same type. In markup, write ˋelse Html.noneˋ to show
nothing.

    (if 2 > 1 then "yes" else "no") == "yes"

Book: chapter 3, "Conditionals"`},
	{"import", "", `
Bring another module of the app into scope.

ˋimport Table.Todoˋ gives the module a short name, its qualifier, so this module can write
ˋTodo.findˋ and ˋTodo.TodoIdˋ. Standard library modules such as ˋListˋ and ˋTextˋ are always in
scope and are never imported.

Book: chapter 3, "Files and modules"`},
	{"in", "", `
Separate the name of a ˋforˋ loop from the list it walks.

In ˋ{for todo in todos -> …}ˋ, each element of ˋtodosˋ is named ˋtodoˋ in turn.

Book: chapter 5, "Control flow in HTML"`},
	{"job", "", `
Declare a background job: its name and the parameters it takes.

ˋjob SendWelcome = { user: User.UserId }ˋ declares the job ˋSendWelcomeˋ, whose code is
declared under its name in the same module: ˋSendWelcome.run : SendWelcome.Params -> Task Unitˋ.
ˋSendWelcome { user: id }ˋ builds a ˋJobˋ, and ˋJob.enqueueˋ puts it on the queue. The
parameters are stored as JSON: use table-column types, records, lists, ˋMaybeˋ and the app's own
types without functions. Any other parameter is E0049, "A job parameter that cannot be stored".
Options follow the parameters on the same line, in any order and each at most once: ˋeveryˋ,
ˋcronˋ, ˋretriesˋ, ˋtimeoutˋ, ˋsiloˋ and ˋpriorityˋ.

Book: chapter 17, "Declaring a job"`},
	{"let", "", `
Name a value inside a block.

ˋlet x = eˋ names ˋxˋ for the lines below it in the same block, and the block ends with one
expression, its result. A ˋletˋ may carry a type: ˋlet n : Int = …ˋ. In an effect block,
ˋlet x <- eˋ runs an effect instead.

    (\x -> {
        let y = x * 3
        y + 1
      }) 2 == 7

Book: chapter 3, "Blocks"`},
	{"login", "", `
Name the route a visitor is sent to when a route ˋrequiresˋ a session field they do not have.

ˋlogin = Auth.Login.Loginˋ names the module and then the route. It is allowed only in ˋAppˋ.

Book: chapter 6, "The ˋAppˋ module"`},
	{"match", "", `
Choose a branch by the shape of a value.

Each arm is ˋpattern -> expressionˋ, separated by commas, and the first arm whose pattern fits
runs. The arms must cover every case; ˋ_ˋ matches anything and is allowed only as the last arm.

    (match Just 3 { Just n -> n * 2, Nothing -> 0 }) == 6
    (match [1, 2] { [] -> "empty", _ -> "some" }) == "some"

Book: chapter 3, "Match"`},
	{"module", "", `
Name the module a file holds, on the file's first line.

The name is the file's path: ˋmodule Todos.Indexˋ lives in ˋapp/Todos/Index.alderˋ.

Book: chapter 3, "Files and modules"`},
	{"priority", "", `
In a job's declaration, set which queued runs are claimed first.

ˋpriority Nˋ is an integer, 0 by default and negative allowed: ˋjob Charge = { … } priority 10ˋ.
A run is claimed before every run of a lower priority, and higher goes first. The order is
strict: a lower priority waits while a higher one is due, and it is per store, so each silo
orders its own queue. Among runs of equal priority, the one due first is claimed first.

Book: chapter 17, "Priorities"`},
	{"requires", "", `
Serve a route only to a session that has a field set.

ˋrequires userˋ names a field of ˋSessionˋ whose type is ˋMaybe aˋ. The route is served only
when that field is ˋJustˋ; otherwise the visitor is sent to the route named by ˋloginˋ. Inside
the route the field has type ˋaˋ. ˋrequires user, role Authorˋ also answers 404 to a visitor
whose session ˋroleˋ is not ˋAuthorˋ.

Book: chapter 6, "ˋrequiresˋ"`},
	{"retries", "", `
In a job's declaration, set how many times a failed run is tried again.

ˋretries Nˋ is the number of attempts after the first: ˋjob Charge = { … } retries 0ˋ runs once,
and a job without ˋretriesˋ has 4, five attempts in all. The waits before them are 10 s, 1 min,
10 min, then 1 h before each later one. A job whose last attempt fails is dead, and so is one
whose parameters cannot be decoded, whatever its ˋretriesˋ.

Book: chapter 17, "Retries and idempotency"`},
	{"route", "", `
Declare a route: its name, its method, its path and the parameters it takes.

ˋroute Show = GET "/todos/{id}" { id: Todo.TodoId }ˋ declares the route ˋShowˋ. A write route
may add ˋform Fˋ for the fields posted to it, and any route may add ˋrequires …ˋ. The code of
a route is declared under its name: ˋShow.loadˋ, ˋShow.viewˋ, ˋToggle.updateˋ.

Book: chapter 6, "ˋrouteˋ"`},
	{"session", "", `
Declare the fields of the session, which the runtime keeps for each visitor in a signed cookie.

ˋsession { user: Maybe User.UserId }ˋ is allowed only in ˋAppˋ. Its field types follow the
rules of form fields, and ˋSession.putˋ changes it.

Book: chapter 6, "The ˋAppˋ module"`},
	{"silo", "", `
In a job's declaration, run the job in the silo that one of its parameters names.

ˋjob Digest = { org: Org.OrgId } silo orgˋ runs ˋDigest.runˋ bound to the silo whose key ˋorgˋ
is the job's ˋorgˋ parameter. ˋAppˋ must declare ˋsilo org { … }ˋ and a ˋMaybeˋ session field
ˋorgˋ, and the parameter has that field's type without the ˋMaybeˋ: ˋOrg.OrgIdˋ for a session
field ˋorg: Maybe Org.OrgIdˋ (E0033 otherwise). Without ˋsiloˋ, a job runs on the shared store.

Book: chapter 17, "Silos"`},
	{"table", "", `
Declare a database table and the record type of its rows.

ˋtable Todo { … }ˋ defines the record type ˋTodoˋ and the storage for it. A field may carry
attributes such as ˋ@idˋ and ˋ@refˋ, and a default after ˋ=ˋ.

Book: chapter 8, "Tables"`},
	{"test", "", `
Declare a test, run by ˋalder testˋ.

ˋtest "name" = expressionˋ passes when its body is ˋTrueˋ. A body that is a block is an effect
test: a ˋTask Boolˋ that may run queries and tasks and ends with ˋTask.succeedˋ of the verdict.

Book: chapter 10, "1. A test is a ˋBoolˋ"`},
	{"then", "", `
Start the branch an ˋifˋ takes when its condition is ˋTrueˋ.

    (if 2 > 1 then "yes" else "no") == "yes"

Book: chapter 3, "Conditionals"`},
	{"timeout", "", `
In a job's declaration, set the most time one attempt may take.

ˋjob Report = {} cron "0 6 * * 1" timeout "20m"ˋ gives each attempt of ˋReport.runˋ 20
minutes. The duration is written as ˋeveryˋ's is; without ˋtimeoutˋ it is 5 minutes, and it is
at most ˋ1hˋ (E0050). An attempt that runs out of time fails with "timed out after" its limit
and is retried like any other failure.

Book: chapter 17, "The options"`},
	{"type", "", `
Declare a new type: a record, a union or a brand.

ˋtype Point = { x: Float, y: Float }ˋ is a record, ˋtype Filter = All | Active | Completedˋ a
union of constructors, and ˋtype Cents = brand Intˋ a brand. A new name is always a new type:
there are no aliases.

Book: chapter 3, "Declarations"`},
	{"where", "", `
Add a check to a brand, run when a value of it comes in from outside the program.

ˋtype Age = brand Int where (\n -> n >= 0 && n < 150)ˋ checks the value when it is decoded from
a route, a form or a database row: a failing form shows an error and a failing route is a 404.
Inside the program the value is trusted and not checked again.

Book: chapter 4, "Refinements"`},
}
