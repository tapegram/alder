package deploy

import (
	"strings"
	"testing"
)

func initFiles(t *testing.T, s InitSpec) map[string]string {
	t.Helper()
	fs, err := InitFiles(s)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, f := range fs {
		m[f.Name] = f.Body
	}
	return m
}

func TestInitFiles_Render(t *testing.T) {
	m := initFiles(t, InitSpec{Target: "render", App: "blog", Ref: "4141f55053f0efb06299ada67d31a3a6bf60f58f", Disk: true})
	if len(m) != 3 || m["Dockerfile"] == "" || m[".dockerignore"] == "" || m["render.yaml"] == "" {
		t.Fatalf("files: %v", keys(m))
	}
	for _, want := range []string{"ARG ALDER_REF=4141f55053f0efb06299ada67d31a3a6bf60f58f", "on Render's Docker runtime", "RUN alder build", "/blog/build/blog/blog", "gcr.io/distroless/static-debian12", `ENTRYPOINT ["/app/blog"]`} {
		if !strings.Contains(m["Dockerfile"], want) {
			t.Errorf("Dockerfile lacks %q:\n%s", want, m["Dockerfile"])
		}
	}
	for _, want := range []string{"runtime: docker", "name: blog", "healthCheckPath: /_alder/health", "disk:", "mountPath: /data", "/data/app.db", "generateValue: true"} {
		if !strings.Contains(m["render.yaml"], want) {
			t.Errorf("render.yaml lacks %q:\n%s", want, m["render.yaml"])
		}
	}
}

func TestInitFiles_NoDisk(t *testing.T) {
	m := initFiles(t, InitSpec{Target: "render", App: "blog", Ref: "v0.1.0"})
	if strings.Contains(m["render.yaml"], "disk:") || strings.Contains(m["render.yaml"], "ALDER_DB") {
		t.Errorf("render.yaml has a disk for an app without tables:\n%s", m["render.yaml"])
	}
	f := initFiles(t, InitSpec{Target: "fly", App: "blog", Ref: "v0.1.0"})
	if strings.Contains(f["fly.toml"], "[mounts]") {
		t.Errorf("fly.toml has a volume:\n%s", f["fly.toml"])
	}
}

func TestInitFiles_Fly(t *testing.T) {
	m := initFiles(t, InitSpec{Target: "fly", App: "blog", Ref: "v0.1.0", Disk: true})
	if len(m) != 3 || m["fly.toml"] == "" {
		t.Fatalf("files: %v", keys(m))
	}
	for _, want := range []string{`app = "blog"`, "internal_port = 8080", "[mounts]", `destination = "/data"`, "/_alder/health", `ALDER_DB = "/data/app.db"`} {
		if !strings.Contains(m["fly.toml"], want) {
			t.Errorf("fly.toml lacks %q:\n%s", want, m["fly.toml"])
		}
	}
	if m[".dockerignore"] == "" || !strings.Contains(m["Dockerfile"], "ARG ALDER_REF=v0.1.0") {
		t.Errorf("dockerfile/ignore: %q", m["Dockerfile"])
	}
}

// render.yaml and fly.toml carry ALDER_PUBLIC_URL, the platform's address of the app: a
// Blueprint or `fly deploy` app would otherwise derive http://localhost:<port>.
func TestInitFiles_PublicURL(t *testing.T) {
	r := initFiles(t, InitSpec{Target: "render", App: "blog", Ref: "v0.1.0"})
	if want := "      - key: ALDER_PUBLIC_URL\n        value: https://blog.onrender.com # change if Render gave the service another name\n"; !strings.Contains(r["render.yaml"], want) {
		t.Errorf("render.yaml lacks %q:\n%s", want, r["render.yaml"])
	}
	f := initFiles(t, InitSpec{Target: "fly", App: "blog", Ref: "v0.1.0"})
	if want := "[env]\n  ALDER_ADDR = \":8080\"\n  ALDER_PUBLIC_URL = \"https://blog.fly.dev\"\n"; !strings.Contains(f["fly.toml"], want) {
		t.Errorf("fly.toml lacks %q:\n%s", want, f["fly.toml"])
	}
	if strings.Contains(FlyToml("blog", "ord", false, false), "ALDER_PUBLIC_URL") {
		t.Error("the image build's stub has an [env]")
	}
}

func TestInitFiles_Placeholder(t *testing.T) {
	m := initFiles(t, InitSpec{Target: "render", App: "blog"})
	if !strings.Contains(m["Dockerfile"], "ARG ALDER_REF="+InitRefPlaceholder) || !strings.Contains(m["Dockerfile"], "REPLACE") {
		t.Errorf("no marked placeholder:\n%s", m["Dockerfile"])
	}
}

func TestInitFiles_Unknown(t *testing.T) {
	if _, err := InitFiles(InitSpec{Target: "heroku", App: "x"}); err == nil {
		t.Fatal("unknown target accepted")
	}
}

// The image build's fly.toml stub and the one `deploy init fly` writes come from one template.
func TestFlyToml_Shared(t *testing.T) {
	stub := FlyToml("blog-staging", "ord", false, false)
	if !strings.Contains(stub, `app = "blog-staging"`) || !strings.Contains(stub, `primary_region = "ord"`) || strings.Contains(stub, "[http_service]") {
		t.Errorf("stub:\n%s", stub)
	}
	full := FlyToml("blog", "", true, true)
	if !strings.Contains(full, "[http_service]") || strings.Contains(full, "primary_region") {
		t.Errorf("full:\n%s", full)
	}
}

func keys(m map[string]string) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	return k
}
