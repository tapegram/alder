package fill

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// effectsApp is holeApp with a capability, helpers that reach it or a standard effect directly,
// through one or two other values, or through a cycle, pure helpers, and in Home a helper of its
// own that pauses.
func effectsApp(t *testing.T) string {
	t.Helper()
	dir := holeApp(t, map[string]string{
		"app/Email.alder": "module Email\n\ncapability Email {\n  send : Text -> Task Unit,\n}\n",
		"app/Mail.alder":  "module Mail\n\nimport Email\n\nnotify : Text -> Task Unit\nnotify t = Email.send t\n",
		"app/Chain.alder": "module Chain\n\nimport Mail\n\nwarn : Text -> Task Unit\nwarn t = Mail.notify t\n",
		"app/Lib.alder":   "module Lib\n\npause : Task Unit\npause = Task.sleep 10\n",
		"app/Loop.alder":  "module Loop\n\nping : Int -> Task Int\nping n = if n > 0 then pong (n - 1) else Task.succeed n\n\npong : Int -> Task Int\npong n = if n > 5 then Task.map (\\u -> n) (Task.sleep 1) else ping n\n",
		"app/Ring.alder":  "module Ring\n\nodd : Int -> Bool\nodd n = if n == 0 then False else even (n - 1)\n\neven : Int -> Bool\neven n = if n == 0 then True else odd (n - 1)\n",
		"app/Pure.alder":  "module Pure\n\nhome : Task Nav\nhome = Task.succeed Rerender\n",
	})
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	os.WriteFile(home, append(b, []byte("\nsend : Task Nav\nsend = {\n  let _ <- Task.sleep 1\n  Task.succeed Rerender\n}\n")...), 0o644)
	return dir
}

func TestEffects_Semantic(t *testing.T) {
	dir := effectsApp(t)
	then := func(call string) string { return "{\n  let _ <- " + call + "\n  Task.succeed Rerender\n}" }
	for _, c := range []struct {
		name, expr string
		want       []string
	}{
		{"capability call", then(`Email.send "hi"`), []string{"capability Email.send"}},
		{"same-module helper", "send", []string{"Home.send → Task.sleep"}},
		{"imported helper", then(`Mail.notify "hi"`), []string{"Mail.notify → Email.send"}},
		{"imported std helper", then("Lib.pause"), []string{"Lib.pause → Task.sleep"}},
		{"two-hop chain", then(`Chain.warn "hi"`), []string{"Chain.warn → Mail.notify → Email.send"}},
		{"mutual recursion", then("Loop.ping 3"), []string{"Loop.ping → Loop.pong → Task.sleep"}},
		{"pure cycle", "if Ring.odd 3 then Task.succeed Rerender else stay", nil},
		{"pure helper", "Pure.home", nil},
		{"same-module pure value", "stay", nil},
		{"scoped write", then("Db.insertMine Post { title: p.form.title, body: p.form.body }"), nil},
		{"session", then(`Session.put (\s -> s)`), []string{"Session.put"}},
		{"two at once", "{\n  let _ <- Email.send \"a\"\n  let _ <- Lib.pause\n  Task.succeed Rerender\n}", []string{"Lib.pause → Task.sleep", "capability Email.send"}},
	} {
		r, err := Run(context.Background(), &script{answers: []string{c.expr}}, dir, "Home.update.Submit", "", nil)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !slices.Equal(r.Effects, c.want) {
			t.Errorf("%s: effects %q, want %q", c.name, r.Effects, c.want)
		}
		if !strings.Contains(r.Placed, strings.Split(strings.TrimPrefix(c.expr, "{\n  "), "\n")[0]) {
			t.Errorf("%s: placed %q", c.name, r.Placed)
		}
	}
}

// TestEffects_OnlyTheExpression: effects elsewhere in the declaration are not the fill's.
func TestEffects_OnlyTheExpression(t *testing.T) {
	dir := effectsApp(t)
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	// an effect in the arm, before the hole
	src := strings.Replace(string(b), "    let _ <- Db.insertMine Post", "    let _ <- Task.sleep 1\n    let _ <- Db.insertMine Post", 1)
	if src == string(b) {
		t.Fatal("no arm to add an effect to")
	}
	os.WriteFile(home, []byte(src), 0o644)
	r, err := Run(context.Background(), &script{answers: []string{"Pure.home"}}, dir, "Home.update.Submit", "", nil)
	if err != nil || len(r.Effects) != 0 {
		t.Errorf("effects %q, %v", r.Effects, err)
	}
}

func TestExcerpt(t *testing.T) {
	if got := Excerpt("a\nb", []int{1}); got != "a\nb" {
		t.Errorf("short: %q", got)
	}
	var ls []string
	for i := range 50 {
		ls = append(ls, "line"+string(rune('A'+i%26)))
	}
	ls[45] = "  let _ <- Email.send x"
	got := Excerpt(strings.Join(ls, "\n"), []int{45})
	if got != "…\n  let _ <- Email.send x\n…" {
		t.Errorf("long: %q", got)
	}
}
