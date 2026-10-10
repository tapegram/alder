package main

import "strings"

// mainKind is what a scratch build's main does with the scratch module's `it`.
type mainKind int

const (
	mainPure  mainKind = iota // prints rt.Show(it)
	mainQuery                 // runs it as a Query against ALDER_EVAL_DB and prints the result
	mainTask                  // runs it as a Task (writes commit) and prints the result
)

// scratchMain is the generated cmd/main.go of a scratch build: it evaluates the Go expression
// it (e.g. "app.Scratch__it") of the given kind. Query and Task mains open ALDER_EVAL_DB
// (created with the app's tables when absent, O_EXCL; an existing one checked against the
// schema and never rebuilt), run with Session app.TestSession(), Now time.Now and the
// capabilities of the adapters package when withAdapters (else none: a capability call fails
// with "capability X is not wired in eval"), and print rt.Show of the value; a failed effect
// prints its message to stderr and exits 1.
//
// The database file is created with O_EXCL, so of this process and an `alder run` starting at
// the same moment only the one that created it builds its tables.
func scratchMain(kind mainKind, withAdapters bool, it string) string {
	if kind == mainPure {
		return strings.ReplaceAll(pureMain, "{{it}}", it)
	}
	imp, caps := "", "app.Caps{}.Registry()"
	if withAdapters {
		imp, caps = "\t\"app/adapters\"\n", "adapters.Caps().Registry()"
	}
	run := "eff.RunQuery(env, " + it + ")"
	if kind == mainTask {
		run = "eff.Run(env, eff.KTask, " + it + ")"
	}
	return strings.NewReplacer("{{imports}}", imp, "{{caps}}", caps, "{{run}}", run).Replace(effectMain)
}

// pureMain prints the value of {{it}} as Alder (rt.Show).
const pureMain = `package main

import (
	"fmt"

	"alder/rt"
	"app"
)

func main() { fmt.Print(rt.Show({{it}})) }
`

// effectMain runs an effect ({{run}}) against ALDER_EVAL_DB with the app's default session and
// the capabilities {{caps}}, and prints its result; see scratchMain.
const effectMain = `package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"app"
{{imports}})

// unwired is the message of a capability call that finds no implementation.
var unwired = regexp.MustCompile(` + "`" + `alder: capability "([^"]*)" is not wired` + "`" + `)

func fail(msg string) {
	fmt.Fprintln(os.Stderr, unwired.ReplaceAllString(msg, "capability $1 is not wired in eval"))
	os.Exit(1)
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fail(fmt.Sprint(r))
		}
	}()
	path := os.Getenv("ALDER_EVAL_DB")
	created := false
	if f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644); err == nil {
		f.Close()
		created = true
	} else if !os.IsExist(err) {
		fail(err.Error())
	}
	store, err := db.OpenSQLite(path)
	if err != nil {
		fail(err.Error())
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Ensure(ctx, app.Tables, created); err != nil {
		fail(err.Error())
	}
	env := &eff.Env{Ctx: ctx, Store: store, Session: app.TestSession(), Caps: {{caps}}, Now: time.Now}
	v, err := {{run}}
	if err != nil {
		fail(err.Error())
	}
	fmt.Print(rt.Show(v))
}
`
