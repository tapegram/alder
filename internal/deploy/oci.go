package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"alder/internal/deploy/cacerts"
)

// Image is a one-layer OCI image built from an artifact directory. Everything in it is a
// function of the artifact and the build id: the same inputs give the same Digest.
type Image struct {
	Layer        []byte // the gzipped tar
	LayerDigest  string // sha256:<hex> of Layer
	DiffID       string // sha256:<hex> of the uncompressed tar
	Config       []byte
	ConfigDigest string
	Manifest     []byte
	Digest       string // sha256:<hex> of Manifest: the image's digest
}

const (
	mediaManifest = "application/vnd.oci.image.manifest.v1+json"
	mediaConfig   = "application/vnd.oci.image.config.v1+json"
	mediaLayer    = "application/vnd.oci.image.layer.v1.tar+gzip"
)

// caBundlePath is where Go's crypto/x509 looks for roots on Linux.
const caBundlePath = "etc/ssl/certs/ca-certificates.crt"

type ociDesc struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

type ociConfig struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		Entrypoint   []string            `json:"Entrypoint"`
		Env          []string            `json:"Env"`
		ExposedPorts map[string]struct{} `json:"ExposedPorts"`
		WorkingDir   string              `json:"WorkingDir"`
	} `json:"config"`
	RootFS struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
}

type ociManifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	MediaType     string    `json:"mediaType"`
	Config        ociDesc   `json:"config"`
	Layers        []ociDesc `json:"layers"`
}

type layerEntry struct {
	name string // tar name; a directory ends in "/"
	mode int64
	data []byte
	src  string // a file to read when data is nil
}

// BuildImage builds the image of the artifact directory artifact for the build id. The layer
// holds app/app, app/static/, app/migrations/, app/alder-deploy.json and the CA bundle, in
// sorted order with fixed times, owners and gzip header, so the bytes do not depend on when or
// where it was built.
func BuildImage(artifact, id string) (*Image, error) {
	entries := map[string]layerEntry{}
	dir := func(name string) {
		for d := path.Dir(strings.TrimSuffix(name, "/")); d != "." && d != "/"; d = path.Dir(d) {
			entries[d+"/"] = layerEntry{name: d + "/", mode: 0o755}
		}
	}
	add := func(e layerEntry) {
		entries[e.name] = e
		dir(e.name)
	}
	for _, f := range []struct {
		name, src string
		mode      int64
	}{
		{"app/app", BinaryName, 0o755},
		{"app/" + ArtifactFile, ArtifactFile, 0o644},
	} {
		p := filepath.Join(artifact, f.src)
		if st, err := os.Lstat(p); err != nil || !st.Mode().IsRegular() {
			return nil, fmt.Errorf("artifact %s: %s is missing or not a regular file", artifact, f.src)
		}
		add(layerEntry{name: f.name, mode: f.mode, src: p})
	}
	for _, sub := range []string{"static", "migrations"} {
		root := filepath.Join(artifact, sub)
		if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(artifact, p)
			name := "app/" + filepath.ToSlash(rel)
			switch {
			case d.IsDir():
				add(layerEntry{name: name + "/", mode: 0o755})
			case d.Type().IsRegular():
				add(layerEntry{name: name, mode: 0o644, src: p})
			default:
				return fmt.Errorf("artifact %s: %s is neither a file nor a directory", artifact, rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	add(layerEntry{name: caBundlePath, mode: 0o644, data: cacerts.PEM})

	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)

	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, n := range names {
		e := entries[n]
		data := e.data
		if e.src != "" {
			var err error
			if data, err = os.ReadFile(e.src); err != nil {
				return nil, err
			}
		}
		h := &tar.Header{Name: e.name, Mode: e.mode, ModTime: epoch, Format: tar.FormatPAX}
		if strings.HasSuffix(e.name, "/") {
			h.Typeflag = tar.TypeDir
		} else {
			h.Typeflag = tar.TypeReg
			h.Size = int64(len(data))
		}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}

	var gz bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	zw.Header = gzip.Header{OS: 255}
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	img := &Image{Layer: gz.Bytes(), LayerDigest: sha256Hex(gz.Bytes()), DiffID: sha256Hex(raw.Bytes())}
	var cfg ociConfig
	cfg.Architecture, cfg.OS = "amd64", "linux"
	cfg.Config.Entrypoint = []string{"/app/app"}
	cfg.Config.Env = []string{"ALDER_BUILD=" + id, "ALDER_STATIC=/app/static", "ALDER_ADDR=:10000"}
	cfg.Config.ExposedPorts = map[string]struct{}{"10000/tcp": {}}
	cfg.Config.WorkingDir = "/app"
	cfg.RootFS.Type, cfg.RootFS.DiffIDs = "layers", []string{img.DiffID}
	var err error
	if img.Config, err = json.Marshal(cfg); err != nil {
		return nil, err
	}
	img.ConfigDigest = sha256Hex(img.Config)
	img.Manifest, err = json.Marshal(ociManifest{
		SchemaVersion: 2, MediaType: mediaManifest,
		Config: ociDesc{mediaConfig, img.ConfigDigest, len(img.Config)},
		Layers: []ociDesc{{mediaLayer, img.LayerDigest, len(img.Layer)}},
	})
	if err != nil {
		return nil, err
	}
	img.Digest = sha256Hex(img.Manifest)
	return img, nil
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

// Registry is a repository at a container registry, pushed to with the registry HTTP API v2.
type Registry struct {
	Host, Repo, User, Token string
	Client                  *http.Client
	Base                    string // tests: the fake's URL; "" means https://<Host>

	bearer    string // the token the exchange returned; never in an error
	basicAuth bool   // the registry asked for Basic credentials
}

// ParseImage splits an image path such as ghcr.io/tavish/blog (no tag) into the registry's API
// host and the repository. docker.io is served from registry-1.docker.io, and a one-segment
// Docker Hub name lives under library/.
func ParseImage(p string) (host, repo string, err error) {
	host, repo, ok := strings.Cut(p, "/")
	if !ok || host == "" || repo == "" || strings.HasSuffix(repo, "/") || strings.ContainsAny(repo, "@: ") {
		return "", "", fmt.Errorf("image %q: want <registry>/<repository> without a tag, such as ghcr.io/you/app", p)
	}
	if host == "docker.io" {
		host = "registry-1.docker.io"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	}
	return host, repo, nil
}

// retryWait is the first backoff between attempts; the next is twice it. Tests shorten it.
var retryWait = time.Second

const attempts = 3

// Push uploads the image's blobs the registry lacks, then puts the manifest under tag.
func (r *Registry) Push(ctx context.Context, img *Image, tag string) error {
	if err := r.authenticate(ctx); err != nil {
		return err
	}
	for _, b := range []struct {
		digest string
		data   []byte
	}{{img.LayerDigest, img.Layer}, {img.ConfigDigest, img.Config}} {
		if err := r.pushBlob(ctx, b.digest, b.data); err != nil {
			return err
		}
	}
	resp, body, err := r.do(ctx, http.MethodPut, r.base()+"/v2/"+r.Repo+"/manifests/"+tag, img.Manifest, mediaManifest)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return r.statusError(resp, body)
	}
	return nil
}

func (r *Registry) base() string {
	if r.Base != "" {
		return strings.TrimSuffix(r.Base, "/")
	}
	return "https://" + r.Host
}

func (r *Registry) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

// authenticate pings /v2/ and, when the registry challenges, gets the bearer token the
// challenge's realm issues for pull and push on the repository (anonymously without a Token).
func (r *Registry) authenticate(ctx context.Context) error {
	r.bearer, r.basicAuth = "", false
	resp, body, err := r.do(ctx, http.MethodGet, r.base()+"/v2/", nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return r.statusError(resp, body)
	}
	scheme, params := parseChallenge(resp.Header.Get("WWW-Authenticate"))
	switch scheme {
	case "basic":
		r.basicAuth = r.Token != ""
		return nil
	case "bearer":
	default:
		return fmt.Errorf("registry %s: unsupported authentication challenge %q", r.Host, resp.Header.Get("WWW-Authenticate"))
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || realm.Host == "" {
		return fmt.Errorf("registry %s: the challenge names no usable realm", r.Host)
	}
	if realm.Scheme != "https" && !(realm.Scheme == "http" && strings.HasPrefix(r.base(), "http://")) {
		return fmt.Errorf("registry %s: refusing a token realm that is not https", r.Host)
	}
	q := realm.Query()
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	q.Set("scope", "repository:"+r.Repo+":pull,push")
	realm.RawQuery = q.Encode()
	resp, body, err = r.doAuth(ctx, http.MethodGet, realm.String(), nil, "", r.Token != "")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return r.statusError(resp, body)
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || (tok.Token == "" && tok.AccessToken == "") {
		return fmt.Errorf("registry %s: the token exchange answered no token", r.Host)
	}
	r.bearer = tok.Token
	if r.bearer == "" {
		r.bearer = tok.AccessToken
	}
	return nil
}

// parseChallenge reads `Bearer realm="…",service="…"`: the lower-cased scheme and its parameters.
func parseChallenge(h string) (scheme string, params map[string]string) {
	params = map[string]string{}
	scheme, rest, _ := strings.Cut(strings.TrimSpace(h), " ")
	scheme = strings.ToLower(scheme)
	for rest = strings.TrimSpace(rest); rest != ""; {
		k, v, ok := strings.Cut(rest, "=")
		if !ok {
			break
		}
		k = strings.ToLower(strings.TrimSpace(k))
		if strings.HasPrefix(v, `"`) {
			end := strings.Index(v[1:], `"`)
			if end < 0 {
				break
			}
			params[k], rest = v[1:1+end], v[end+2:]
		} else {
			val, after, _ := strings.Cut(v, ",")
			params[k], rest = strings.TrimSpace(val), after
			rest = strings.TrimLeft(rest, ", ")
			continue
		}
		rest = strings.TrimLeft(rest, ", ")
	}
	return scheme, params
}

func (r *Registry) pushBlob(ctx context.Context, digest string, data []byte) error {
	resp, body, err := r.do(ctx, http.MethodHead, r.base()+"/v2/"+r.Repo+"/blobs/"+digest, nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode != http.StatusNotFound {
		return r.statusError(resp, body)
	}
	resp, body, err = r.do(ctx, http.MethodPost, r.base()+"/v2/"+r.Repo+"/blobs/uploads/", nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusAccepted {
		return r.statusError(resp, body)
	}
	loc, err := url.Parse(r.base() + "/v2/" + r.Repo + "/blobs/uploads/")
	if err != nil {
		return err
	}
	ref, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || resp.Header.Get("Location") == "" {
		return fmt.Errorf("registry %s: the upload answered no location", r.Host)
	}
	loc = loc.ResolveReference(ref)
	q := loc.Query()
	q.Set("digest", digest)
	loc.RawQuery = q.Encode()
	resp, body, err = r.do(ctx, http.MethodPut, loc.String(), data, "application/octet-stream")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return r.statusError(resp, body)
	}
	return nil
}

// sameOrigin reports whether u is on base's scheme, host and port, a default port (:443 on
// https, :80 on http) the same as none. Credentials go only there: never to another host, and
// never over http unless the registry itself is served over http.
func sameOrigin(u, base *url.URL) bool {
	port := func(v *url.URL) string {
		if p := v.Port(); p != "" {
			return p
		}
		switch strings.ToLower(v.Scheme) {
		case "https":
			return "443"
		case "http":
			return "80"
		}
		return ""
	}
	return strings.EqualFold(u.Scheme, base.Scheme) && strings.EqualFold(u.Hostname(), base.Hostname()) && port(u) == port(base)
}

// do sends one request with the bearer token (or Basic credentials when the registry asked for
// them and the token exchange was not needed), retrying 5xx and 429.
func (r *Registry) do(ctx context.Context, method, u string, body []byte, contentType string) (*http.Response, []byte, error) {
	return r.doAuth(ctx, method, u, body, contentType, false)
}

func (r *Registry) doAuth(ctx context.Context, method, u string, body []byte, contentType string, basic bool) (*http.Response, []byte, error) {
	target, err := url.Parse(u)
	if err != nil {
		return nil, nil, err
	}
	base, _ := url.Parse(r.base())
	sameHost := sameOrigin(target, base)
	wait := retryWait
	for attempt := 1; ; attempt++ {
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rd)
		if err != nil {
			return nil, nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		switch {
		case basic || (r.basicAuth && sameHost):
			req.SetBasicAuth(r.User, r.Token)
		case r.bearer != "" && sameHost:
			req.Header.Set("Authorization", "Bearer "+r.bearer)
		}
		resp, err := r.client().Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("registry %s %s: %s", method, redactURL(target), r.redact(err.Error()))
		}
		data, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if rerr != nil {
			return nil, nil, fmt.Errorf("registry %s %s: %s", method, redactURL(target), r.redact(rerr.Error()))
		}
		if (resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests) || attempt == attempts {
			return resp, data, nil
		}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
			wait = min(time.Duration(s)*time.Second, 30*time.Second)
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(wait):
		}
		wait *= 2
	}
}

func redactURL(u *url.URL) string { return u.Scheme + "://" + u.Host + u.Path }

func (r *Registry) redact(s string) string {
	for _, secret := range []string{r.Token, r.bearer} {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "***")
		}
	}
	return s
}

func (r *Registry) statusError(resp *http.Response, body []byte) error {
	b := strings.TrimSpace(string(body))
	if len(b) > 512 {
		b = b[:512] + "..."
	}
	return fmt.Errorf("registry %s %s: %d %s: %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, http.StatusText(resp.StatusCode), r.redact(b))
}
