package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// RenderAPI is a client for the slice of Render's public REST API (https://api-docs.render.com)
// that the Render target uses. The key travels only in the Authorization header: it is in no
// URL, error or String() output.
type RenderAPI struct {
	Base, Key string // Base has no trailing slash, e.g. https://api.render.com/v1
	Client    *http.Client
}

// NewRenderAPI reads ALDER_RENDER_API (default https://api.render.com/v1) and RENDER_API_KEY.
func NewRenderAPI() *RenderAPI {
	base := strings.TrimRight(os.Getenv("ALDER_RENDER_API"), "/")
	if base == "" {
		base = "https://api.render.com/v1"
	}
	return &RenderAPI{Base: base, Key: os.Getenv("RENDER_API_KEY"), Client: &http.Client{Timeout: 60 * time.Second}}
}

// String and GoString never show the key.
func (a RenderAPI) String() string   { return "RenderAPI{Base: " + a.Base + "}" }
func (a RenderAPI) GoString() string { return a.String() }

// AuthHeader is the header set a websocket dial to LogsURL needs.
func (a *RenderAPI) AuthHeader() http.Header {
	return http.Header{"Authorization": {"Bearer " + a.Key}}
}

// RenderAPIError is a non-2xx answer.
type RenderAPIError struct {
	Status  int
	Message string
}

func (e *RenderAPIError) Error() string { return fmt.Sprintf("Render API %d: %s", e.Status, e.Message) }

// renderWait waits d or until ctx ends; tests replace it.
var renderWait = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// do sends one request (and, after a 429, one retry following Retry-After) and decodes a JSON
// answer into out when out is not nil.
func (a *RenderAPI) do(ctx context.Context, method, path string, q url.Values, in, out any) error {
	var payload []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		payload = b
	}
	u := a.Base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	for attempt := 0; ; attempt++ {
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rd)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+a.Key)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("Render API %s %s: %s", method, path, a.redact(err.Error()))
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			wait := time.Second
			if n, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && n >= 0 {
				wait = time.Duration(min(n, 120)) * time.Second
			}
			if err := renderWait(ctx, wait); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return &RenderAPIError{Status: resp.StatusCode, Message: a.errMessage(data)}
		}
		if out == nil || len(bytes.TrimSpace(data)) == 0 {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("Render API %s %s: bad response: %w", method, path, err)
		}
		return nil
	}
}

func (a *RenderAPI) redact(s string) string {
	if a.Key != "" {
		s = strings.ReplaceAll(s, a.Key, "[redacted]")
	}
	return s
}

// errMessage is the API's "message" field, else a bounded piece of the body.
func (a *RenderAPI) errMessage(body []byte) string {
	var m struct{ Message string }
	s := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		s = m.Message
	}
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	if s == "" {
		s = "(no message)"
	}
	return a.redact(s)
}

// pages walks a cursor-paged list ([{cursor, <key>: item}]) calling fn on each item until fn
// returns true or the list ends.
func (a *RenderAPI) pages(ctx context.Context, path string, q url.Values, key string, limit int, fn func(json.RawMessage) bool) error {
	cursor := ""
	for {
		qq := url.Values{}
		for k, v := range q {
			qq[k] = v
		}
		qq.Set("limit", strconv.Itoa(limit))
		if cursor != "" {
			qq.Set("cursor", cursor)
		}
		var items []map[string]json.RawMessage
		if err := a.do(ctx, "GET", path, qq, nil, &items); err != nil {
			return err
		}
		for _, it := range items {
			if fn(it[key]) {
				return nil
			}
		}
		if len(items) < limit {
			return nil
		}
		if err := json.Unmarshal(items[len(items)-1]["cursor"], &cursor); err != nil || cursor == "" {
			return nil
		}
	}
}

// RenderOwner is a workspace.
type RenderOwner struct{ ID, Name, Type string }

// Owners lists the workspaces the key can see.
func (a *RenderAPI) Owners(ctx context.Context) ([]RenderOwner, error) {
	var out []RenderOwner
	err := a.pages(ctx, "/owners", nil, "owner", 100, func(r json.RawMessage) bool {
		var o RenderOwner
		if json.Unmarshal(r, &o) == nil {
			out = append(out, o)
		}
		return false
	})
	return out, err
}

// RenderService is a web service.
type RenderService struct {
	ID, Name, OwnerID, URL, Region, Plan string
	NumInstances                         int
	ImagePath, RegistryCredentialID      string
}

type renderServiceJSON struct {
	ID, Name, OwnerID, ImagePath string
	RegistryCredential           struct{ ID string }
	ServiceDetails               struct {
		URL, Region, Plan string
		NumInstances      int
	}
}

func (j renderServiceJSON) service() RenderService {
	return RenderService{ID: j.ID, Name: j.Name, OwnerID: j.OwnerID, URL: j.ServiceDetails.URL, Region: j.ServiceDetails.Region,
		Plan: j.ServiceDetails.Plan, NumInstances: j.ServiceDetails.NumInstances, ImagePath: j.ImagePath, RegistryCredentialID: j.RegistryCredential.ID}
}

// ServiceByName finds the web service with exactly this name (the API's name filter is a
// substring match).
func (a *RenderAPI) ServiceByName(ctx context.Context, name string) (RenderService, bool, error) {
	var found RenderService
	var ok bool
	q := url.Values{"name": {name}, "type": {"web_service"}}
	err := a.pages(ctx, "/services", q, "service", 20, func(r json.RawMessage) bool {
		var j renderServiceJSON
		if json.Unmarshal(r, &j) == nil && j.Name == name {
			found, ok = j.service(), true
		}
		return ok
	})
	return found, ok, err
}

// RenderEnv is an environment variable. Its String shows the key only.
type RenderEnv struct{ Key, Value string }

func (e RenderEnv) String() string   { return e.Key }
func (e RenderEnv) GoString() string { return "RenderEnv{" + e.Key + "}" }

// RenderCreate is what CreateService needs.
type RenderCreate struct {
	OwnerID, Name, Region, Plan, ImagePath, CredentialID string
	Instances                                            int
	Env                                                  []RenderEnv
}

type renderEnvJSON struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func envBody(env []RenderEnv) []renderEnvJSON {
	out := make([]renderEnvJSON, len(env))
	for i, e := range env {
		out[i] = renderEnvJSON{e.Key, e.Value}
	}
	return out
}

// CreateService creates an image-backed web service. Render starts a deploy of the image at
// once; its id is returned.
func (a *RenderAPI) CreateService(ctx context.Context, c RenderCreate) (RenderService, string, error) {
	image := map[string]any{"imagePath": c.ImagePath, "ownerId": c.OwnerID}
	if c.CredentialID != "" {
		image["registryCredentialId"] = c.CredentialID
	}
	body := map[string]any{
		"type": "web_service", "name": c.Name, "ownerId": c.OwnerID,
		"image": image,
		"serviceDetails": map[string]any{
			"plan": c.Plan, "region": c.Region, "numInstances": c.Instances,
			"healthCheckPath": "/_alder/health", "maxShutdownDelaySeconds": 30,
			"envSpecificDetails": map[string]any{},
		},
		"envVars": envBody(c.Env),
	}
	var out struct {
		Service  renderServiceJSON
		DeployID string
	}
	if err := a.do(ctx, "POST", "/services", nil, body, &out); err != nil {
		return RenderService{}, "", err
	}
	return out.Service.service(), out.DeployID, nil
}

// EnvVars is the service's environment, values included.
func (a *RenderAPI) EnvVars(ctx context.Context, serviceID string) ([]RenderEnv, error) {
	var out []RenderEnv
	err := a.pages(ctx, "/services/"+url.PathEscape(serviceID)+"/env-vars", nil, "envVar", 100, func(r json.RawMessage) bool {
		var e renderEnvJSON
		if json.Unmarshal(r, &e) == nil {
			out = append(out, RenderEnv{e.Key, e.Value})
		}
		return false
	})
	return out, err
}

// PutEnvVars replaces the service's whole environment. It starts no deploy.
func (a *RenderAPI) PutEnvVars(ctx context.Context, serviceID string, env []RenderEnv) error {
	return a.do(ctx, "PUT", "/services/"+url.PathEscape(serviceID)+"/env-vars", nil, envBody(env), nil)
}

// Scale sets the instance count.
func (a *RenderAPI) Scale(ctx context.Context, serviceID string, n int) error {
	return a.do(ctx, "POST", "/services/"+url.PathEscape(serviceID)+"/scale", nil, map[string]int{"numInstances": n}, nil)
}

// Credential finds a registry credential by exact name.
func (a *RenderAPI) Credential(ctx context.Context, name string) (string, bool, error) {
	var id string
	err := a.pages(ctx, "/registrycredentials", url.Values{"name": {name}}, "registryCredential", 20, func(r json.RawMessage) bool {
		var c struct{ ID, Name string }
		if json.Unmarshal(r, &c) == nil && c.Name == name {
			id = c.ID
		}
		return id != ""
	})
	return id, id != "", err
}

// CreateCredential stores a registry credential and returns its id. registry is Render's enum
// (GITHUB, DOCKER, GITLAB, GOOGLE_ARTIFACT, AWS_ECR).
func (a *RenderAPI) CreateCredential(ctx context.Context, ownerID, name, registry, user, token string) (string, error) {
	var out struct{ ID string }
	err := a.do(ctx, "POST", "/registrycredentials", nil, map[string]string{
		"name": name, "registry": registry, "username": user, "authToken": token, "ownerId": ownerID,
	}, &out)
	return out.ID, err
}

// RenderDeploy is a deploy. Status is Render's: created, queued, build_in_progress,
// update_in_progress, live, deactivated, build_failed, update_failed, pre_deploy_in_progress,
// pre_deploy_failed, canceled.
type RenderDeploy struct {
	ID, Status, ImageURL  string
	CreatedAt, FinishedAt time.Time
}

// Failed reports whether the status is a terminal failure.
func (d RenderDeploy) Failed() bool {
	switch d.Status {
	case "build_failed", "update_failed", "pre_deploy_failed", "canceled", "deactivated":
		return true
	}
	return false
}

type renderDeployJSON struct {
	ID, Status string
	Image      struct{ Ref string }
	CreatedAt  time.Time
	FinishedAt *time.Time
}

func (j renderDeployJSON) deploy() RenderDeploy {
	d := RenderDeploy{ID: j.ID, Status: j.Status, ImageURL: j.Image.Ref, CreatedAt: j.CreatedAt}
	if j.FinishedAt != nil {
		d.FinishedAt = *j.FinishedAt
	}
	return d
}

func deploysPath(serviceID string) string {
	return "/services/" + url.PathEscape(serviceID) + "/deploys"
}

// CreateDeploy triggers a deploy of imageURL ("<image>:<tag>").
func (a *RenderAPI) CreateDeploy(ctx context.Context, serviceID, imageURL string) (RenderDeploy, error) {
	var j renderDeployJSON
	err := a.do(ctx, "POST", deploysPath(serviceID), nil, map[string]string{"imageUrl": imageURL}, &j)
	return j.deploy(), err
}

// Deploy fetches one deploy.
func (a *RenderAPI) Deploy(ctx context.Context, serviceID, id string) (RenderDeploy, error) {
	var j renderDeployJSON
	err := a.do(ctx, "GET", deploysPath(serviceID)+"/"+url.PathEscape(id), nil, nil, &j)
	return j.deploy(), err
}

// Deploys lists the newest deploys, newest first.
func (a *RenderAPI) Deploys(ctx context.Context, serviceID string, limit int) ([]RenderDeploy, error) {
	var items []struct{ Deploy renderDeployJSON }
	if err := a.do(ctx, "GET", deploysPath(serviceID), url.Values{"limit": {strconv.Itoa(limit)}}, nil, &items); err != nil {
		return nil, err
	}
	out := make([]RenderDeploy, len(items))
	for i, it := range items {
		out[i] = it.Deploy.deploy()
	}
	return out, nil
}

// CancelDeploy cancels a deploy that has not finished.
func (a *RenderAPI) CancelDeploy(ctx context.Context, serviceID, id string) error {
	return a.do(ctx, "POST", deploysPath(serviceID)+"/"+url.PathEscape(id)+"/cancel", nil, nil, nil)
}

// RenderLog is one log line; Instance is the label "instance".
type RenderLog struct {
	Time              time.Time
	Instance, Message string
}

// Logs reads up to limit lines oldest first from start (zero: Render's default window), and
// returns the start time for the next call.
func (a *RenderAPI) Logs(ctx context.Context, ownerID, resource string, limit int, start time.Time) ([]RenderLog, time.Time, error) {
	q := url.Values{"ownerId": {ownerID}, "resource": {resource}, "limit": {strconv.Itoa(limit)}, "direction": {"forward"}}
	if !start.IsZero() {
		q.Set("startTime", start.UTC().Format(time.RFC3339Nano))
	}
	var out struct {
		NextStartTime *time.Time
		Logs          []struct {
			Message   string
			Timestamp time.Time
			Labels    []struct{ Name, Value string }
		}
	}
	if err := a.do(ctx, "GET", "/logs", q, nil, &out); err != nil {
		return nil, start, err
	}
	logs := make([]RenderLog, len(out.Logs))
	for i, l := range out.Logs {
		logs[i] = RenderLog{Time: l.Timestamp, Message: l.Message}
		for _, lb := range l.Labels {
			if lb.Name == "instance" {
				logs[i].Instance = lb.Value
			}
		}
	}
	next := start
	if out.NextStartTime != nil {
		next = *out.NextStartTime
	}
	return logs, next, nil
}

// LogsURL is the websocket URL for live logs. Dial it with AuthHeader.
func (a *RenderAPI) LogsURL(ownerID, resource string) string {
	base := a.Base
	switch {
	case strings.HasPrefix(base, "https://"):
		base = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	return base + "/logs/subscribe?" + url.Values{"ownerId": {ownerID}, "resource": {resource}}.Encode()
}
