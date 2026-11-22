package check

import (
	"strings"
	"testing"
)

func workersApp(workers string) map[string]string {
	return map[string]string{"App.alder": `module App

error : Error -> Html
error err = <h1>Something went wrong</h1>

notFound : Html
notFound = <h1>Not found</h1>

deploy {
  production: Render "oregon" { service: "blog", instances: 2, workers: ` + workers + `, database: Turso "acme" { app: "blog" } },
}
`}
}

// `workers` is a whole number of at least 1, in `instances`' wording (E0030); a Render field only.
func TestRender_Workers(t *testing.T) {
	if _, got := checkFiles(t, workersApp("2")); len(got) != 0 {
		t.Errorf("workers: 2 should check: %v", got)
	}
	for _, bad := range []string{"0", `"2"`, "-1"} {
		_, got := checkFiles(t, workersApp(bad))
		if len(got) != 1 || !strings.Contains(got[0], "E0030") || !strings.Contains(got[0], "`workers` is how many worker instances Render runs: a whole number of at least 1") {
			t.Errorf("workers: %s: %v", bad, got)
		}
	}
	files := workersApp("2")
	files["App.alder"] = strings.Replace(files["App.alder"], `Render "oregon" { service: "blog", instances: 2, workers: 2, database: Turso "acme" { app: "blog" } }`, `Fly "ord" { app: "blog", workers: 2 }`, 1)
	if _, got := checkFiles(t, files); len(got) != 1 || !strings.Contains(got[0], "has no field `workers`") {
		t.Errorf("Fly with workers: %v", got)
	}
}
