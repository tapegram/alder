// Package renderfake is a fake of the slice of Render's public REST API that `alder deploy`
// uses (owners, web services, env vars, scaling, registry credentials, deploys, logs and the
// logs websocket), served by net/http/httptest and held in memory, with the paths and bodies of
// https://api-docs.render.com. It is not Render: one workspace, no autoscaling, no builds, and a
// deploy's progress is a script the test sets rather than a clock.
//
// Beyond the API it simulates what the target's health check observes. Every service's
// serviceDetails.url is <fake>/svc/<name>, and GET /svc/<name>/_alder/health answers
// {"build":"<tag>","db":"ok"} round-robin over the service's instances, each running the image
// it was last switched to. An instance moves to a deploy's image when that deploy reaches
// "live"; DelaySwitch holds one instance back until ReleaseSwitch.
package renderfake

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// OwnerID is the id of the fake's one workspace.
const OwnerID = "tea-fake0001"

// Env is one environment variable.
type Env struct{ Key, Value string }

// Deploy is a deploy as the fake holds it.
type Deploy struct {
	ID, Status, Image string
	Created, Finished time.Time
}

// Credential is a registry credential, token included (it is a test fake).
type Credential struct{ ID, Name, Registry, Username, Token, OwnerID string }

// LogLine is a canned log line.
type LogLine struct {
	Time              time.Time
	Instance, Message string
}

// Call is one recorded request.
type Call struct {
	Method, Path string
	Query        url.Values
	Body         string
	Auth         string // the Authorization header as sent
}

// Service is a snapshot of a service's state.
type Service struct {
	ID, Name, OwnerID, Region, Plan, URL string
	Instances                            int
	Env                                  []Env
	Image                                string // the image of the newest live deploy
	CredentialID                         string
	HealthCheckPath                      string
	MaxShutdownDelay                     int
	Deploys                              []Deploy // oldest first
	InstanceImages                       []string // what each simulated instance runs
}

type dep struct {
	Deploy
	script []string
	step   int
}

type svc struct {
	Service
	deps    []*dep
	delayed map[int]bool
	rr      int
	hits    int
}

// Server is a running fake.
type Server struct {
	URL string // http://127.0.0.1:<port>

	srv *httptest.Server

	mu       sync.Mutex
	calls    []Call
	services []*svc
	creds    []Credential
	logs     []LogLine
	pending  []string // the script for the next deploy
	hasPend  bool
	fail429  int
	key      string
	n        int
}

// New starts a fake and stops it when t ends. Any non-empty bearer token is accepted until
// RequireKey says otherwise.
func New(t testing.TB) *Server {
	s := &Server{}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = s.srv.URL
	t.Cleanup(s.srv.Close)
	return s
}

// Requests is every API request so far, "METHOD /path" with "?query" when there is one. Health
// requests (/svc/...) are not listed; see HealthHits.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	for i, c := range s.calls {
		out[i] = c.Method + " " + c.Path
		if len(c.Query) > 0 {
			out[i] += "?" + c.Query.Encode()
		}
	}
	return out
}

// Calls is every API request so far with its query and body.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// RequireKey makes the fake answer 401 to any other bearer token. The 401 message echoes the
// Authorization header it received, as a careless API might, so tests can prove the client
// scrubs its key.
func (s *Server) RequireKey(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = key
}

// Fail429Once makes the next API request answer 429 with "Retry-After: 1". Calling it twice
// makes the next two requests fail.
func (s *Server) Fail429Once() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail429++
}

// Script sets the statuses the next deploy created walks through, one step per GET of that
// deploy; the last status repeats. A deploy created without a script walks
// update_in_progress, live. A script applies to one deploy only (a service creation's own deploy
// included).
func (s *Server) Script(statuses ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending, s.hasPend = append([]string(nil), statuses...), true
}

// Logs sets the canned log lines, served by GET /logs and sent on /logs/subscribe. Give them in
// ascending time.
func (s *Server) Logs(lines ...LogLine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append([]LogLine(nil), lines...)
}

// AddService seeds a service that already runs image on instances instances, with a live deploy
// of it.
func (s *Server) AddService(name, image string, instances int, env ...Env) *Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.newSvc(name, OwnerID, "oregon", "starter", instances, "", env)
	d := s.newDep(v, image)
	d.Status, d.Finished = "live", d.Created
	v.Image = image
	for i := range v.InstanceImages {
		v.InstanceImages[i] = image
	}
	snap := v.snapshot()
	return &snap
}

// Service is a snapshot of the named service, or nil.
func (s *Server) Service(name string) *Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.byName(name); v != nil {
		snap := v.snapshot()
		return &snap
	}
	return nil
}

// Credentials is the registry credentials created so far.
func (s *Server) Credentials() []Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Credential(nil), s.creds...)
}

// SetEnv sets the service's variable key to value, as the dashboard does (no deploy).
func (s *Server) SetEnv(name, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.byName(name)
	if v == nil {
		return
	}
	for i := range v.Env {
		if v.Env[i].Key == key {
			v.Env[i].Value = value
			return
		}
	}
	v.Env = append(v.Env, Env{key, value})
}

// InstancesServe makes every instance of the service run image at once.
func (s *Server) InstancesServe(name, image string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.byName(name); v != nil {
		for i := range v.InstanceImages {
			v.InstanceImages[i] = image
		}
	}
}

// DelaySwitch holds instance i back: when a deploy reaches live, that instance keeps running
// its old image until ReleaseSwitch.
func (s *Server) DelaySwitch(name string, i int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.byName(name); v != nil {
		v.delayed[i] = true
	}
}

// ReleaseSwitch lets the held instances move to the service's current image.
func (s *Server) ReleaseSwitch(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.byName(name); v != nil {
		v.delayed = map[int]bool{}
		for i := range v.InstanceImages {
			v.InstanceImages[i] = v.Image
		}
	}
}

// HealthHits is how many health requests the service has answered.
func (s *Server) HealthHits(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.byName(name); v != nil {
		return v.hits
	}
	return 0
}

// ---- state helpers (s.mu held)

func (v *svc) snapshot() Service {
	c := v.Service
	c.Env = append([]Env(nil), v.Env...)
	c.InstanceImages = append([]string(nil), v.InstanceImages...)
	c.Deploys = nil
	for _, d := range v.deps {
		c.Deploys = append(c.Deploys, d.Deploy)
	}
	return c
}

func (s *Server) id(prefix string) string {
	s.n++
	return fmt.Sprintf("%s-%010d", prefix, s.n)
}

func (s *Server) byName(name string) *svc {
	for _, v := range s.services {
		if v.Name == name {
			return v
		}
	}
	return nil
}

func (s *Server) byID(id string) *svc {
	for _, v := range s.services {
		if v.ID == id {
			return v
		}
	}
	return nil
}

func (s *Server) newSvc(name, owner, region, plan string, n int, cred string, env []Env) *svc {
	v := &svc{delayed: map[int]bool{}}
	v.Service = Service{
		ID: s.id("srv"), Name: name, OwnerID: owner, Region: region, Plan: plan,
		URL: s.URL + "/svc/" + name, Instances: n, Env: append([]Env(nil), env...),
		CredentialID: cred, HealthCheckPath: "/_alder/health", MaxShutdownDelay: 30,
		InstanceImages: make([]string, n),
	}
	s.services = append(s.services, v)
	return v
}

func (s *Server) newDep(v *svc, image string) *dep {
	d := &dep{Deploy: Deploy{ID: s.id("dep"), Status: "created", Image: image, Created: time.Now().UTC()}}
	if s.hasPend {
		d.script = s.pending
	} else {
		d.script = []string{"update_in_progress", "live"}
	}
	s.pending, s.hasPend = nil, false
	v.deps = append(v.deps, d)
	return d
}

func terminal(st string) bool {
	switch st {
	case "live", "build_failed", "update_failed", "pre_deploy_failed", "canceled", "deactivated":
		return true
	}
	return false
}

// advance takes one scripted step of a deploy.
func (v *svc) advance(d *dep) {
	if terminal(d.Status) || d.step >= len(d.script) {
		return
	}
	d.Status = d.script[d.step]
	d.step++
	if !terminal(d.Status) {
		return
	}
	d.Finished = time.Now().UTC()
	if d.Status != "live" {
		return
	}
	for _, o := range v.deps {
		if o != d && o.Status == "live" {
			o.Status = "deactivated"
		}
	}
	v.Image = d.Image
	for i := range v.InstanceImages {
		if !v.delayed[i] {
			v.InstanceImages[i] = d.Image
		}
	}
}

func tagOf(ref string) string {
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[i+1:]
	}
	return ref
}

// ---- HTTP

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if strings.HasPrefix(r.URL.Path, "/svc/") {
		s.health(w, r)
		return
	}
	s.mu.Lock()
	s.calls = append(s.calls, Call{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: string(body), Auth: r.Header.Get("Authorization")})
	auth := r.Header.Get("Authorization")
	tok := strings.TrimPrefix(auth, "Bearer ")
	if !strings.HasPrefix(auth, "Bearer ") || tok == "" || s.key != "" && tok != s.key {
		s.mu.Unlock()
		jsonOut(w, 401, map[string]any{"message": "unauthorized: " + auth})
		return
	}
	if s.fail429 > 0 {
		s.fail429--
		s.mu.Unlock()
		w.Header().Set("Retry-After", "1")
		jsonOut(w, 429, map[string]any{"message": "rate limited"})
		return
	}
	if r.URL.Path == "/logs/subscribe" {
		lines := append([]LogLine(nil), s.logs...)
		s.mu.Unlock()
		wsLogs(w, r, lines)
		return
	}
	defer s.mu.Unlock()
	s.api(w, r, body)
}

func jsonOut(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func msg(w http.ResponseWriter, code int, m string) { jsonOut(w, code, map[string]any{"message": m}) }

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/svc/"), "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.byName(parts[0])
	if v == nil || len(parts) != 3 || parts[1] != "_alder" || parts[2] != "health" || len(v.InstanceImages) == 0 {
		http.NotFound(w, r)
		return
	}
	v.hits++
	img := v.InstanceImages[v.rr%len(v.InstanceImages)]
	v.rr++
	if img == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		io.WriteString(w, `{"build":"","db":"starting"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"build":%q,"db":"ok"}`, tagOf(img))
}

// page serves a Render-style cursor page: [{"cursor":..,"<key>":item}].
func page(w http.ResponseWriter, r *http.Request, key string, items []any) {
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	start := 0
	if c := r.URL.Query().Get("cursor"); strings.HasPrefix(c, "c") {
		if k, err := strconv.Atoi(c[1:]); err == nil {
			start = k + 1
		}
	}
	out := []any{}
	for i := start; i < len(items) && len(out) < limit; i++ {
		out = append(out, map[string]any{"cursor": "c" + strconv.Itoa(i), key: items[i]})
	}
	jsonOut(w, 200, out)
}

func (v *svc) json() map[string]any {
	m := map[string]any{
		"id": v.ID, "name": v.Name, "type": "web_service", "ownerId": v.OwnerID,
		"imagePath": v.Image,
		"serviceDetails": map[string]any{
			"url": v.URL, "region": v.Region, "plan": v.Plan, "numInstances": v.Instances,
			"healthCheckPath": v.HealthCheckPath, "maxShutdownDelaySeconds": v.MaxShutdownDelay,
		},
	}
	if v.CredentialID != "" {
		m["registryCredential"] = map[string]any{"id": v.CredentialID}
	}
	return m
}

func (d *dep) json() map[string]any {
	m := map[string]any{"id": d.ID, "status": d.Status, "image": map[string]any{"ref": d.Image}, "createdAt": ts(d.Created), "trigger": "api"}
	if !d.Finished.IsZero() {
		m["finishedAt"] = ts(d.Finished)
	}
	return m
}

func (s *Server) api(w http.ResponseWriter, r *http.Request, body []byte) {
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	m := r.Method
	switch {
	case m == "GET" && len(p) == 1 && p[0] == "owners":
		page(w, r, "owner", []any{map[string]any{"id": OwnerID, "name": "alder-fake", "email": "fake@example.com", "type": "team"}})
	case len(p) == 1 && p[0] == "services":
		if m == "GET" {
			var items []any
			for _, v := range s.services {
				if n := r.URL.Query().Get("name"); n != "" && !strings.Contains(v.Name, n) {
					continue
				}
				if ty := r.URL.Query().Get("type"); ty != "" && ty != "web_service" {
					continue
				}
				items = append(items, v.json())
			}
			page(w, r, "service", items)
		} else if m == "POST" {
			s.createService(w, body)
		} else {
			msg(w, 405, "method not allowed")
		}
	case len(p) >= 2 && p[0] == "services":
		v := s.byID(p[1])
		if v == nil {
			msg(w, 404, "service not found")
			return
		}
		s.serviceRoute(w, r, v, p[2:], body)
	case len(p) == 1 && p[0] == "registrycredentials":
		s.credentials(w, r, body)
	case len(p) == 2 && p[0] == "registrycredentials" && m == "PATCH":
		s.updateCredential(w, p[1], body)
	case m == "GET" && len(p) == 1 && p[0] == "logs":
		s.logsList(w, r)
	default:
		msg(w, 404, "not found")
	}
}

func (s *Server) createService(w http.ResponseWriter, body []byte) {
	var in struct {
		Type, Name, OwnerID string
		Image               struct {
			ImagePath, OwnerID, RegistryCredentialID string
		}
		ServiceDetails struct {
			Runtime, Region, HealthCheckPath string
			Plan                             *string
			NumInstances                     int
			MaxShutdownDelaySeconds          int
		}
		EnvVars []struct{ Key, Value string }
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Name == "" || in.Type != "web_service" || in.Image.ImagePath == "" {
		msg(w, 400, "invalid service")
		return
	}
	// Like Render: a service without a build source needs its runtime, and a plan, when
	// given, must be one of the plans (the empty string is not; leave it out for the default).
	if in.ServiceDetails.Runtime != "image" {
		msg(w, 400, "serviceDetails.runtime: must be one of docker, elixir, go, node, python, ruby, rust, image")
		return
	}
	if in.ServiceDetails.Plan != nil && *in.ServiceDetails.Plan == "" {
		msg(w, 400, "serviceDetails.plan: invalid plan")
		return
	}
	plan := "starter"
	if in.ServiceDetails.Plan != nil {
		plan = *in.ServiceDetails.Plan
	}
	if s.byName(in.Name) != nil {
		msg(w, 409, "service name already in use")
		return
	}
	if in.ServiceDetails.NumInstances == 0 {
		in.ServiceDetails.NumInstances = 1
	}
	if in.ServiceDetails.Region == "" {
		in.ServiceDetails.Region = "oregon"
	}
	var env []Env
	for _, e := range in.EnvVars {
		env = append(env, Env{e.Key, e.Value})
	}
	v := s.newSvc(in.Name, in.OwnerID, in.ServiceDetails.Region, plan, in.ServiceDetails.NumInstances, in.Image.RegistryCredentialID, env)
	if in.ServiceDetails.HealthCheckPath != "" {
		v.HealthCheckPath = in.ServiceDetails.HealthCheckPath
	}
	if in.ServiceDetails.MaxShutdownDelaySeconds != 0 {
		v.MaxShutdownDelay = in.ServiceDetails.MaxShutdownDelaySeconds
	}
	// Like Render, creating an image-backed service starts a deploy of its image.
	d := s.newDep(v, in.Image.ImagePath)
	j := v.json()
	j["imagePath"] = in.Image.ImagePath
	jsonOut(w, 201, map[string]any{"service": j, "deployId": d.ID})
}

func (s *Server) serviceRoute(w http.ResponseWriter, r *http.Request, v *svc, rest []string, body []byte) {
	m := r.Method
	switch {
	case len(rest) == 0 && m == "GET":
		jsonOut(w, 200, v.json())
	case len(rest) == 1 && rest[0] == "env-vars" && m == "GET":
		var items []any
		for _, e := range v.Env {
			items = append(items, map[string]any{"key": e.Key, "value": e.Value})
		}
		page(w, r, "envVar", items)
	case len(rest) == 1 && rest[0] == "env-vars" && m == "PUT":
		var in []struct{ Key, Value string }
		if err := json.Unmarshal(body, &in); err != nil {
			msg(w, 400, "invalid env vars")
			return
		}
		v.Env = nil
		var items []any
		for _, e := range in {
			v.Env = append(v.Env, Env{e.Key, e.Value})
			items = append(items, map[string]any{"key": e.Key, "value": e.Value})
		}
		page(w, r, "envVar", items) // like Render, a PUT starts no deploy
	case len(rest) == 1 && rest[0] == "scale" && m == "POST":
		var in struct{ NumInstances *int }
		if err := json.Unmarshal(body, &in); err != nil || in.NumInstances == nil || *in.NumInstances < 0 {
			msg(w, 400, "numInstances required")
			return
		}
		n := *in.NumInstances
		for len(v.InstanceImages) < n {
			v.InstanceImages = append(v.InstanceImages, v.Image)
		}
		v.InstanceImages = v.InstanceImages[:n]
		v.Instances = n
		w.WriteHeader(202)
	case len(rest) == 1 && rest[0] == "deploys" && m == "GET":
		var items []any
		for i := len(v.deps) - 1; i >= 0; i-- { // newest first
			items = append(items, v.deps[i].json())
		}
		page(w, r, "deploy", items)
	case len(rest) == 1 && rest[0] == "deploys" && m == "POST":
		var in struct{ ImageURL string }
		if err := json.Unmarshal(body, &in); err != nil || in.ImageURL == "" {
			msg(w, 400, "imageUrl required")
			return
		}
		jsonOut(w, 201, s.newDep(v, in.ImageURL).json())
	case len(rest) >= 2 && rest[0] == "deploys":
		var d *dep
		for _, c := range v.deps {
			if c.ID == rest[1] {
				d = c
			}
		}
		if d == nil {
			msg(w, 404, "deploy not found")
		} else if len(rest) == 2 && m == "GET" {
			v.advance(d)
			jsonOut(w, 200, d.json())
		} else if len(rest) == 3 && rest[2] == "cancel" && m == "POST" {
			if terminal(d.Status) {
				msg(w, 409, "deploy is not cancelable")
				return
			}
			d.Status, d.Finished = "canceled", time.Now().UTC()
			jsonOut(w, 200, d.json())
		} else {
			msg(w, 404, "not found")
		}
	default:
		msg(w, 404, "not found")
	}
}

func (s *Server) credentials(w http.ResponseWriter, r *http.Request, body []byte) {
	if r.Method == "GET" {
		var items []any
		for _, c := range s.creds {
			if n := r.URL.Query().Get("name"); n != "" && !strings.Contains(c.Name, n) {
				continue
			}
			items = append(items, map[string]any{"id": c.ID, "name": c.Name, "registry": c.Registry, "username": c.Username})
		}
		page(w, r, "registryCredential", items)
		return
	}
	var in struct{ Name, Registry, Username, AuthToken, OwnerID string }
	if r.Method != "POST" || json.Unmarshal(body, &in) != nil || in.Name == "" || in.Registry == "" || in.Username == "" || in.AuthToken == "" || in.OwnerID == "" {
		msg(w, 400, "invalid registry credential")
		return
	}
	c := Credential{ID: s.id("rcr"), Name: in.Name, Registry: in.Registry, Username: in.Username, Token: in.AuthToken, OwnerID: in.OwnerID}
	s.creds = append(s.creds, c)
	jsonOut(w, 200, map[string]any{"id": c.ID, "name": c.Name, "registry": c.Registry, "username": c.Username})
}

// updateCredential is PATCH /registrycredentials/{id}: like Render, every field is required.
func (s *Server) updateCredential(w http.ResponseWriter, id string, body []byte) {
	var in struct{ Name, Registry, Username, AuthToken string }
	if json.Unmarshal(body, &in) != nil || in.Name == "" || in.Registry == "" || in.Username == "" || in.AuthToken == "" {
		msg(w, 400, "invalid registry credential")
		return
	}
	for i := range s.creds {
		if c := &s.creds[i]; c.ID == id {
			c.Name, c.Registry, c.Username, c.Token = in.Name, in.Registry, in.Username, in.AuthToken
			jsonOut(w, 200, map[string]any{"id": c.ID, "name": c.Name, "registry": c.Registry, "username": c.Username, "updatedAt": ts(time.Now())})
			return
		}
	}
	msg(w, 404, "registry credential not found")
}

func logJSON(l LogLine, resource string) map[string]any {
	return map[string]any{
		"id": "log-" + strconv.FormatInt(l.Time.UnixNano(), 10), "message": l.Message, "timestamp": ts(l.Time),
		"labels": []any{
			map[string]any{"name": "resource", "value": resource},
			map[string]any{"name": "instance", "value": l.Instance},
		},
	}
}

func (s *Server) logsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("ownerId") == "" || q.Get("resource") == "" {
		msg(w, 400, "ownerId and resource are required")
		return
	}
	limit := 20
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	var start time.Time
	if st := q.Get("startTime"); st != "" {
		t, err := time.Parse(time.RFC3339Nano, st)
		if err != nil {
			msg(w, 400, "bad startTime")
			return
		}
		start = t
	}
	var sel []LogLine
	for _, l := range s.logs {
		if !l.Time.Before(start) {
			sel = append(sel, l)
		}
	}
	if q.Get("direction") != "forward" { // backward (the default): newest first
		for i, j := 0, len(sel)-1; i < j; i, j = i+1, j-1 {
			sel[i], sel[j] = sel[j], sel[i]
		}
	}
	more := len(sel) > limit
	if more {
		sel = sel[:limit]
	}
	out := []any{}
	next := start
	for _, l := range sel {
		out = append(out, logJSON(l, q.Get("resource")))
		next = l.Time.Add(time.Nanosecond)
	}
	if next.IsZero() {
		next = time.Now()
	}
	jsonOut(w, 200, map[string]any{"hasMore": more, "nextStartTime": ts(next), "logs": out})
}

// ---- websocket (RFC 6455, minimal: text frames out, client frames read to notice a close)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func wsFrame(op byte, payload []byte) []byte {
	b := []byte{0x80 | op}
	n := len(payload)
	switch {
	case n < 126:
		b = append(b, byte(n))
	case n < 65536:
		b = append(b, 126, byte(n>>8), byte(n))
	default:
		b = append(b, 127)
		b = binary.BigEndian.AppendUint64(b, uint64(n))
	}
	return append(b, payload...)
}

func wsLogs(w http.ResponseWriter, r *http.Request, lines []LogLine) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if r.Header.Get("Upgrade") != "websocket" || key == "" {
		msg(w, 400, "websocket upgrade required")
		return
	}
	conn, brw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	sum := sha1.Sum([]byte(key + wsGUID))
	fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
	for _, l := range lines {
		b, _ := json.Marshal(logJSON(l, r.URL.Query().Get("resource")))
		brw.Write(wsFrame(0x1, b))
	}
	brw.Flush()
	readClient(brw.Reader, conn)
}

// readClient reads (and discards) client frames until the client closes or the connection ends,
// echoing a close.
func readClient(r *bufio.Reader, conn io.Writer) {
	for {
		var h [2]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return
		}
		n := uint64(h[1] & 0x7f)
		switch n {
		case 126:
			var l [2]byte
			if _, err := io.ReadFull(r, l[:]); err != nil {
				return
			}
			n = uint64(binary.BigEndian.Uint16(l[:]))
		case 127:
			var l [8]byte
			if _, err := io.ReadFull(r, l[:]); err != nil {
				return
			}
			n = binary.BigEndian.Uint64(l[:])
		}
		if h[1]&0x80 != 0 {
			var k [4]byte
			if _, err := io.ReadFull(r, k[:]); err != nil {
				return
			}
		}
		if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
			return
		}
		if h[0]&0x0f == 0x8 {
			conn.Write(wsFrame(0x8, []byte{0x03, 0xe8}))
			return
		}
	}
}
