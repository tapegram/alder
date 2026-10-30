package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"alder/internal/deploy"
	"alder/internal/ir"
	"alder/internal/version"
)

const ciUsage = "usage: alder ci init [dir] [--github] [--force]"

// ciWorkflowPath is where `alder ci init` writes the workflow, relative to the app's root.
const ciWorkflowPath = ".github/workflows/alder.yml"

// runCiTo is `alder ci init [dir] [--github] [--force]` (chapter 16): it writes
// .github/workflows/alder.yml from App's `deploy` declaration (ciWorkflow): on a pull request
// `alder check`, `alder test --mutate` (with the declared floor) and, when `preview` is
// declared, `alder deploy preview` for the pull request's host and a comment with its address;
// on a push to main the same check and `alder deploy production`. It refuses to replace an
// existing file without --force, and lints the file with actionlint when it is on PATH. GitHub
// Actions is the only CI it writes for (--github, the default). Exit 0, 1 when actionlint
// finds a problem, 2 for usage errors.
func runCiTo(args []string, stdout, stderr io.Writer) int {
	var pos []string
	force := false
	for _, a := range args {
		switch {
		case a == "--github":
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, ciUsage)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 || pos[0] != "init" || len(pos) > 2 {
		fmt.Fprintln(stderr, ciUsage)
		return 2
	}
	dir := "."
	if len(pos) == 2 {
		dir = pos[1]
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "alder ci init: "+format+"\n", a...)
		return 2
	}
	r := &deployRunner{dir: dir}
	errs, _, _, out := r.Check()
	if r.loadErr != "" {
		return fail("%s", r.loadErr)
	}
	if errs > 0 {
		return fail("the app does not check (the workflow is made from App's `deploy`): fix it first\n%s", strings.TrimRight(out, "\n"))
	}
	if r.decl != nil && hasRender(r.decl) {
		// the workflow pushes an image: it needs a registry path, here or from the GitHub origin (R5)
		if _, err := imagePath(dir, r.decl); err != nil {
			return fail("%v", err)
		}
	}
	path := filepath.Join(dir, filepath.FromSlash(ciWorkflowPath))
	if _, err := os.Stat(path); err == nil && !force {
		return fail("%s exists: remove it, or pass --force to replace it", ciWorkflowPath)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail("%v", err)
	}
	wf := ciWorkflow(r.decl, version.String(), goMinor(version.Go()))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(path, []byte(wf.YAML), 0o644); err != nil {
		return fail("%v", err)
	}
	fmt.Fprintf(stdout, "wrote %s\n", ciWorkflowPath)
	for _, l := range wf.Summary {
		fmt.Fprintln(stdout, "  "+l)
	}
	for _, n := range wf.Notes {
		fmt.Fprintln(stdout, n)
	}
	names := []string{}
	for _, s := range wf.Secrets {
		names = append(names, s.Name)
	}
	if len(names) == 0 {
		fmt.Fprintln(stdout, "repository secrets it needs: none")
	} else {
		fmt.Fprintf(stdout, "repository secrets it needs: %s (the file's header says what each is)\n", strings.Join(names, ", "))
	}
	if lint, err := exec.LookPath("actionlint"); err == nil {
		cmd := exec.Command(lint, path)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(stderr, "%s actionlint finds problems in %s:\n%s", errTag(stderr, "alder ci init:"), ciWorkflowPath, out)
			return 1
		}
		fmt.Fprintln(stdout, "actionlint ok")
	}
	return 0
}

// goMinor is "1.27" of "1.27.0".
func goMinor(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return v
}

// workflow is the file `alder ci init` writes and what it tells the user.
type workflow struct {
	YAML    string
	Summary []string // "pull request  …", "push to main  …"
	Notes   []string
	Secrets []ciSecret
}

// ciSecret is a repository secret the workflow reads.
type ciSecret struct{ Name, What string }

// ciDeploy is one deploy the workflow runs: the environment, its target, and what the job
// needs.
type ciDeploy struct {
	env    string
	t      ir.Target
	host   string // the pull request's host for a preview (a shell expression of $PR)
	url    string // the address the preview comment names
	values []string
	// fromJSON: the address is not known when the file is written (a Render service's URL), so
	// the deploy step prints it with --json and the comment step reads it from there.
	fromJSON bool
}

// ciWorkflow is the GitHub Actions workflow of declaration d (nil: none), installing alder
// version alderVersion (a release tag such as v0.1.0; anything else installs the latest
// release) with Go goVersion.
func ciWorkflow(d *ir.Deploy, alderVersion, goVersion string) workflow {
	var wf workflow
	mutate := "alder test --mutate"
	if d != nil && d.FloorSet {
		mutate += " --min " + strconv.FormatFloat(d.Floor, 'f', -1, 64)
	}
	var preview, production *ciDeploy
	if d != nil {
		if t, ok := d.Envs["preview"]; ok {
			preview = &ciDeploy{env: "preview", t: t}
			switch t.Kind {
			case "ssh":
				h := t.Domain
				if rest, ok := strings.CutPrefix(h, "*."); ok {
					h = rest
				}
				preview.host = "${{ github.event.pull_request.number }}." + h
				preview.url = "https://$HOST"
			case "fly":
				preview.url = "https://" + t.App + ".fly.dev"
			case "render":
				preview.url, preview.fromJSON = "$ADDRESS", true
			case "script":
				preview.host = "pr-${{ github.event.pull_request.number }}"
			}
		}
		if t, ok := d.Envs["production"]; ok {
			production = &ciDeploy{env: "production", t: t}
		}
	}
	// the secrets, in the order the header lists them
	add := func(name, what string) {
		for _, s := range wf.Secrets {
			if s.Name == name {
				return
			}
		}
		wf.Secrets = append(wf.Secrets, ciSecret{name, what})
	}
	var sshHosts, boxEnv, flyApps, renderEnvs, tursoOrgs []string
	valueEnvs := map[string][]string{} // a value the job reads -> the environments that read it
	var valueNames []string
	for _, c := range []*ciDeploy{production, preview} {
		if c == nil {
			continue
		}
		switch c.t.Kind {
		case "ssh":
			host := c.t.Host
			if c.t.User != "" {
				if _, h, ok := strings.Cut(host, "@"); ok {
					host = h
				}
				host = c.t.User + "@" + host
			}
			if !slices.Contains(sshHosts, host) {
				sshHosts = append(sshHosts, host)
			}
			boxEnv = append(boxEnv, c.env)
		case "fly", "script", "render":
			if c.t.Kind == "fly" && !slices.Contains(flyApps, c.t.App) {
				flyApps = append(flyApps, c.t.App)
			}
			if c.t.Kind == "render" {
				renderEnvs = append(renderEnvs, c.env)
				if db := c.t.Database; db != nil && !slices.Contains(tursoOrgs, db.Org) {
					tursoOrgs = append(tursoOrgs, db.Org)
				}
			}
			c.values = append(append([]string{}, d.Requires...), deploy.SecretName)
			for _, n := range c.values {
				if valueEnvs[n] == nil {
					valueNames = append(valueNames, n)
				}
				valueEnvs[n] = append(valueEnvs[n], c.env)
			}
		}
	}
	switch len(flyApps) {
	case 0:
	case 1:
		add("FLY_API_TOKEN", "a deploy token of the Fly app "+flyApps[0]+": fly tokens create deploy -a "+flyApps[0])
	default:
		add("FLY_API_TOKEN", "a token that deploys the Fly apps "+strings.Join(flyApps, " and ")+": fly tokens create org")
	}
	if len(renderEnvs) > 0 {
		add("RENDER_API_KEY", "a Render API key of the workspace that owns the service")
		add("TURSO_API_TOKEN", "a Turso API token of the organization "+strings.Join(tursoOrgs, " and ")+": turso auth api-tokens mint alder-ci")
		add("ALDER_REGISTRY_TOKEN", "a personal access token with read:packages and write:packages (see below)")
	}
	if len(sshHosts) > 0 {
		var bare []string
		for _, h := range sshHosts {
			_, b, ok := strings.Cut(h, "@")
			if !ok {
				b = h
			}
			bare = append(bare, b)
		}
		add("SSH_PRIVATE_KEY", "a private key "+strings.Join(sshHosts, " and ")+" accepts ("+strings.Join(boxEnv, ", ")+")")
		add("SSH_KNOWN_HOSTS", "the host key lines of "+strings.Join(bare, " and ")+": ssh-keyscan "+strings.Join(bare, " "))
	}
	for _, n := range valueNames {
		what := "named in App's `requires`; " + strings.Join(valueEnvs[n], " and ") + " read" + map[bool]string{true: "s", false: ""}[len(valueEnvs[n]) == 1] + " it here"
		if n == deploy.SecretName {
			what = "the session key (16 bytes or more) of " + strings.Join(valueEnvs[n], " and ")
		}
		add(n, what)
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	para := func(text string) { b.WriteString(commentWrap(text, "# ", 96)) }
	w("# Written by `alder ci init` (book chapter 16). The pipeline is alder's own commands: what\n")
	w("# runs here runs the same from a laptop.\n#\n")
	prLine := "alder fmt --check; alder check; " + mutate
	pushLine := "alder fmt --check; alder check; " + mutate
	if preview != nil {
		prLine += "; alder deploy preview"
		if preview.host != "" {
			prLine += " --host " + strings.Replace(preview.host, "${{ github.event.pull_request.number }}", "<number>", 1)
		}
		if preview.url != "" {
			prLine += ", and a comment with its address"
		}
	}
	if production != nil {
		pushLine += "; alder deploy production"
	}
	b.WriteString(commentWrap(prLine, "#   pull request   ", 96))
	b.WriteString(commentWrap(pushLine, "#   push to main   ", 96))
	b.WriteString("#\n")
	wf.Summary = []string{"pull request  " + strings.ReplaceAll(prLine, "; ", ", "), "push to main  " + strings.ReplaceAll(pushLine, "; ", ", ")}
	if preview == nil && production == nil {
		wf.Notes = append(wf.Notes, "no `production` or `preview` environment in App's `deploy`: the workflow deploys nothing")
	}
	if preview != nil && preview.t.Kind == "ssh" && !strings.HasPrefix(preview.t.Domain, "*.") {
		wf.Notes = append(wf.Notes, fmt.Sprintf("the preview domain %s is not a wildcard: each pull request is served at <number>.%s, which needs a DNS record for each name (declare \"*.%s\")", preview.t.Domain, preview.t.Domain, preview.t.Domain))
	}
	if len(wf.Secrets) == 0 {
		w("# Repository secrets: none.\n")
	} else {
		w("# Repository secrets (Settings > Secrets and variables > Actions):\n")
		width := 0
		for _, s := range wf.Secrets {
			width = max(width, len(s.Name))
		}
		for _, s := range wf.Secrets {
			w("#   %-*s  %s\n", width, s.Name, s.What)
		}
	}
	if len(boxEnv) > 0 {
		names := append(append([]string{}, d.Requires...), deploy.SecretName)
		w("#\n")
		para(fmt.Sprintf("Not repository secrets: %s of %s live in the box's environment file (<path>/env, NAME=value lines); `alder deploy` checks that they are set there.", strings.Join(names, ", "), strings.Join(boxEnv, " and ")))
	}
	if len(renderEnvs) > 0 {
		w("#\n")
		para("ALDER_REGISTRY_TOKEN must be a personal access token with `read:packages` and `write:packages`, not the job's own token (`github.token`): alder pushes the image with it and keeps Render's registry credential set to it, and Render pulls the image with that credential on every deploy and every restart, long after a run's token has expired.")
		w("#\n")
		para("The first deploy creates the Render service (`alder deploy` does it, from App's `deploy`), so Render's dashboard is for looking.")
	}
	if preview != nil {
		w("#\n")
		switch preview.t.Kind {
		case "render":
			para("Every pull request deploys to the one Render service " + preview.t.Service + ": the last push wins. ")
		case "fly":
			para("Every pull request deploys to the one Fly app " + preview.t.App + ": the last push wins. ")
		case "ssh":
			if !strings.HasPrefix(preview.t.Domain, "*.") {
				para("The preview domain " + preview.t.Domain + " is not a wildcard: each pull request is served at <number>." + preview.t.Domain + ", which needs a DNS record (and a certificate) for each such name; declare \"*." + preview.t.Domain + "\" to say so.")
			}
		}
		para("A preview stays up after its pull request is closed: remove it by hand for now.")
	}
	if d == nil || !d.FloorSet {
		w("#\n")
		para("No `floor` is declared: `alder deploy` holds the score to the last deploy's.")
	}
	w("name: alder\n\n")
	w("on:\n  pull_request:\n  push:\n    branches: [main]\n\n")
	w("permissions:\n  contents: read\n\n")
	pinned := strings.HasPrefix(alderVersion, "v") && !strings.Contains(alderVersion, "devel")
	if pinned {
		w("env:\n  ALDER_VERSION: %s\n\n", alderVersion)
	}
	setup := func(c *ciDeploy) {
		w("    steps:\n")
		w("      - uses: actions/checkout@v4\n")
		w("      - uses: actions/setup-go@v5\n        with:\n          go-version: %q\n", goVersion)
		w("      - name: Install alder\n        env:\n          GH_TOKEN: ${{ github.token }}\n        run: |\n")
		if pinned {
			w("          gh release download \"$ALDER_VERSION\" --repo tapegram/alder --pattern 'alder_*_linux_amd64.tar.gz' --output \"$RUNNER_TEMP/alder.tar.gz\"\n")
		} else {
			w("          # the latest release: this file was written by a development build of alder (%s)\n", alderVersion)
			w("          gh release download --repo tapegram/alder --pattern 'alder_*_linux_amd64.tar.gz' --output \"$RUNNER_TEMP/alder.tar.gz\"\n")
		}
		w("          tar -xzf \"$RUNNER_TEMP/alder.tar.gz\" -C \"$RUNNER_TEMP\" alder\n")
		w("          echo \"$RUNNER_TEMP\" >> \"$GITHUB_PATH\"\n")
		if c == nil {
			return
		}
		switch c.t.Kind {
		case "ssh":
			w("      - name: SSH key\n        env:\n          SSH_PRIVATE_KEY: ${{ secrets.SSH_PRIVATE_KEY }}\n          SSH_KNOWN_HOSTS: ${{ secrets.SSH_KNOWN_HOSTS }}\n        run: |\n")
			w("          install -m 700 -d ~/.ssh\n")
			w("          printf '%%s\\n' \"$SSH_PRIVATE_KEY\" > ~/.ssh/id_ed25519\n")
			w("          chmod 600 ~/.ssh/id_ed25519\n")
			w("          printf '%%s\\n' \"$SSH_KNOWN_HOSTS\" >> ~/.ssh/known_hosts\n")
		case "fly":
			w("      - uses: superfly/flyctl-actions/setup-flyctl@ed8efb33836e8b2096c7fd3ba1c8afe303ebbff1 # 1.6\n")
		}
	}
	deployStep := func(c *ciDeploy) {
		w("      - name: alder deploy %s\n", c.env)
		if c.fromJSON {
			w("        id: deploy\n")
		}
		var env []string
		switch c.t.Kind {
		case "fly":
			env = append(env, "FLY_API_TOKEN")
		case "render":
			env = append(env, "RENDER_API_KEY", "TURSO_API_TOKEN", "ALDER_REGISTRY_TOKEN")
		}
		env = append(env, c.values...)
		if len(env) > 0 {
			w("        env:\n")
			for _, n := range env {
				w("          %s: ${{ secrets.%s }}\n", n, n)
			}
		}
		if c.fromJSON {
			// progress goes to stderr, the record to stdout: the address is the record's
			w("        run: |\n")
			w("          alder deploy %s --json > \"$RUNNER_TEMP/deploy.json\"\n", c.env)
			w("          echo \"address=$(jq -r .address \"$RUNNER_TEMP/deploy.json\")\" >> \"$GITHUB_OUTPUT\"\n")
		} else if c.host != "" {
			w("        run: alder deploy %s --host \"$HOST\"\n", c.env)
		} else {
			w("        run: alder deploy %s\n", c.env)
		}
	}

	w("jobs:\n")
	w("  check:\n    runs-on: ubuntu-latest\n")
	setup(nil)
	w("      - run: alder fmt --check\n")
	w("      - run: alder check\n")
	w("      - run: %s\n", mutate)
	if preview != nil {
		w("\n  preview:\n")
		w("    # a pull request from a fork gets no secrets: it is checked, not deployed\n")
		w("    if: github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name == github.repository\n")
		w("    needs: check\n    runs-on: ubuntu-latest\n")
		w("    permissions:\n      contents: read\n")
		if preview.t.Kind == "render" {
			w("      packages: write\n")
		}
		w("      pull-requests: write\n")
		w("    concurrency:\n      group: preview-${{ github.event.pull_request.number }}\n")
		if preview.host != "" {
			w("    env:\n      HOST: %s\n", preview.host)
		}
		setup(preview)
		deployStep(preview)
		if preview.url != "" {
			w("      - name: Comment the address\n        env:\n          GH_TOKEN: ${{ github.token }}\n          PR: ${{ github.event.pull_request.number }}\n          SHA: ${{ github.event.pull_request.head.sha }}\n")
			if preview.fromJSON {
				w("          ADDRESS: ${{ steps.deploy.outputs.address }}\n")
			}
			w("        run: |\n")
			w("          body=\"Preview of $SHA: %s\"\n", preview.url)
			w("          gh pr comment \"$PR\" --edit-last --body \"$body\" || gh pr comment \"$PR\" --body \"$body\"\n")
		}
	}
	if production != nil {
		w("\n  production:\n")
		w("    if: github.event_name == 'push' && github.ref == 'refs/heads/main'\n")
		w("    needs: check\n    runs-on: ubuntu-latest\n")
		if production.t.Kind == "render" {
			w("    permissions:\n      contents: read\n      packages: write\n")
		}
		w("    concurrency:\n      group: production\n      cancel-in-progress: false\n")
		setup(production)
		deployStep(production)
	}
	wf.YAML = b.String()
	return wf
}

// commentWrap is text as comment lines of at most width columns, the first after prefix, the
// rest indented to match with "#".
func commentWrap(text, prefix string, width int) string {
	var b strings.Builder
	indent := "#" + strings.Repeat(" ", len(prefix)-1)
	line := prefix
	empty := true
	for _, word := range strings.Fields(text) {
		if !empty && len(line)+1+len(word) > width {
			b.WriteString(line + "\n")
			line, empty = indent, true
		}
		if !empty {
			line += " "
		}
		line += word
		empty = false
	}
	b.WriteString(line + "\n")
	return b.String()
}

// hasRender reports whether any of d's environments is a Render service.
func hasRender(d *ir.Deploy) bool {
	for _, t := range d.Envs {
		if t.Kind == "render" {
			return true
		}
	}
	return false
}
