package check

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// The `deploy` declaration (chapter 16). It is data for `alder deploy`: the checker only refuses
// what the command could not use (E0030) and compares `requires` with the environment variables
// the adapters read (W0006). Nothing of it reaches the program.

// checkDeploy checks App's deploy declarations (decls: every one App declares).
func (mc *modCtx) checkDeploy(decls []*syntax.DeployDecl) {
	if len(decls) == 0 {
		return
	}
	c := mc.c
	for _, extra := range decls[1:] {
		c.errorf(diag.DeployDecl, extra.Span, "`deploy` is declared once: merge this one into the first")
	}
	d := decls[0]
	seen := map[string]bool{}
	var requires *syntax.DeployField
	for _, f := range d.Fields {
		if seen[f.Name] {
			what := "`" + f.Name + "` is declared twice in `deploy`"
			if !deployKnownField[f.Name] {
				what = "environment `" + f.Name + "` is declared twice in `deploy`"
			}
			c.errorf(diag.DeployDecl, f.NameSpan, "%s: declare it once", what)
			continue
		}
		seen[f.Name] = true
		if f.Name == "requires" {
			requires = f
		}
		mc.deployField(f)
	}
	named := map[string]bool{}
	if requires != nil && requires.Value != nil && requires.Value.Kind == "list" {
		for _, e := range requires.Value.List {
			named[e.Str] = true
		}
	}
	at := d.Span
	if requires != nil {
		at = requires.Span
	}
	for _, r := range c.prog.AdapterEnv {
		if named[r.Name] || strings.HasPrefix(r.Name, "ALDER_") { // ALDER_… are the runtime's own
			continue
		}
		c.bag.Warn(diag.UnnamedEnv, at, "adapter `%s` reads `%s`, which `deploy.requires` does not name", r.Package, r.Name)
	}
}

var deployKnownField = map[string]bool{"floor": true, "checks": true, "requires": true, "backup": true, "keep": true, "proxy": true}

func (mc *modCtx) deployField(f *syntax.DeployField) {
	c := mc.c
	v := f.Value
	if v == nil {
		return
	}
	bad := func(format string, args ...any) { c.errorf(diag.DeployDecl, v.Span, format, args...) }
	switch f.Name {
	case "floor":
		x, err := strconv.ParseFloat(v.Str, 64)
		switch {
		case (v.Kind != "float" && v.Kind != "int") || err != nil:
			bad("`floor` is a number between 0 and 1, such as `0.95`")
		case math.IsNaN(x) || x < 0 || x > 1:
			bad("`floor` is %s, outside 0..1: it is a mutation score", v.Str)
		}
	case "keep":
		if n, err := strconv.Atoi(v.Str); v.Kind != "int" || err != nil || n < 1 {
			bad("`keep` is the number of builds to keep: a whole number of at least 1")
		}
	case "checks", "requires":
		if v.Kind != "list" {
			bad("`%s` is a list of text, such as `[\"…\"]`", f.Name)
			return
		}
		for _, e := range v.List {
			if e.Kind != "string" {
				c.errorf(diag.DeployDecl, e.Span, "`%s` is a list of text", f.Name)
				continue
			}
			switch {
			case looksSecret(e.Str): // reported once, by deploySecrets, without the value
			case strings.TrimSpace(e.Str) == "" && f.Name == "requires":
				c.errorf(diag.DeployDecl, e.Span, "a `requires` name is empty: name an environment variable such as \"SMTP_URL\"")
			case strings.TrimSpace(e.Str) == "":
				c.errorf(diag.DeployDecl, e.Span, "a `checks` command is empty")
			case f.Name == "requires" && !envName(e.Str):
				c.errorf(diag.DeployDecl, e.Span, "a `requires` entry must be an environment variable name (letters, digits, `_`)")
			}
		}
	case "backup":
		switch {
		case v.Kind != "ctor" || v.Ctor != "S3":
			bad("`backup` is `S3 \"s3://bucket/prefix\"`")
		case v.Arg == nil || !strings.HasPrefix(v.Arg.Str, "s3://") || len(v.Arg.Str) == len("s3://"):
			bad("`S3` takes the location as text, starting `s3://`")
		case v.Rec != nil:
			bad("`S3` takes only its location")
		}
	case "proxy":
		if v.Kind != "ctor" || (v.Ctor != "Caddy" && v.Ctor != "None") || v.Arg != nil || v.Rec != nil {
			bad("`proxy` is `Caddy` or `None`")
		}
	default:
		mc.deployTarget(f)
	}
	mc.deploySecrets(v)
}

func envName(s string) bool {
	for i, r := range s {
		switch {
		case r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return s != ""
}

// deployTarget checks the environment f: its value is `Ssh "host" { domain, path, user?, port? }`,
// `Fly "region" { app }` or `Script "path"`.
func (mc *modCtx) deployTarget(f *syntax.DeployField) {
	c := mc.c
	v := f.Value
	if v.Kind != "ctor" {
		c.errorf(diag.DeployDecl, v.Span, "`%s` is not a deploy setting: an environment's value is a target, `Ssh \"user@host\" { … }`, `Fly \"region\" { … }` or `Script \"./path\"`", f.Name)
		return
	}
	var arg, argWhat string
	var fields map[string]string // field -> "text" | "port"
	var required []string
	switch v.Ctor {
	case "Ssh":
		arg, argWhat, required = "host", "the host as text, such as \"deploy@box.example.com\"", []string{"domain", "path"}
		fields = map[string]string{"domain": "text", "path": "text", "user": "text", "port": "port"}
	case "Fly":
		arg, argWhat, required = "region", "the region as text, such as \"ord\"", []string{"app"}
		fields = map[string]string{"app": "text"}
	case "Script":
		arg, argWhat = "path", "the script's path as text, such as \"./scripts/deploy.sh\""
		fields = map[string]string{}
	default:
		c.errorf(diag.DeployDecl, v.Span, "unknown deploy target `%s` in environment `%s`: a target is `Ssh`, `Fly` or `Script`", v.Ctor, f.Name)
		return
	}
	if v.Arg == nil || strings.TrimSpace(v.Arg.Str) == "" {
		c.errorf(diag.DeployDecl, v.Span, "`%s` needs its %s: write it as %s", v.Ctor, arg, argWhat)
	}
	if v.Ctor == "Script" && v.Rec != nil {
		c.errorf(diag.DeployDecl, v.Span, "`Script` takes only its path; environment `%s` gives it fields", f.Name)
		return
	}
	have := map[string]bool{}
	for _, rf := range v.Rec {
		kind, ok := fields[rf.Name]
		if !ok {
			c.errorf(diag.DeployDecl, rf.NameSpan, "`%s` has no field `%s`", v.Ctor, rf.Name)
			continue
		}
		if have[rf.Name] {
			c.errorf(diag.DeployDecl, rf.NameSpan, "field `%s` is given twice in environment `%s`", rf.Name, f.Name)
			continue
		}
		have[rf.Name] = true
		if rf.Value == nil {
			continue
		}
		switch {
		case kind == "port":
			if n, err := strconv.Atoi(rf.Value.Str); rf.Value.Kind != "int" || err != nil || n < 1 || n > 65535 {
				c.errorf(diag.DeployDecl, rf.Value.Span, "`port` is a whole number from 1 to 65535")
			}
		case rf.Value.Kind != "string":
			c.errorf(diag.DeployDecl, rf.Value.Span, "`%s` is text", rf.Name)
		case strings.TrimSpace(rf.Value.Str) == "":
			c.errorf(diag.DeployDecl, rf.Value.Span, "`%s` is empty", rf.Name)
		}
	}
	for _, r := range required {
		if !have[r] {
			c.errorf(diag.DeployDecl, v.Span, "environment `%s` (%s) has no `%s`: write `%s \"…\" { %s: \"…\" }`", f.Name, v.Ctor, r, v.Ctor, r)
		}
	}
	if v.Ctor == "Ssh" && have["path"] {
		for _, rf := range v.Rec {
			if rf.Name == "path" && rf.Value != nil && rf.Value.Kind == "string" && !strings.HasPrefix(rf.Value.Str, "/") {
				c.errorf(diag.DeployDecl, rf.Value.Span, "`path` is an absolute directory on the host, such as \"/srv/blog\"")
			}
		}
	}
	if v.Ctor == "Ssh" {
		c.sshCharacters(v)
	}
}

// The character sets of an Ssh target's texts: they reach ssh's command line, shell scripts,
// a systemd unit and Caddy's configuration, so each is held to what it can be.
var (
	sshHostChars   = regexp.MustCompile(`^[A-Za-z0-9._@:\[\]-]+$`)
	sshUserChars   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	sshDomainChars = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9.-]+$`)
)

// sshCharacters reports an Ssh target's host, user, domain or path outside its character set
// (E0030): a host, user or domain beginning with `-` would read as an option.
func (c *Checker) sshCharacters(v *syntax.DeployValue) {
	if a := v.Arg; a != nil && strings.TrimSpace(a.Str) != "" && !looksSecret(a.Str) && (!sshHostChars.MatchString(a.Str) || strings.HasPrefix(a.Str, "-")) {
		c.errorf(diag.DeployDecl, a.Span, "the host of `Ssh` is a host name or `user@host`, such as \"deploy@box.example.com\": letters, digits, `.`, `-`, `_`, `@` (and `:`, `[`, `]` for an address), not beginning with `-`")
	}
	for _, rf := range v.Rec {
		if rf.Value == nil || rf.Value.Kind != "string" || strings.TrimSpace(rf.Value.Str) == "" || looksSecret(rf.Value.Str) {
			continue // a secret is reported as one (deploySecrets)
		}
		str := rf.Value.Str
		switch rf.Name {
		case "user":
			if !sshUserChars.MatchString(str) || strings.HasPrefix(str, "-") {
				c.errorf(diag.DeployDecl, rf.Value.Span, "`user` is a user name: letters, digits, `.`, `-`, `_`, not beginning with `-`")
			}
		case "domain":
			if !sshDomainChars.MatchString(str) || strings.HasPrefix(str, "-") {
				c.errorf(diag.DeployDecl, rf.Value.Span, "`domain` is a host name such as \"blog.example.com\" (or \"*.preview.example.com\"): letters, digits, `.` and `-`")
			}
		case "path":
			if strings.ContainsAny(str, "\"\\$`%") || strings.IndexFunc(str, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
				c.errorf(diag.DeployDecl, rf.Value.Span, "`path` may not contain a control character, `\"`, `\\`, `$`, `` ` `` or `%%`: it is written into shell scripts and a systemd unit")
			}
		}
	}
}

// deploySecrets reports every text of v that looks like a secret: the file carries none.
func (mc *modCtx) deploySecrets(v *syntax.DeployValue) {
	if v == nil {
		return
	}
	switch v.Kind {
	case "string":
		if looksSecret(v.Str) {
			mc.c.errorf(diag.DeployDecl, v.Span, "a secret belongs in the environment, not in `App`: name it in `requires`").NoSource = true
		}
	case "list":
		for _, e := range v.List {
			mc.deploySecrets(e)
		}
	case "ctor":
		mc.deploySecrets(v.Arg)
		for _, rf := range v.Rec {
			mc.deploySecrets(rf.Value)
		}
	}
}

// looksSecret: a URL with a password (`scheme://user:pass@host`), a text beginning like a known
// key (`sk_`, `ghp_`, `AKIA`, `xoxb-`, `xoxp-`, `-----BEGIN`), or 32 or more base64 or hex
// characters without a space. A path (starting `/` or `./`, or with a `/` and no `=` or `+`) is never taken for the last kind.
func looksSecret(s string) bool {
	if i := strings.Index(s, "://"); i >= 0 {
		auth := s[i+3:]
		if j := strings.IndexAny(auth, "/?#"); j >= 0 {
			auth = auth[:j]
		}
		if at := strings.LastIndex(auth, "@"); at >= 0 && strings.Contains(auth[:at], ":") {
			return true
		}
	}
	for _, p := range []string{"sk_", "ghp_", "AKIA", "xoxb-", "xoxp-", "-----BEGIN"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	if len(s) >= 32 && !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "./") && !(strings.Contains(s, "/") && !strings.ContainsAny(s, "=+")) {
		for _, r := range s {
			if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=') {
				return false
			}
		}
		return true
	}
	return false
}
