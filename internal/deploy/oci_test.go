package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"alder/internal/registryfake"
)

const ociArtifact = "testdata/oci/artifact"

func hexSum(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }

func TestImage_Deterministic(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(ociArtifact)); err != nil {
		t.Fatal(err)
	}
	a, err := BuildImage(dir, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(48 * time.Hour)
	filepath.Walk(dir, func(p string, _ os.FileInfo, _ error) error { return os.Chtimes(p, later, later) })
	b, err := BuildImage(dir, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest || a.LayerDigest != b.LayerDigest {
		t.Errorf("digests differ: %s vs %s", a.Digest, b.Digest)
	}
	c, _ := BuildImage(dir, "other")
	if c.Digest == a.Digest || c.LayerDigest != a.LayerDigest {
		t.Errorf("the build id should change the config and manifest only")
	}
}

func TestImage_Golden(t *testing.T) {
	img, err := BuildImage(ociArtifact, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string][]byte{"manifest.golden.json": img.Manifest, "config.golden.json": img.Config} {
		path := filepath.Join("testdata/oci", name)
		if os.Getenv("UPDATE_GOLDEN") != "" {
			os.WriteFile(path, append(got, '\n'), 0o644)
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(bytes.TrimSpace(want)) != string(got) {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
}

func TestImage_Layer(t *testing.T) {
	img, err := BuildImage(ociArtifact, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(img.Layer))
	if err != nil {
		t.Fatal(err)
	}
	if zr.Header.OS != 255 || !zr.Header.ModTime.IsZero() || zr.Header.Name != "" {
		t.Errorf("gzip header %+v", zr.Header)
	}
	got := map[string]int64{}
	var names []string
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		got[h.Name] = h.Mode
		if !h.ModTime.Equal(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) || h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" {
			t.Errorf("%s: header %+v", h.Name, h)
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("entries not sorted: %v", names)
	}
	want := map[string]int64{
		"app/": 0o755, "app/app": 0o755, "app/alder-deploy.json": 0o644,
		"app/static/": 0o755, "app/static/css/": 0o755, "app/static/css/app.css": 0o644, "app/static/index.html": 0o644,
		"app/migrations/": 0o755, "app/migrations/001_init.sql": 0o644, "app/migrations/schema.json": 0o644,
		"etc/": 0o755, "etc/ssl/": 0o755, "etc/ssl/certs/": 0o755, "etc/ssl/certs/ca-certificates.crt": 0o644,
	}
	if len(got) != len(want) {
		t.Errorf("entries %v\nwant %v", got, want)
	}
	for n, m := range want {
		if g, ok := got[n]; !ok || g != m {
			t.Errorf("%s: mode %o (present %v), want %o", n, g, ok, m)
		}
	}
}

func TestImage_DigestsMatch(t *testing.T) {
	img, err := BuildImage(ociArtifact, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if img.LayerDigest != hexSum(img.Layer) {
		t.Error("LayerDigest")
	}
	zr, _ := gzip.NewReader(bytes.NewReader(img.Layer))
	raw, _ := io.ReadAll(zr)
	if img.DiffID != hexSum(raw) || img.ConfigDigest != hexSum(img.Config) || img.Digest != hexSum(img.Manifest) {
		t.Error("DiffID, ConfigDigest or Digest")
	}
	if !strings.Contains(string(img.Config), img.DiffID) || !strings.Contains(string(img.Manifest), img.LayerDigest) {
		t.Error("config or manifest does not name the digests")
	}
}

func TestParseImage(t *testing.T) {
	for in, want := range map[string][2]string{
		"ghcr.io/tavish/blog":      {"ghcr.io", "tavish/blog"},
		"docker.io/u/app":          {"registry-1.docker.io", "u/app"},
		"docker.io/app":            {"registry-1.docker.io", "library/app"},
		"localhost:5000/team/blog": {"localhost:5000", "team/blog"},
	} {
		h, r, err := ParseImage(in)
		if err != nil || h != want[0] || r != want[1] {
			t.Errorf("ParseImage(%q) = %q, %q, %v; want %v", in, h, r, err, want)
		}
	}
	for _, in := range []string{"", "ghcr.io", "ghcr.io/", "ghcr.io/u/app:v1", "ghcr.io/u/app@sha256:ab", "/u/app"} {
		if _, _, err := ParseImage(in); err == nil {
			t.Errorf("ParseImage(%q) should fail", in)
		}
	}
}

func fastRetry(t *testing.T) {
	old := retryWait
	retryWait = time.Millisecond
	t.Cleanup(func() { retryWait = old })
}

func testRegistry(f *registryfake.Server, token string) *Registry {
	return &Registry{Host: "ghcr.io", Repo: "tavish/blog", User: "tavish", Token: token, Base: f.URL}
}

func TestPush_FirstAndSecond(t *testing.T) {
	fastRetry(t)
	f := registryfake.New(t, registryfake.Options{User: "tavish", Token: "s3cret", RequireAuth: true})
	reg := testRegistry(f, "s3cret")
	img, err := BuildImage(ociArtifact, "one")
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Push(context.Background(), img, "one"); err != nil {
		t.Fatal(err)
	}
	if got := f.TokenAuths(); len(got) != 1 || got[0] != "basic tavish:s3cret" {
		t.Errorf("token exchanges %v", got)
	}
	var token string
	for _, r := range f.Requests() {
		if strings.HasPrefix(r, "GET /token?") {
			token = r
		}
	}
	if !strings.Contains(token, "scope=repository%3Atavish%2Fblog%3Apull%2Cpush") || !strings.Contains(token, "service=registryfake") {
		t.Errorf("token request %q", token)
	}
	n := func(prefix string) (c int) {
		for _, r := range f.Requests() {
			if strings.HasPrefix(r, prefix) {
				c++
			}
		}
		return
	}
	if n("POST /v2/tavish/blog/blobs/uploads/") != 2 || n("PUT /v2/tavish/blog/blobs/uploads/") != 2 || n("PUT /v2/tavish/blog/manifests/one") != 1 {
		t.Errorf("first push: %v", f.Requests())
	}
	if !bytes.Equal(f.Manifest("tavish/blog", "one"), img.Manifest) || !bytes.Equal(f.Blob(img.LayerDigest), img.Layer) || !bytes.Equal(f.Blob(img.ConfigDigest), img.Config) {
		t.Error("the fake did not store the image")
	}

	// The same image under another tag: both blobs exist, nothing is uploaded.
	before := len(f.Requests())
	if err := reg.Push(context.Background(), img, "two"); err != nil {
		t.Fatal(err)
	}
	second := strings.Join(f.Requests()[before:], "\n")
	if n("HEAD ") != 4 || strings.Contains(second, "blobs/uploads/") || strings.Count(second, "PUT /v2/tavish/blog/manifests/two") != 1 || strings.Count(second, "HEAD ") != 2 {
		t.Errorf("second push:\n%s", second)
	}

	// Another build id over the same layer: the config differs, the layer is not uploaded again.
	img2, _ := BuildImage(ociArtifact, "three")
	before = len(f.Requests())
	if err := reg.Push(context.Background(), img2, "three"); err != nil {
		t.Fatal(err)
	}
	third := strings.Join(f.Requests()[before:], "\n")
	if strings.Count(third, "POST /v2/tavish/blog/blobs/uploads/") != 1 || strings.Count(third, "PUT /v2/tavish/blog/blobs/uploads/") != 1 || strings.Count(third, "HEAD ") != 2 {
		t.Errorf("third push:\n%s", third)
	}
}

func TestPush_Retry503(t *testing.T) {
	fastRetry(t)
	f := registryfake.New(t, registryfake.Options{Fail503Once: true})
	img, _ := BuildImage(ociArtifact, "one")
	if err := testRegistry(f, "").Push(context.Background(), img, "one"); err != nil {
		t.Fatal(err)
	}
	if f.Manifest("tavish/blog", "one") == nil {
		t.Error("no manifest")
	}
}

func TestPush_ErrorHidesToken(t *testing.T) {
	fastRetry(t)
	f := registryfake.New(t, registryfake.Options{User: "tavish", Token: "s3cret-token", ManifestStatus: 401})
	img, _ := BuildImage(ociArtifact, "one")
	err := testRegistry(f, "s3cret-token").Push(context.Background(), img, "one")
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "denied: 401") {
		t.Errorf("error %q", err)
	}
	if strings.Contains(err.Error(), "s3cret-token") {
		t.Errorf("error carries the token: %q", err)
	}
}

func TestPush_Anonymous(t *testing.T) {
	fastRetry(t)
	f := registryfake.New(t, registryfake.Options{RequireAuth: true})
	img, _ := BuildImage(ociArtifact, "one")
	if err := testRegistry(f, "").Push(context.Background(), img, "one"); err != nil {
		t.Fatal(err)
	}
	if got := f.TokenAuths(); len(got) != 1 || got[0] != "anonymous" {
		t.Errorf("token exchanges %v", got)
	}
}
