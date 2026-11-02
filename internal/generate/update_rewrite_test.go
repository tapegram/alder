package generate

import (
	"testing"

	"alder/internal/syntax"
)

// updatePage is a minimal page whose `msg` declarations are msgs and whose `update` is
// `update <update>`.
func updatePage(msgs, update string) string {
	return "module Home\n\npage \"/\" { }\n\n" + msgs + "update : Msg -> Session -> Task Nav\nupdate " + update +
		"\n\nview : Session -> Html Msg\nview session = <p>Hi</p>\n"
}

// matchify parses src and runs matchifyUpdate on it with added.
func matchify(t *testing.T, src string, added ...string) (string, error) {
	t.Helper()
	m, ds := syntax.ParseFile("Home.alder", []byte(src))
	if len(ds) > 0 {
		t.Fatalf("parse: %v\n%s", ds, src)
	}
	return matchifyUpdate(src, m, added)
}

func wantMatchify(t *testing.T, src, want string, added ...string) {
	t.Helper()
	got, err := matchify(t, src, added...)
	if err != nil {
		t.Fatalf("matchifyUpdate: %v", err)
	}
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMatchifyUpdate_OneLineMatch(t *testing.T) {
	msgs := "msg Other\n\nmsg Again\n\n"
	wantMatchify(t,
		updatePage(msgs, "msg session = match msg { Other -> Task.succeed Rerender, Again -> Task.succeed (Redirect (Home {})) }"),
		updatePage(msgs, "msg session = match msg {\n  Other -> Task.succeed Rerender,\n  Again -> Task.succeed (Redirect (Home {})),\n}"),
		"Submit")
}

func TestMatchifyUpdate_BraceSharesLine(t *testing.T) {
	msgs := "msg Other\n\n"
	wantMatchify(t,
		updatePage(msgs, "msg session = match msg {\n  Other -> {\n    let _ <- Log.info \"x\"\n    Task.succeed Rerender\n  } }"),
		updatePage(msgs, "msg session = match msg {\n  Other -> {\n    let _ <- Log.info \"x\"\n    Task.succeed Rerender\n  },\n}"),
		"Submit")
}

func TestMatchifyUpdate_CatchAll(t *testing.T) {
	msgs := "msg Other\n\n"
	t.Run("body", func(t *testing.T) {
		wantMatchify(t,
			updatePage(msgs, "msg session = Task.succeed Rerender"),
			updatePage(msgs, "msg session = match msg {\n  _ -> Task.succeed Rerender,\n}"),
			"Submit")
	})
	t.Run("hole", func(t *testing.T) { // the parameter's own name is the scrutinee
		wantMatchify(t,
			updatePage(msgs, "m session = ?update"),
			updatePage(msgs, "m session = match m {\n  _ -> ?update,\n}"),
			"Submit")
	})
	t.Run("several lines", func(t *testing.T) {
		wantMatchify(t,
			updatePage(msgs, "msg session = {\n  let _ <- Log.info \"x\"\n  Task.succeed Rerender\n}"),
			updatePage(msgs, "msg session = match msg {\n  _ -> {\n    let _ <- Log.info \"x\"\n    Task.succeed Rerender\n  },\n}"),
			"Submit")
	})
}

func TestMatchifyUpdate_HoleNoOtherCtor(t *testing.T) {
	t.Run("no messages", func(t *testing.T) {
		wantMatchify(t, updatePage("", "msg session = ?update"), updatePage("", "msg session = match msg {\n}"), "Submit")
	})
	t.Run("only the added one", func(t *testing.T) {
		msgs := "msg Submit\n\n"
		wantMatchify(t, updatePage(msgs, "msg session = ?update"), updatePage(msgs, "msg session = match msg {\n}"), "Submit")
	})
}

func TestMatchifyUpdate_RefuseBodyNoOtherCtor(t *testing.T) {
	_, err := matchify(t, updatePage("", "msg session = Task.succeed Rerender"), "Submit")
	want := "`update` handles no message yet, and its body is not a hole: write it as `match msg { … }` yourself"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestMatchifyUpdate_RefuseCommentInMatch(t *testing.T) {
	_, err := matchify(t, updatePage("msg Other\n\n", "msg session = match msg {\n  -- the other one\n  Other -> Task.succeed Rerender }"), "Submit")
	if _, ok := err.(*Refusal); !ok {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestMatchifyUpdate_AlreadyMultiLine(t *testing.T) {
	for _, src := range []string{
		updatePage("msg Other\n\n", "msg session = match msg {\n  Other -> Task.succeed Rerender, -- a comment\n}"),
		updatePage("msg Other\n\n", "msg session = match msg {\n}"),
		"module Home\n\npage \"/\" { }\n\nview : Session -> Html Never\nview session = <p>Hi</p>\n", // no update
	} {
		wantMatchify(t, src, src, "Submit")
	}
}

// TestUpdateArmSpot_BeforeCatchAll: a new arm goes before a trailing `_` arm, where it is
// reachable, and before the `}` otherwise.
func TestUpdateArmSpot_BeforeCatchAll(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"catch-all", updatePage("msg Other\n\n", "msg session = match msg {\n  Other -> Task.succeed Rerender,\n  _ -> Task.succeed Rerender,\n}"), 10},
		{"no catch-all", updatePage("msg Other\n\n", "msg session = match msg {\n  Other -> Task.succeed Rerender,\n}"), 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := syntax.ParseFile("Home.alder", []byte(tc.src))
			line, ind, has, ok := updateArmSpot(newSrcEdit(tc.src), m)
			if !ok || !has || line != tc.want || ind != "  " {
				t.Fatalf("updateArmSpot = %d %q %v %v, want %d", line, ind, has, ok, tc.want)
			}
		})
	}
}
