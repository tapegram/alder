package deploy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"alder/internal/ir"
)

// The Fly target's environment: the API token (`fly tokens create deploy`), which both flyctl and
// the Machines API calls read, and the organization a new Fly app is created in.
const (
	EnvFlyToken = "FLY_API_TOKEN"
	EnvFlyOrg   = "FLY_ORG"
)

// FlyAPI is the Machines API's address.
const FlyAPI = "https://api.machines.dev/v1"

// FlyPort is the port the app listens on inside its machine; Fly's proxy sends ports 80 and
// 443 of <app>.fly.dev there.
const FlyPort = 8080

// flyVolume is the name of the volume that holds the environment's state (mounted at /data).
const flyVolume = "alder_data"

// Fly is the target `Fly "region" { app }`: one Fly Machine running the build's image, with a
// volume mounted at /data holding the environment's state in the layout of the Ssh target:
//
//	/data/builds/<id>/   migrations/ (the build's own), migrate.out, backup.db, deployed.json
//	/data/current        the id of the build serving
//	/data/db/app.db      the database, and db/schema.json, db/migrations: the last migration's
//
// A build is an OCI image, registry.fly.io/<app>:<id>: the artifact's executable, static/ and
// migrations/ on a small Alpine base (the shape of the M10 Dockerfile: a base image, the
// binary copied in, a CMD; an app needs no Go), built and pushed by `flyctl deploy --build-only
// --push` (Fly's remote builder: no Docker is needed here). The machine, the volume and the
// app are made with the Machines API over HTTP; the `requires` values are set as Fly secrets
// with `flyctl secrets import`, read from THIS machine's environment at deploy time (never from
// a file); the app's public address is https://<app>.fly.dev.
//
// A volume attaches to one machine, and SQLite wants one writer, so a Fly environment is one
// machine and a new build REPLACES the old one in place (the image swapped; a few seconds
// without a listener, while Fly's proxy holds the requests): the image's start script applies
// the migrations (--migrate-only) and then serves. A build whose migrations refuse, or that
// fails its health check (GET https://<app>.fly.dev/ until the page's alder-build meta is the
// build id), is replaced by the previous image again. A rollback is the same swap to an older
// image. Images stay in Fly's registry; Prune removes only their directories under /data/builds.
type Fly struct {
	App, Env  string // the Alder app and environment
	Region    string
	FlyApp    string // the Fly app
	BackupURL string
	Warn      io.Writer

	API       string // the Machines API (FlyAPI)
	PublicURL string // https://<app>.fly.dev
	Token     string // FLY_API_TOKEN
	Org       string // the organization a new app is created in (FLY_ORG; "personal")
	Data      string // where the volume is mounted in the machine ("/data")
	HTTP      *http.Client
	// Flyctl runs flyctl with args and stdin (nil: none), returning its standard output; nil
	// runs the flyctl (or fly) on PATH.
	Flyctl func(ctx context.Context, stdin io.Reader, args ...string) (string, error)
	// Getenv reads this machine's environment: the `requires` values (os.Getenv when nil).
	Getenv func(string) string
	// Poll is how often the machine's state and the health check are asked (1s when 0).
	Poll time.Duration
	// MigrateTimeout bounds a new build's migrations at its start (10 minutes when 0).
	MigrateTimeout time.Duration

	allowLoss bool        // this run's --allow-data-loss (AllowDataLoss)
	exists    bool        // the Fly app exists
	machine   *flyMachine // the environment's machine (nil before the first deploy)
	requires  []string    // the names Requires was asked, set as secrets by Upload
	conn      bool
}

// NewFly is environment env's Fly target t of app, with the declaration's backup.
func NewFly(app, env string, t ir.Target, d *ir.Deploy) *Fly {
	f := &Fly{App: app, Env: env, Region: t.Region, FlyApp: t.App, Warn: io.Discard,
		API: FlyAPI, PublicURL: "https://" + t.App + ".fly.dev", Token: os.Getenv(EnvFlyToken), Org: os.Getenv(EnvFlyOrg), Data: "/data"}
	if d != nil && d.Backup != nil {
		f.BackupURL = d.Backup.URL
	}
	return f
}

// AllowDataLoss is told this run's --allow-data-loss (the engine's DataLossAllower): the
// machine's configuration then carries ALDER_ALLOW_DATA_LOSS=1, without which the runtime's
// --migrate-only refuses a migration that drops data. The next deploy without the flag writes the
// configuration without it.
func (f *Fly) AllowDataLoss(allow bool) { f.allowLoss = allow }

// Describe is "Fly ord (app blog-staging)".
func (f *Fly) Describe() string { return "Fly " + f.Region + " (app " + f.FlyApp + ")" }

// Identity is the Fly app, for the record's By.
func (f *Fly) Identity() string { return "fly:" + f.FlyApp }

// Close does nothing: every call is its own request.
func (f *Fly) Close() error { return nil }

func (f *Fly) getenv(name string) string {
	if f.Getenv != nil {
		return f.Getenv(name)
	}
	return os.Getenv(name)
}

func (f *Fly) poll() time.Duration {
	if f.Poll > 0 {
		return f.Poll
	}
	return time.Second
}

// image is build id's image.
func (f *Fly) image(id string) string { return "registry.fly.io/" + f.FlyApp + ":" + id }

// metaKey is the machine metadata naming the Alder app and environment it runs.
const metaKey = "alder"

// flyMachine is the part of a Machines API machine the target reads and writes.
type flyMachine struct {
	ID     string    `json:"id"`
	Name   string    `json:"name,omitempty"`
	State  string    `json:"state,omitempty"`
	Region string    `json:"region,omitempty"`
	Config flyConfig `json:"config"`
}

type flyConfig struct {
	Image    string            `json:"image"`
	Env      map[string]string `json:"env,omitempty"`
	Mounts   []flyMount        `json:"mounts,omitempty"`
	Services []flyService      `json:"services,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Restart  *flyRestart       `json:"restart,omitempty"`
	Guest    *flyGuest         `json:"guest,omitempty"`
}

type flyMount struct {
	Volume string `json:"volume"`
	Path   string `json:"path"`
}

type flyService struct {
	Protocol     string    `json:"protocol"`
	InternalPort int       `json:"internal_port"`
	Ports        []flyPort `json:"ports"`
}

type flyPort struct {
	Port       int      `json:"port"`
	Handlers   []string `json:"handlers"`
	ForceHTTPS bool     `json:"force_https,omitempty"`
}

type flyRestart struct {
	Policy     string `json:"policy"`
	MaxRetries int    `json:"max_retries,omitempty"`
}

type flyGuest struct {
	CPUKind  string `json:"cpu_kind"`
	CPUs     int    `json:"cpus"`
	MemoryMB int    `json:"memory_mb"`
}

type flyVol struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	State  string `json:"state,omitempty"`
}

// build is the build id the machine's config runs.
func (m *flyMachine) build() string { return m.Config.Metadata["alder_build"] }

// FlyAPIError is a Machines API request that failed.
type FlyAPIError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *FlyAPIError) Error() string {
	return fmt.Sprintf("the Fly Machines API refused %s %s (%d): %s", e.Method, e.Path, e.Status, orNone(strings.TrimSpace(lastLines(e.Body, 5))))
}

// call sends a Machines API request (body as JSON when not nil) and decodes the answer into out.
func (f *Fly) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(f.API, "/")+path, rd)
	if err != nil {
		return err
	}
	auth := f.Token
	if !strings.HasPrefix(auth, "FlyV1 ") {
		auth = "Bearer " + auth
	}
	req.Header.Set("Authorization", auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := f.HTTP
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach the Fly Machines API (%s): %v", f.API, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		return &FlyAPIError{Method: method, Path: path, Status: resp.StatusCode, Body: string(b)}
	}
	if out != nil && len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("the Fly Machines API's answer to %s %s: %v", method, path, err)
		}
	}
	return nil
}

func notFound(err error) bool {
	var e *FlyAPIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// flyctl runs flyctl (the Flyctl hook in tests).
func (f *Fly) flyctl(ctx context.Context, stdin io.Reader, args ...string) (string, error) {
	if f.Flyctl != nil {
		return f.Flyctl(ctx, stdin, args...)
	}
	bin, err := flyctlPath()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = stdin
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = append(os.Environ(), EnvFlyToken+"="+f.Token)
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("`flyctl %s` failed: %v\n%s", args[0]+" "+firstArg(args[1:]), err, lastLines(errb.String()+out.String(), 20))
	}
	return out.String(), nil
}

func firstArg(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

// flyctlPath finds flyctl, or fly, on PATH.
func flyctlPath() (string, error) {
	for _, n := range []string{"flyctl", "fly"} {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", refuse("", "the Fly target builds and pushes the build's image with `flyctl`, which is not on PATH: install it (https://fly.io/docs/flyctl/install/) and deploy again")
}

// Connect checks the token and flyctl, and finds the Fly app and the environment's machine.
// It changes nothing at Fly. A Fly machine runs linux/amd64.
func (f *Fly) Connect(ctx context.Context) (Platform, error) {
	plat := Platform{"linux", "amd64"}
	if f.conn {
		return plat, nil
	}
	if f.Token == "" {
		return Platform{}, refuse("", "the Fly target needs %s in this environment (`fly tokens create deploy -a %s`); set it and deploy again", EnvFlyToken, f.FlyApp)
	}
	if f.Flyctl == nil {
		if _, err := flyctlPath(); err != nil {
			return Platform{}, err
		}
	}
	if err := f.call(ctx, "GET", "/apps/"+f.FlyApp, nil, nil); err != nil {
		if !notFound(err) {
			return Platform{}, err
		}
	} else {
		f.exists = true
		if err := f.findMachine(ctx); err != nil {
			return Platform{}, err
		}
	}
	f.conn = true
	return plat, nil
}

// findMachine finds the machine this environment runs on (its metadata names the app and
// environment).
func (f *Fly) findMachine(ctx context.Context) error {
	var ms []flyMachine
	if err := f.call(ctx, "GET", "/apps/"+f.FlyApp+"/machines", nil, &ms); err != nil {
		return err
	}
	f.machine = nil
	for i, m := range ms {
		if m.Config.Metadata[metaKey] == f.App+"/"+f.Env && m.State != "destroyed" && m.State != "destroying" {
			f.machine = &ms[i]
			return nil
		}
	}
	return nil
}

// refresh reads the machine again.
func (f *Fly) refresh(ctx context.Context) error {
	if f.machine == nil {
		return nil
	}
	var m flyMachine
	if err := f.call(ctx, "GET", "/apps/"+f.FlyApp+"/machines/"+f.machine.ID, nil, &m); err != nil {
		return err
	}
	f.machine = &m
	return nil
}

// Requires reports the names neither this machine's environment sets (the value is set as a
// Fly secret at upload) nor the Fly app has as a secret already. No value is read from Fly.
func (f *Fly) Requires(ctx context.Context, names []string) ([]string, error) {
	f.requires = append([]string{}, names...)
	var set map[string]bool
	var missing []string
	for _, n := range names {
		if strings.TrimSpace(f.getenv(n)) != "" {
			continue
		}
		if set == nil {
			set = map[string]bool{}
			if f.exists {
				out, err := f.flyctl(ctx, nil, "secrets", "list", "--app", f.FlyApp, "--json")
				if err != nil {
					return nil, err
				}
				var secrets []struct{ Name string }
				if err := json.Unmarshal([]byte(out), &secrets); err != nil {
					return nil, fmt.Errorf("reading `flyctl secrets list --json`: %v", err)
				}
				for _, s := range secrets {
					set[s.Name] = true
				}
			}
		}
		if !set[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return nil, refuse("", "the Fly app %s has no secret %s, and this environment does not set %s: set %s here (deploy makes each `requires` value a Fly secret) or with `fly secrets set -a %s`, and deploy again; nothing was uploaded",
			f.FlyApp, strings.Join(missing, ", "), map[bool]string{true: "it", false: "them"}[len(missing) == 1], map[bool]string{true: "it", false: "them"}[len(missing) == 1], f.FlyApp)
	}
	return nil, nil
}

// execResult is the Machines API's answer to an exec.
type execResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// exec runs the shell script in the environment's machine (R is the volume's mount point) and
// returns its standard output; a non-zero exit is an error with its standard error.
func (f *Fly) exec(ctx context.Context, script string) (string, error) {
	if f.machine == nil {
		return "", errors.New("the environment has no machine yet")
	}
	var r execResult
	body := map[string]any{"command": []string{"sh", "-c", "R=" + q(f.Data) + "\n" + script}, "timeout": 60}
	if err := f.call(ctx, "POST", "/apps/"+f.FlyApp+"/machines/"+f.machine.ID+"/exec", body, &r); err != nil {
		return "", err
	}
	if r.ExitCode != 0 {
		return r.Stdout, fmt.Errorf("a command in machine %s failed (exit %d): %s", f.machine.ID, r.ExitCode, orNone(lastLines(strings.TrimSpace(r.Stderr), 20)))
	}
	return r.Stdout, nil
}

// running reports whether the machine is started, starting it when it is stopped (Fly may
// stop an idle machine; the state on its volume is read through it).
func (f *Fly) running(ctx context.Context) (bool, error) {
	if f.machine == nil {
		return false, nil
	}
	if err := f.refresh(ctx); err != nil {
		return false, err
	}
	if f.machine.State == "started" {
		return true, nil
	}
	if f.machine.State == "stopped" || f.machine.State == "suspended" {
		if err := f.call(ctx, "POST", "/apps/"+f.FlyApp+"/machines/"+f.machine.ID+"/start", nil, nil); err != nil {
			return false, err
		}
	}
	return f.waitState(ctx, "", 60*time.Second)
}

// waitState waits until the machine runs image (any when "") and is started (true), or has
// stopped running it (false).
func (f *Fly) waitState(ctx context.Context, image string, timeout time.Duration) (bool, error) {
	end := time.Now().Add(timeout)
	for {
		if err := f.refresh(ctx); err != nil {
			return false, err
		}
		if image == "" || f.machine.Config.Image == image {
			switch f.machine.State {
			case "started":
				return true, nil
			case "stopped", "failed":
				return false, nil
			}
		}
		if time.Now().After(end) {
			return false, fmt.Errorf("machine %s did not start within %s (state %s)", f.machine.ID, timeout, f.machine.State)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(f.poll()):
		}
	}
}

// read runs script in the machine when there is one and it runs; "" otherwise.
func (f *Fly) read(ctx context.Context, script string) (string, error) {
	ok, err := f.running(ctx)
	if err != nil || !ok {
		return "", err
	}
	return f.exec(ctx, script)
}

// Schema is /data/db/schema.json and /data/db/migrations, as the Ssh target keeps them.
func (f *Fly) Schema(ctx context.Context) (Schema, error) {
	out, err := f.read(ctx, `[ -f "$R/db/app.db" ] || { echo none; exit 0; }
c=$(cat "$R/current" 2>/dev/null)
echo db
if [ -f "$R/db/migrations" ]; then cat "$R/db/migrations"; else ls "$R/builds/$c/migrations" 2>/dev/null | sed -n 's/\.sql$//p'; fi
echo @@schema
if [ -f "$R/db/schema.json" ]; then cat "$R/db/schema.json"
elif [ -f "$R/builds/$c/migrations/schema.json" ]; then cat "$R/builds/$c/migrations/schema.json"
else echo '{"tables": []}'; fi
`)
	if err != nil {
		return Schema{}, err
	}
	return parseSchema(out, "db"), nil
}

// BuildSchema is /data/builds/<id>/migrations/, which the build's start script copied there.
func (f *Fly) BuildSchema(ctx context.Context, id string) (Schema, error) {
	out, err := f.read(ctx, `d="$R/builds/"`+q(id)+`/migrations; [ -f "$d/schema.json" ] || { echo none; exit 0; }
echo yes
ls "$d" | sed -n 's/\.sql$//p'
echo @@schema
cat "$d/schema.json"
`)
	if err != nil {
		return Schema{}, err
	}
	return parseSchema(out, "yes"), nil
}

// Current is /data/current.
func (f *Fly) Current(ctx context.Context) (string, error) {
	out, err := f.read(ctx, `cat "$R/current" 2>/dev/null; exit 0`)
	return strings.TrimSpace(out), err
}

// Builds lists /data/builds/, newest first; the running one is the image the machine runs.
func (f *Fly) Builds(ctx context.Context) ([]Build, error) {
	out, err := f.read(ctx, `cd "$R/builds" 2>/dev/null || exit 0
cur=$(cat "$R/current" 2>/dev/null)
echo "@@current $cur"
for d in */; do
  d=${d%/}; [ -d "$d" ] || continue
  echo "@@build $d $(date -r "$d" +%s 2>/dev/null || stat -c %Y "$d" 2>/dev/null || echo 0)"
  cat "$d/deployed.json" 2>/dev/null
done
exit 0
`)
	if err != nil || out == "" {
		return nil, err
	}
	cur := ""
	if first, _, ok := strings.Cut(out, "\n"); ok {
		cur = strings.TrimSpace(strings.TrimPrefix(first, "@@current"))
	}
	running := ""
	if f.machine != nil && f.machine.State == "started" {
		running = f.machine.build()
	}
	var builds []Build
	for _, chunk := range strings.Split(out, "@@build ")[1:] {
		head, body, _ := strings.Cut(chunk, "\n")
		fs := strings.Fields(head)
		if len(fs) != 2 || !idRe.MatchString(fs[0]) {
			continue
		}
		secs, _ := strconv.ParseInt(fs[1], 10, 64)
		b := Build{ID: fs[0], Modified: time.Unix(secs, 0), Running: fs[0] == running, Current: fs[0] == cur}
		if strings.TrimSpace(body) != "" {
			var r Record
			if json.Unmarshal([]byte(body), &r) == nil {
				b.Record = &r
			}
		}
		builds = append(builds, b)
	}
	sort.SliceStable(builds, func(i, j int) bool { return newer(builds[i], builds[j]) })
	return builds, nil
}

// Upload makes the Fly app when it does not exist (with its public addresses), builds and
// pushes build id's image (`flyctl deploy --build-only --push`, on Fly's remote builder), and
// sets the `requires` values this machine's environment holds as Fly secrets (staged: the
// machine reads them when it next starts, which the deploy does next).
func (f *Fly) Upload(ctx context.Context, dir, id string) error {
	if !f.exists {
		org := f.Org
		if org == "" {
			org = "personal"
		}
		if err := f.call(ctx, "POST", "/apps", map[string]string{"app_name": f.FlyApp, "org_slug": org}, nil); err != nil {
			return fmt.Errorf("creating the Fly app %s in organization %s (%s names another): %w", f.FlyApp, org, EnvFlyOrg, err)
		}
		f.exists = true
	}
	out, err := f.flyctl(ctx, nil, "ips", "list", "--app", f.FlyApp, "--json")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" || strings.TrimSpace(out) == "null" {
		if _, err := f.flyctl(ctx, nil, "ips", "allocate-v4", "--shared", "--app", f.FlyApp); err != nil {
			return err
		}
		if _, err := f.flyctl(ctx, nil, "ips", "allocate-v6", "--app", f.FlyApp); err != nil {
			return err
		}
	}
	stage, err := os.MkdirTemp("", "alder-fly-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := f.writeContext(stage, dir, id); err != nil {
		return err
	}
	if _, err := f.flyctl(ctx, nil, "deploy", stage, "--config", filepath.Join(stage, "fly.toml"), "--app", f.FlyApp,
		"--build-only", "--push", "--remote-only", "--image-label", id); err != nil {
		return err
	}
	var lines strings.Builder
	for _, n := range f.requires {
		if v := f.getenv(n); strings.TrimSpace(v) != "" {
			if strings.ContainsAny(v, "\n\r") {
				return refuse("", "the value of %s holds a line break, which `flyctl secrets import` cannot take: set it with `fly secrets set` yourself and unset it here", n)
			}
			lines.WriteString(n + "=" + v + "\n")
		}
	}
	if lines.Len() > 0 {
		if _, err := f.flyctl(ctx, strings.NewReader(lines.String()), "secrets", "import", "--app", f.FlyApp, "--stage"); err != nil {
			return errors.New("setting the `requires` values as Fly secrets failed (`flyctl secrets import`); no value is shown")
		}
	}
	return nil
}

// writeContext writes the image's build context into stage: the artifact dir, the Dockerfile,
// the start script and a fly.toml naming the app.
func (f *Fly) writeContext(stage, dir, id string) error {
	if err := copyTree(dir, stage); err != nil {
		return err
	}
	for _, d := range []string{"static", "migrations"} {
		if err := os.MkdirAll(filepath.Join(stage, d), 0o755); err != nil {
			return err
		}
	}
	files := map[string]string{
		"Dockerfile":    FlyDockerfile(f.App, id),
		"run":           FlyRunScript(f.App, f.Env),
		"fly.toml":      "# Written by alder deploy for the image build only; the machine is made with the Machines API.\napp = " + strconv.Quote(f.FlyApp) + "\nprimary_region = " + strconv.Quote(f.Region) + "\n\n[build]\n  dockerfile = \"Dockerfile\"\n",
		".dockerignore": "fly.toml\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(stage, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// FlyDockerfile is the Dockerfile of build id's image: the artifact on Alpine (a shell for the
// start script and the deploy's commands; CA certificates for the adapters' TLS).
func FlyDockerfile(app, id string) string {
	return `# Written by alder deploy (book chapter 16): the image of build ` + id + ` of ` + app + `.
# The executable is static (CGO off); Alpine adds a shell for the start script and the CA
# certificates the adapters' TLS needs.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY app /app/app
COPY static /app/static
COPY migrations /app/migrations
COPY alder-deploy.json /app/alder-deploy.json
COPY run /app/run
RUN chmod 755 /app/app /app/run
ENV ALDER_BUILD=` + id + `
CMD ["/app/run"]
`
}

// FlyRunScript is the image's start script, /app/run: it applies a restore that `alder restore`
// left, copies the build's migrations to its directory on the volume, runs --migrate-only (a
// refusal stops the machine with exit 0, so Fly does not restart it, and the deploy puts the
// previous image back), records the migrated schema as db/schema.json, and serves. It holds no
// value: the secrets are the machine's environment.
func FlyRunScript(app, env string) string {
	return `#!/bin/sh
# Written by alder deploy (book chapter 16): starts the build of ` + app + ` (environment ` + env + `)
# whose id is $ALDER_BUILD on its Fly machine, the volume mounted at $ALDER_DATA (/data).
R="${ALDER_DATA:-/data}"
A="${ALDER_APP_DIR:-/app}"
B="$R/builds/$ALDER_BUILD"
mkdir -p "$B" "$R/db"
if [ -f "$R/db/app.db.restore" ]; then
  ts=$(date -u +%Y%m%dT%H%M%SZ)
  for x in "" -wal -shm; do [ -f "$R/db/app.db$x" ] && mv "$R/db/app.db$x" "$R/db/app.db.before-restore-$ts$x"; done
  mv "$R/db/app.db.restore" "$R/db/app.db"
  rm -f "$R/db/schema.json" "$R/db/migrations"
  echo "alder: the database restored by alder restore is in place (the previous one: app.db.before-restore-$ts)"
fi
rm -rf "$B/migrations" && cp -R "$A/migrations" "$B/migrations"
rm -f "$B/migrate.status"
if "$A/app" --migrate-only > "$B/migrate.out" 2>&1; then
  if [ -f "$A/migrations/schema.json" ]; then
    cp "$A/migrations/schema.json" "$R/db/schema.json.new" && mv "$R/db/schema.json.new" "$R/db/schema.json"
    ls "$A/migrations" | sed -n 's/\.sql$//p' > "$R/db/migrations.new" && mv "$R/db/migrations.new" "$R/db/migrations"
  fi
  cat "$B/migrate.out"
  echo ok > "$B/migrate.status"
else
  cat "$B/migrate.out"
  echo failed > "$B/migrate.status"
  exit 0
fi
exec "$A/app"
`
}

// config is the machine's configuration running build id: its image, the volume at /data, the
// environment (no secret: Fly adds the app's secrets), ports 80 and 443 of the app's address.
func (f *Fly) config(id, volume string) flyConfig {
	env := map[string]string{
		"ALDER_DB":     f.Data + "/db/app.db",
		"ALDER_ADDR":   ":" + strconv.Itoa(FlyPort),
		"ALDER_STATIC": "/app/static",
		"ALDER_DATA":   f.Data,
	}
	if f.BackupURL != "" {
		env["ALDER_BACKUP_URL"] = f.BackupURL
	}
	if f.allowLoss {
		env["ALDER_ALLOW_DATA_LOSS"] = "1"
	}
	return flyConfig{
		Image:  f.image(id),
		Env:    env,
		Mounts: []flyMount{{Volume: volume, Path: f.Data}},
		Services: []flyService{{Protocol: "tcp", InternalPort: FlyPort, Ports: []flyPort{
			{Port: 80, Handlers: []string{"http"}, ForceHTTPS: true},
			{Port: 443, Handlers: []string{"tls", "http"}},
		}}},
		Metadata: map[string]string{metaKey: f.App + "/" + f.Env, "alder_build": id},
		Restart:  &flyRestart{Policy: "on-failure", MaxRetries: 5},
		Guest:    &flyGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 256},
	}
}

// volume is the environment's volume, created in the region when there is none.
func (f *Fly) volume(ctx context.Context) (string, error) {
	var vols []flyVol
	if err := f.call(ctx, "GET", "/apps/"+f.FlyApp+"/volumes", nil, &vols); err != nil {
		return "", err
	}
	for _, v := range vols {
		if v.Name == flyVolume && v.Region == f.Region && v.State != "destroyed" && v.State != "pending_destroy" {
			return v.ID, nil
		}
	}
	var v flyVol
	if err := f.call(ctx, "POST", "/apps/"+f.FlyApp+"/volumes", map[string]any{"name": flyVolume, "region": f.Region, "size_gb": 1, "encrypted": true}, &v); err != nil {
		return "", err
	}
	return v.ID, nil
}

// Backup copies the database into /data/builds/<id>/backup.db with the serving build's binary
// (VACUUM INTO), before the new image starts.
func (f *Fly) Backup(ctx context.Context, id string) (string, error) {
	out, err := f.read(ctx, `id=`+q(id)+`
rm -f "$R/builds/$id/migrate.status"
[ -f "$R/db/app.db" ] || exit 0
mkdir -p "$R/builds/$id"
ALDER_DB="$R/db/app.db" "${ALDER_APP_DIR:-/app}/app" --backup-to "$R/builds/$id/backup.db" >&2 && echo "backup=builds/$id/backup.db"
`)
	if err != nil {
		return "", err
	}
	return keyValues(out)["backup"], nil
}

// Migrate puts build id's image on the machine (made, with its volume, on the first deploy):
// its start script applies the migrations before it serves. When they refuse, the previous
// image is put back, and the error carries the refusal.
func (f *Fly) Migrate(ctx context.Context, id string) (string, error) {
	prev := ""
	if f.machine != nil {
		prev = f.machine.build()
	}
	if err := f.put(ctx, id); err != nil {
		return "", err
	}
	timeout := f.MigrateTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	end := time.Now().Add(timeout)
	for {
		started, err := f.waitState(ctx, f.image(id), time.Until(end))
		if err != nil {
			return "", err
		}
		if !started {
			break // the start script stopped: the migrations refused
		}
		st, err := f.exec(ctx, `cat "$R/builds/"`+q(id)+`/migrate.status 2>/dev/null; exit 0`)
		if err == nil && strings.TrimSpace(st) == "ok" {
			return f.exec(ctx, `cat "$R/builds/"`+q(id)+`/migrate.out 2>/dev/null; exit 0`)
		}
		if err == nil && strings.TrimSpace(st) == "failed" {
			break
		}
		if time.Now().After(end) {
			return "", fmt.Errorf("build %s's migrations did not finish within %s", id, timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(f.poll()):
		}
	}
	logs := f.recentLogs(ctx)
	if prev != "" && prev != id {
		if err := f.revert(ctx, prev); err != nil {
			return logs, fmt.Errorf("%s\nand build %s did not start again: %v", logs, prev, err)
		}
		if out, err := f.exec(ctx, `cat "$R/builds/"`+q(id)+`/migrate.out 2>/dev/null; exit 0`); err == nil && strings.TrimSpace(out) != "" {
			logs = out
		}
	}
	return logs, errors.New("--migrate-only refused")
}

// put makes the machine run build id's image: an update (Fly restarts it) or, on the first
// deploy, a new machine with the environment's volume.
func (f *Fly) put(ctx context.Context, id string) error {
	if f.machine != nil {
		vol := ""
		for _, m := range f.machine.Config.Mounts {
			if m.Path == f.Data {
				vol = m.Volume
			}
		}
		if vol == "" {
			var err error
			if vol, err = f.volume(ctx); err != nil {
				return err
			}
		}
		var m flyMachine
		if err := f.call(ctx, "POST", "/apps/"+f.FlyApp+"/machines/"+f.machine.ID, map[string]any{"config": f.config(id, vol)}, &m); err != nil {
			return err
		}
		f.machine = &m
		return nil
	}
	vol, err := f.volume(ctx)
	if err != nil {
		return err
	}
	var m flyMachine
	body := map[string]any{"name": unitName(f.App + "-" + f.Env), "region": f.Region, "config": f.config(id, vol)}
	if err := f.call(ctx, "POST", "/apps/"+f.FlyApp+"/machines", body, &m); err != nil {
		return err
	}
	f.machine = &m
	return nil
}

// revert puts build prev's image back and waits for it to start.
func (f *Fly) revert(ctx context.Context, prev string) error {
	if err := f.put(ctx, prev); err != nil {
		return err
	}
	ok, err := f.waitState(ctx, f.image(prev), 2*time.Minute)
	if err == nil && !ok {
		err = fmt.Errorf("its machine stopped; see `alder logs %s`", f.Env)
	}
	return err
}

// Start makes the machine run build id (Migrate already did when deploying; a rollback starts
// here) and returns the port the app listens on inside it.
func (f *Fly) Start(ctx context.Context, id string) (int, error) {
	if f.machine == nil || f.machine.build() != id {
		if err := f.put(ctx, id); err != nil {
			return 0, err
		}
	}
	ok, err := f.waitState(ctx, f.image(id), 2*time.Minute)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("build %s exited at start; its log (`alder logs %s`):\n%s", id, f.Env, f.recentLogs(ctx))
	}
	return FlyPort, nil
}

// Health waits until GET <public URL>/ is a page whose alder-build meta names id.
func (f *Fly) Health(ctx context.Context, id string, port int, timeout time.Duration) error {
	client := f.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	want := []byte(`name="alder-build" content="` + id + `"`)
	end := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(f.PublicURL, "/")+"/", nil)
		if err != nil {
			return err
		}
		if resp, err := client.Do(req); err == nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			if bytes.Contains(b, want) {
				return nil
			}
		}
		if time.Now().After(end) {
			return fmt.Errorf("build %s did not answer GET %s/ with its build id within %s; its log (`alder logs %s`):\n%s", id, strings.TrimRight(f.PublicURL, "/"), timeout, f.Env, f.recentLogs(ctx))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.poll()):
		}
	}
}

// recentLogs is the app's last log lines (`flyctl logs --no-tail`), or a sentence when they
// cannot be read.
func (f *Fly) recentLogs(ctx context.Context) string {
	out, err := f.flyctl(ctx, nil, "logs", "--app", f.FlyApp, "--no-tail")
	if err != nil || strings.TrimSpace(out) == "" {
		return "(the log could not be read: `alder logs " + f.Env + "`)"
	}
	return lastLines(out, 40)
}

// Switch makes id current (/data/current): the machine already serves it.
func (f *Fly) Switch(ctx context.Context, id string, port int) (int, error) {
	_, err := f.exec(ctx, `echo `+q(id)+` > "$R/current.new" && mv "$R/current.new" "$R/current"`)
	return port, err
}

// Stop: a build is stopped by putting the current build's image back on the machine (a build
// that failed its health check), or by stopping the machine when no build is current. The
// previous build after a switch already stopped when its image was replaced: nothing to do.
func (f *Fly) Stop(ctx context.Context, id string, drain time.Duration) error {
	if f.machine == nil {
		return nil
	}
	if err := f.refresh(ctx); err != nil {
		return err
	}
	if f.machine.build() != id {
		return nil
	}
	cur, _ := f.exec(ctx, `cat "$R/current" 2>/dev/null; exit 0`)
	if cur = strings.TrimSpace(cur); cur != "" && cur != id {
		return f.revert(ctx, cur)
	}
	return f.call(ctx, "POST", "/apps/"+f.FlyApp+"/machines/"+f.machine.ID+"/stop", nil, nil)
}

// Record writes /data/builds/<id>/deployed.json.
func (f *Fly) Record(ctx context.Context, id string, rec []byte) error {
	_, err := f.exec(ctx, `mkdir -p "$R/builds/"`+q(id)+` && printf %s `+q(base64.StdEncoding.EncodeToString(rec))+` | base64 -d > "$R/builds/"`+q(id)+`/deployed.json`)
	return err
}

// Prune removes the directories of the builds beyond the keep newest (never one in protect, nor
// the current or running one). Their images stay in Fly's registry.
func (f *Fly) Prune(ctx context.Context, keep int, protect []string) ([]string, error) {
	builds, err := f.Builds(ctx)
	if err != nil {
		return nil, err
	}
	var gone []string
	var sc strings.Builder
	for i, b := range builds {
		if i < keep || b.Running || b.Current || contains(protect, b.ID) {
			continue
		}
		gone = append(gone, b.ID)
		sc.WriteString(`rm -rf "$R/builds/"` + q(b.ID) + "\n")
	}
	if len(gone) == 0 {
		return nil, nil
	}
	_, err = f.exec(ctx, sc.String())
	return gone, err
}

// Info is the database's size and its last backup, as the Ssh target reads them.
func (f *Fly) Info(ctx context.Context) (Info, error) {
	out, err := f.read(ctx, `if [ -f "$R/db/app.db" ]; then
  n=$(wc -c < "$R/db/app.db" | tr -d ' '); w=0; [ -f "$R/db/app.db-wal" ] && w=$(wc -c < "$R/db/app.db-wal" | tr -d ' ')
  echo "db=$((n + w))"
fi
[ -f "$R/db/app.db-backup.json" ] && echo "shipped=$(tr -d '\n' < "$R/db/app.db-backup.json")"
for f in "$R"/builds/*/backup.db; do [ -f "$f" ] && echo "copy=$(date -r "$f" +%s 2>/dev/null || stat -c %Y "$f") builds/$(basename "$(dirname "$f")")/backup.db"; done
exit 0
`)
	if err != nil {
		return Info{}, err
	}
	var in Info
	for _, l := range strings.Split(out, "\n") {
		k, v, _ := strings.Cut(l, "=")
		switch k {
		case "db":
			in.HaveDB = true
			in.DBBytes, _ = strconv.ParseInt(v, 10, 64)
		case "shipped":
			var st struct {
				Last time.Time `json:"last"`
				URL  string    `json:"url"`
			}
			if json.Unmarshal([]byte(v), &st) == nil && st.Last.After(in.LastBackup) {
				in.LastBackup, in.BackupFrom = st.Last, "streamed to "+st.URL
			}
		case "copy":
			secs, path, _ := strings.Cut(v, " ")
			n, _ := strconv.ParseInt(secs, 10, 64)
			if t := time.Unix(n, 0); t.After(in.LastBackup) {
				in.LastBackup, in.BackupFrom = t, path
			}
		}
	}
	return in, nil
}

// Logs is `flyctl logs` of the app: the recent lines, or followed until ctx ends.
func (f *Fly) Logs(ctx context.Context, follow bool, w io.Writer) error {
	if _, err := f.Connect(ctx); err != nil {
		return err
	}
	if f.Flyctl != nil {
		args := []string{"logs", "--app", f.FlyApp}
		if !follow {
			args = append(args, "--no-tail")
		}
		out, err := f.Flyctl(ctx, nil, args...)
		io.WriteString(w, out)
		return err
	}
	bin, err := flyctlPath()
	if err != nil {
		return err
	}
	args := []string{"logs", "--app", f.FlyApp}
	if !follow {
		args = append(args, "--no-tail")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = w, w
	cmd.Env = append(os.Environ(), EnvFlyToken+"="+f.Token)
	return cmd.Run()
}

// restoreChunk is how many bytes of the database one exec carries (base64 in an argument).
const restoreChunk = 48 << 10

// Restore sends the local database file db to the volume as db/app.db.restore (in base64
// pieces through exec) and restarts the machine: the start script moves the old database aside
// (app.db.before-restore-<time>) and puts the restored one in place before serving; the build
// is then health checked. The kept name's time is the machine's restart's.
func (f *Fly) Restore(ctx context.Context, db string) (string, error) {
	return "db/app.db.before-restore-<time>", f.restore(ctx, db)
}

func (f *Fly) restore(ctx context.Context, db string) error {
	if _, err := f.Connect(ctx); err != nil {
		return err
	}
	ok, err := f.running(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return refuse("", "the Fly app %s has no running machine to restore into: deploy first", f.FlyApp)
	}
	b, err := os.ReadFile(db)
	if err != nil {
		return err
	}
	if _, err := f.exec(ctx, `mkdir -p "$R/db" && : > "$R/db/app.db.part"`); err != nil {
		return err
	}
	for off := 0; off < len(b); off += restoreChunk {
		end := min(off+restoreChunk, len(b))
		if _, err := f.exec(ctx, `printf %s `+q(base64.StdEncoding.EncodeToString(b[off:end]))+` | base64 -d >> "$R/db/app.db.part"`); err != nil {
			return err
		}
	}
	out, err := f.exec(ctx, `wc -c < "$R/db/app.db.part" | tr -d ' '`)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != strconv.Itoa(len(b)) {
		return fmt.Errorf("the database arrived with %s bytes of %d: nothing was replaced", strings.TrimSpace(out), len(b))
	}
	if _, err := f.exec(ctx, `chmod 600 "$R/db/app.db.part" && mv "$R/db/app.db.part" "$R/db/app.db.restore"`); err != nil {
		return err
	}
	id := f.machine.build()
	if err := f.call(ctx, "POST", "/apps/"+f.FlyApp+"/machines/"+f.machine.ID+"/restart", nil, nil); err != nil {
		return err
	}
	end := time.Now().Add(2 * time.Minute)
	for { // the start script moves app.db.restore into place before it serves
		ok, err := f.waitState(ctx, f.image(id), time.Until(end))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("build %s stopped after the restore; its log (`alder logs %s`):\n%s", id, f.Env, f.recentLogs(ctx))
		}
		if out, err := f.exec(ctx, `[ -f "$R/db/app.db.restore" ] && echo pending; exit 0`); err == nil && strings.TrimSpace(out) == "" {
			break
		}
		if time.Now().After(end) {
			return fmt.Errorf("build %s did not take the restored database within 2m", id)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.poll()):
		}
	}
	return f.Health(ctx, id, FlyPort, DefaultHealthTimeout)
}
