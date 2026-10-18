// Package registryfake is a fake container registry for the tests: the slice of the registry
// HTTP API v2 that `alder deploy` pushes with (the version check, the bearer token exchange,
// blob existence checks, monolithic blob uploads and manifest puts), served by
// net/http/httptest and held in memory. It is not a registry: no chunked uploads, no mounts, no
// deletes, no listing, and manifests are stored as sent.
package registryfake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Options configures a fake.
type Options struct {
	User, Token string // when Token is set, the token exchange needs Basic User:Token
	RequireAuth bool   // every /v2/ request but the ping and the exchange needs the bearer token
	Fail503Once bool   // the first request to a repository path answers 503 once
	// ManifestStatus, when not zero, is the status every manifest PUT answers, with the body
	// "denied: <status>" (a registry refusing the push).
	ManifestStatus int
	// EchoAuth makes the ManifestStatus body also echo the Authorization header received and
	// the bearer the fake issues, as a careless registry might.
	EchoAuth bool
}

// Server is a running fake.
type Server struct {
	URL string // http://127.0.0.1:<port>

	opt Options
	srv *httptest.Server

	mu        sync.Mutex
	reqs      []string
	tokenAuth []string
	blobs     map[string][]byte
	manifests map[string][]byte // "<repo>:<tag>"
	uploads   map[string]string // upload id → repo
	failed    bool
	n         int
}

// Bearer is the token the exchange hands out.
const Bearer = "fake-registry-bearer"

// New starts a fake and stops it when t ends.
func New(t testing.TB, opt Options) *Server {
	s := &Server{opt: opt, blobs: map[string][]byte{}, manifests: map[string][]byte{}, uploads: map[string]string{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = s.srv.URL
	t.Cleanup(s.srv.Close)
	return s
}

// Requests is every request so far, "METHOD /path" (the token exchange also carries its query).
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.reqs...)
}

// TokenAuths is, for each token exchange so far, "basic user:token" or "anonymous".
func (s *Server) TokenAuths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tokenAuth...)
}

// Manifest is the manifest put at repo:tag, or nil.
func (s *Server) Manifest(repo, tag string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.manifests[repo+":"+tag]
}

// Blob is the blob with the digest ("sha256:..."), or nil.
func (s *Server) Blob(digest string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blobs[digest]
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	line := r.Method + " " + r.URL.Path
	if r.URL.Path == "/token" {
		line += "?" + r.URL.RawQuery
	}
	s.reqs = append(s.reqs, line)

	if r.URL.Path == "/token" {
		s.token(w, r)
		return
	}
	path := r.URL.Path
	if path == "/v2/" || path == "/v2" {
		if s.opt.RequireAuth && r.Header.Get("Authorization") != "Bearer "+Bearer {
			s.challenge(w)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if !strings.HasPrefix(path, "/v2/") {
		http.NotFound(w, r)
		return
	}
	if s.opt.RequireAuth && r.Header.Get("Authorization") != "Bearer "+Bearer {
		s.challenge(w)
		return
	}
	if s.opt.Fail503Once && !s.failed {
		s.failed = true
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(path, "/v2/")
	switch {
	case strings.Contains(rest, "/blobs/uploads/"):
		s.upload(w, r, rest, body)
	case strings.Contains(rest, "/blobs/"):
		i := strings.Index(rest, "/blobs/")
		blob, ok := s.blobs[rest[i+len("/blobs/"):]]
		if r.Method != http.MethodHead && r.Method != http.MethodGet || !ok {
			http.Error(w, `{"errors":[{"code":"BLOB_UNKNOWN"}]}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(blob)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			w.Write(blob)
		}
	case strings.Contains(rest, "/manifests/"):
		s.manifest(w, r, rest, body)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="registryfake"`, s.URL))
	http.Error(w, `{"errors":[{"code":"UNAUTHORIZED"}]}`, http.StatusUnauthorized)
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	user, pass, basic := r.BasicAuth()
	if basic {
		s.tokenAuth = append(s.tokenAuth, "basic "+user+":"+pass)
	} else {
		s.tokenAuth = append(s.tokenAuth, "anonymous")
	}
	if s.opt.Token != "" && (!basic || user != s.opt.User || pass != s.opt.Token) {
		http.Error(w, `{"errors":[{"code":"UNAUTHORIZED"}]}`, http.StatusUnauthorized)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"token": Bearer})
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request, rest string, body []byte) {
	repo := rest[:strings.Index(rest, "/blobs/uploads/")]
	id := rest[strings.Index(rest, "/blobs/uploads/")+len("/blobs/uploads/"):]
	switch r.Method {
	case http.MethodPost:
		s.n++
		id = fmt.Sprintf("u%d", s.n)
		s.uploads[id] = repo
		// A path with a state parameter already in it, as real registries send.
		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s?_state=x", repo, id))
		w.WriteHeader(http.StatusAccepted)
	case http.MethodPut:
		if s.uploads[id] != repo {
			http.Error(w, `{"errors":[{"code":"BLOB_UPLOAD_UNKNOWN"}]}`, http.StatusNotFound)
			return
		}
		sum := sha256.Sum256(body)
		got := "sha256:" + hex.EncodeToString(sum[:])
		if want := r.URL.Query().Get("digest"); want != got {
			http.Error(w, fmt.Sprintf(`{"errors":[{"code":"DIGEST_INVALID","message":"%s != %s"}]}`, want, got), http.StatusBadRequest)
			return
		}
		delete(s.uploads, id)
		s.blobs[got] = body
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request, rest string, body []byte) {
	i := strings.Index(rest, "/manifests/")
	repo, tag := rest[:i], rest[i+len("/manifests/"):]
	if r.Method != http.MethodPut {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if s.opt.ManifestStatus != 0 {
		msg := fmt.Sprintf("denied: %d", s.opt.ManifestStatus)
		if s.opt.EchoAuth {
			msg += " authorization=" + r.Header.Get("Authorization") + " bearer=" + Bearer
		}
		http.Error(w, msg, s.opt.ManifestStatus)
		return
	}
	var m struct {
		Config struct{ Digest string }
		Layers []struct{ Digest string }
	}
	if err := json.Unmarshal(body, &m); err != nil {
		http.Error(w, `{"errors":[{"code":"MANIFEST_INVALID"}]}`, http.StatusBadRequest)
		return
	}
	need := []string{m.Config.Digest}
	for _, l := range m.Layers {
		need = append(need, l.Digest)
	}
	for _, d := range need {
		if _, ok := s.blobs[d]; !ok {
			http.Error(w, `{"errors":[{"code":"MANIFEST_BLOB_UNKNOWN"}]}`, http.StatusBadRequest)
			return
		}
	}
	s.manifests[repo+":"+tag] = body
	w.WriteHeader(http.StatusCreated)
}
