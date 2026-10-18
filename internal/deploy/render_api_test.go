package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"alder/internal/renderfake"
)

const testRenderKey = "rnd_test_key_do_not_leak"

func newRenderAPI(t *testing.T) (*RenderAPI, *renderfake.Server) {
	t.Helper()
	f := renderfake.New(t)
	return &RenderAPI{Base: f.URL, Key: testRenderKey}, f
}

// lastCall is the newest recorded call.
func lastCall(t *testing.T, f *renderfake.Server) renderfake.Call {
	t.Helper()
	c := f.Calls()
	if len(c) == 0 {
		t.Fatal("no requests")
	}
	return c[len(c)-1]
}

func TestRenderAPI_Owners(t *testing.T) {
	a, f := newRenderAPI(t)
	owners, err := a.Owners(context.Background())
	if err != nil || len(owners) != 1 || owners[0].ID != renderfake.OwnerID || owners[0].Type != "team" || owners[0].Name == "" {
		t.Fatalf("owners = %+v, %v", owners, err)
	}
	if got := f.Requests(); len(got) != 1 || !strings.HasPrefix(got[0], "GET /owners") {
		t.Fatalf("requests = %v", got)
	}
	if c := lastCall(t, f); c.Auth != "Bearer "+testRenderKey {
		t.Fatalf("auth header = %q", c.Auth)
	}
}

func TestRenderAPI_ServiceByName(t *testing.T) {
	a, f := newRenderAPI(t)
	ctx := context.Background()
	if _, ok, err := a.ServiceByName(ctx, "shop"); err != nil || ok {
		t.Fatalf("absent: ok=%v err=%v", ok, err)
	}
	f.AddService("shop-staging", "ghcr.io/o/r:b0", 1) // a substring match, not the name
	f.AddService("shop", "ghcr.io/o/r:b1", 2)
	svc, ok, err := a.ServiceByName(ctx, "shop")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := f.Service("shop")
	if svc.ID != want.ID || svc.Name != "shop" || svc.OwnerID != renderfake.OwnerID || svc.NumInstances != 2 ||
		svc.URL != f.URL+"/svc/shop" || svc.ImagePath != "ghcr.io/o/r:b1" || svc.Region == "" || svc.Plan == "" {
		t.Fatalf("service = %+v", svc)
	}
	c := lastCall(t, f)
	if c.Method != "GET" || c.Path != "/services" || c.Query.Get("name") != "shop" || c.Query.Get("type") != "web_service" || c.Query.Get("limit") != "20" {
		t.Fatalf("call = %+v", c)
	}
}

func TestRenderAPI_CreateService(t *testing.T) {
	a, f := newRenderAPI(t)
	f.Script("build_in_progress", "update_in_progress", "live")
	svc, dep, err := a.CreateService(context.Background(), RenderCreate{
		OwnerID: renderfake.OwnerID, Name: "shop", Region: "oregon", Plan: "starter",
		ImagePath: "ghcr.io/o/r:b1", CredentialID: "rcr-1", Instances: 2,
		Env: []RenderEnv{{"ALDER_DB", "libsql://x"}, {"ALDER_SECRET", "s3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := lastCall(t, f)
	if c.Method != "POST" || c.Path != "/services" {
		t.Fatalf("call = %+v", c)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(c.Body), &body); err != nil {
		t.Fatal(err)
	}
	sd := body["serviceDetails"].(map[string]any)
	img := body["image"].(map[string]any)
	if body["type"] != "web_service" || body["name"] != "shop" || body["ownerId"] != renderfake.OwnerID ||
		img["imagePath"] != "ghcr.io/o/r:b1" || img["registryCredentialId"] != "rcr-1" || img["ownerId"] != renderfake.OwnerID ||
		sd["plan"] != "starter" || sd["runtime"] != "image" || sd["region"] != "oregon" || sd["numInstances"] != float64(2) ||
		sd["healthCheckPath"] != "/_alder/health" || sd["maxShutdownDelaySeconds"] != float64(30) {
		t.Fatalf("body = %s", c.Body)
	}
	if _, ok := sd["envSpecificDetails"].(map[string]any); !ok {
		t.Fatalf("envSpecificDetails missing: %s", c.Body)
	}
	if ev := body["envVars"].([]any); len(ev) != 2 || ev[0].(map[string]any)["key"] != "ALDER_DB" || ev[0].(map[string]any)["value"] != "libsql://x" {
		t.Fatalf("envVars = %v", body["envVars"])
	}
	// The creation starts its own deploy of the image (R2).
	st := f.Service("shop")
	if st == nil || len(st.Deploys) != 1 || dep == "" || st.Deploys[0].ID != dep || st.Deploys[0].Image != "ghcr.io/o/r:b1" {
		t.Fatalf("state = %+v dep=%q", st, dep)
	}
	if svc.ID != st.ID || svc.URL != f.URL+"/svc/shop" || svc.NumInstances != 2 || svc.RegistryCredentialID != "rcr-1" {
		t.Fatalf("svc = %+v", svc)
	}
	if len(st.Env) != 2 {
		t.Fatalf("env = %v", st.Env)
	}
}

// The create body names the runtime (`image`: Render requires it when there is no build
// source) and leaves `plan` out when the declaration has none, so Render picks its default; the
// fake refuses a body without the runtime or with an empty plan, as Render does.
func TestRenderAPI_CreateServiceDefaultPlan(t *testing.T) {
	a, f := newRenderAPI(t)
	if _, _, err := a.CreateService(context.Background(), RenderCreate{
		OwnerID: renderfake.OwnerID, Name: "shop", ImagePath: "ghcr.io/o/r:b1", Instances: 1,
	}); err != nil {
		t.Fatal(err)
	}
	c := lastCall(t, f)
	var body struct {
		ServiceDetails map[string]any `json:"serviceDetails"`
	}
	if err := json.Unmarshal([]byte(c.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.ServiceDetails["runtime"] != "image" {
		t.Errorf("runtime = %v: %s", body.ServiceDetails["runtime"], c.Body)
	}
	if _, ok := body.ServiceDetails["plan"]; ok {
		t.Errorf("plan sent when the declaration has none: %s", c.Body)
	}
	if st := f.Service("shop"); st == nil || st.Plan != "starter" {
		t.Errorf("service = %+v", st)
	}
}

func TestRenderAPI_EnvVars(t *testing.T) {
	a, f := newRenderAPI(t)
	ctx := context.Background()
	svc := f.AddService("shop", "img:b1", 1, renderfake.Env{Key: "A", Value: "1"})
	env, err := a.EnvVars(ctx, svc.ID)
	if err != nil || len(env) != 1 || env[0] != (RenderEnv{"A", "1"}) {
		t.Fatalf("env = %v, %v", env, err)
	}
	if c := lastCall(t, f); c.Method != "GET" || c.Path != "/services/"+svc.ID+"/env-vars" {
		t.Fatalf("call = %+v", c)
	}
	before := len(f.Service("shop").Deploys)
	if err := a.PutEnvVars(ctx, svc.ID, []RenderEnv{{"B", "2"}, {"C", "3"}}); err != nil {
		t.Fatal(err)
	}
	c := lastCall(t, f)
	if c.Method != "PUT" || c.Path != "/services/"+svc.ID+"/env-vars" || c.Body != `[{"key":"B","value":"2"},{"key":"C","value":"3"}]` {
		t.Fatalf("call = %+v", c)
	}
	st := f.Service("shop")
	if len(st.Env) != 2 || st.Env[0].Key != "B" || len(st.Deploys) != before {
		t.Fatalf("state = %+v (a PUT replaces the set and starts no deploy)", st)
	}
}

func TestRenderAPI_EnvVarsPaged(t *testing.T) {
	a, f := newRenderAPI(t)
	var env []renderfake.Env
	for i := 0; i < 120; i++ {
		env = append(env, renderfake.Env{Key: fmt.Sprintf("K%03d", i), Value: "v"})
	}
	svc := f.AddService("shop", "img:b1", 1, env...)
	got, err := a.EnvVars(context.Background(), svc.ID)
	if err != nil || len(got) != 120 || got[119].Key != "K119" {
		t.Fatalf("got %d vars, err %v", len(got), err)
	}
}

func TestRenderAPI_Scale(t *testing.T) {
	a, f := newRenderAPI(t)
	svc := f.AddService("shop", "img:b1", 1)
	if err := a.Scale(context.Background(), svc.ID, 3); err != nil {
		t.Fatal(err)
	}
	c := lastCall(t, f)
	if c.Method != "POST" || c.Path != "/services/"+svc.ID+"/scale" || c.Body != `{"numInstances":3}` {
		t.Fatalf("call = %+v", c)
	}
	if f.Service("shop").Instances != 3 {
		t.Fatalf("instances = %d", f.Service("shop").Instances)
	}
}

func TestRenderAPI_Credentials(t *testing.T) {
	a, f := newRenderAPI(t)
	ctx := context.Background()
	if _, ok, err := a.Credential(ctx, "alder-o/r"); err != nil || ok {
		t.Fatalf("absent: ok=%v err=%v", ok, err)
	}
	if c := lastCall(t, f); c.Method != "GET" || c.Path != "/registrycredentials" || c.Query.Get("name") != "alder-o/r" {
		t.Fatalf("call = %+v", c)
	}
	id, err := a.CreateCredential(ctx, renderfake.OwnerID, "alder-o/r", "GITHUB", "octo", "ghp_token")
	if err != nil || id == "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	c := lastCall(t, f)
	if c.Method != "POST" || c.Path != "/registrycredentials" {
		t.Fatalf("call = %+v", c)
	}
	var body map[string]string
	json.Unmarshal([]byte(c.Body), &body)
	if body["name"] != "alder-o/r" || body["registry"] != "GITHUB" || body["username"] != "octo" || body["authToken"] != "ghp_token" || body["ownerId"] != renderfake.OwnerID {
		t.Fatalf("body = %s", c.Body)
	}
	got, ok, err := a.Credential(ctx, "alder-o/r")
	if err != nil || !ok || got != id {
		t.Fatalf("found = %q %v %v", got, ok, err)
	}
}

func TestRenderAPI_Deploys(t *testing.T) {
	a, f := newRenderAPI(t)
	ctx := context.Background()
	svc := f.AddService("shop", "img:b1", 2)
	f.Script("build_in_progress", "live")
	d, err := a.CreateDeploy(ctx, svc.ID, "img:b2")
	if err != nil || d.ID == "" || d.ImageURL != "img:b2" || d.Status != "created" {
		t.Fatalf("deploy = %+v, %v", d, err)
	}
	c := lastCall(t, f)
	if c.Method != "POST" || c.Path != "/services/"+svc.ID+"/deploys" || c.Body != `{"imageUrl":"img:b2"}` {
		t.Fatalf("call = %+v", c)
	}
	// One scripted step per GET.
	for i, want := range []string{"build_in_progress", "live", "live"} {
		got, err := a.Deploy(ctx, svc.ID, d.ID)
		if err != nil || got.Status != want {
			t.Fatalf("poll %d: %+v, %v (want %s)", i, got, err, want)
		}
		if c := lastCall(t, f); c.Method != "GET" || c.Path != "/services/"+svc.ID+"/deploys/"+d.ID {
			t.Fatalf("call = %+v", c)
		}
	}
	got, _ := a.Deploy(ctx, svc.ID, d.ID)
	if got.FinishedAt.IsZero() || got.CreatedAt.IsZero() || got.ImageURL != "img:b2" {
		t.Fatalf("live deploy = %+v", got)
	}
	if f.Service("shop").Image != "img:b2" {
		t.Fatalf("current image = %q", f.Service("shop").Image)
	}
	list, err := a.Deploys(ctx, svc.ID, 5)
	if err != nil || len(list) != 2 || list[0].ID != d.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if c := lastCall(t, f); c.Method != "GET" || c.Path != "/services/"+svc.ID+"/deploys" || c.Query.Get("limit") != "5" {
		t.Fatalf("call = %+v", c)
	}
}

func TestRenderAPI_CancelDeploy(t *testing.T) {
	a, f := newRenderAPI(t)
	ctx := context.Background()
	svc := f.AddService("shop", "img:b1", 1)
	f.Script("build_in_progress", "live")
	d, _ := a.CreateDeploy(ctx, svc.ID, "img:b2")
	if err := a.CancelDeploy(ctx, svc.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	if c := lastCall(t, f); c.Method != "POST" || c.Path != "/services/"+svc.ID+"/deploys/"+d.ID+"/cancel" {
		t.Fatalf("call = %+v", c)
	}
	got, _ := a.Deploy(ctx, svc.ID, d.ID)
	if got.Status != "canceled" {
		t.Fatalf("status = %s", got.Status)
	}
	if f.Service("shop").Image != "img:b1" {
		t.Fatal("a canceled deploy must not move the current image")
	}
}

func TestRenderAPI_Logs(t *testing.T) {
	a, f := newRenderAPI(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	f.Logs(
		renderfake.LogLine{Time: t0, Instance: "i-1", Message: "one"},
		renderfake.LogLine{Time: t0.Add(time.Second), Instance: "i-2", Message: "two"},
		renderfake.LogLine{Time: t0.Add(2 * time.Second), Instance: "i-1", Message: "three"},
	)
	logs, next, err := a.Logs(ctx, renderfake.OwnerID, "srv-1", 2, t0)
	if err != nil || len(logs) != 2 || logs[0].Message != "one" || logs[1].Instance != "i-2" || !logs[1].Time.Equal(t0.Add(time.Second)) {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
	c := lastCall(t, f)
	if c.Method != "GET" || c.Path != "/logs" || c.Query.Get("ownerId") != renderfake.OwnerID || c.Query.Get("resource") != "srv-1" ||
		c.Query.Get("limit") != "2" || c.Query.Get("direction") != "forward" || c.Query.Get("startTime") == "" {
		t.Fatalf("call = %+v", c)
	}
	more, next2, err := a.Logs(ctx, renderfake.OwnerID, "srv-1", 2, next)
	if err != nil || len(more) != 1 || more[0].Message != "three" {
		t.Fatalf("page 2 = %+v, %v", more, err)
	}
	none, next3, err := a.Logs(ctx, renderfake.OwnerID, "srv-1", 2, next2)
	if err != nil || len(none) != 0 || !next3.Equal(next2) {
		t.Fatalf("page 3 = %+v next %v vs %v, %v", none, next3, next2, err)
	}
}

func TestRenderAPI_LogsURL(t *testing.T) {
	a := &RenderAPI{Base: "https://api.render.com/v1", Key: testRenderKey}
	u := a.LogsURL("own-1", "srv-1")
	if u != "wss://api.render.com/v1/logs/subscribe?ownerId=own-1&resource=srv-1" {
		t.Fatalf("url = %s", u)
	}
	if strings.Contains(u, testRenderKey) {
		t.Fatal("key in url")
	}
	if b := (&RenderAPI{Base: "http://127.0.0.1:9"}).LogsURL("o", "r"); !strings.HasPrefix(b, "ws://127.0.0.1:9/logs/subscribe?") {
		t.Fatalf("url = %s", b)
	}
}

func TestRenderAPI_LogsSubscribe(t *testing.T) {
	a, f := newRenderAPI(t)
	t0 := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	f.Logs(renderfake.LogLine{Time: t0, Instance: "i-1", Message: "hello"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := DialWS(ctx, a.LogsURL(renderfake.OwnerID, "srv-1"), a.AuthHeader())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	msg, err := conn.Read()
	if err != nil || !strings.Contains(msg, `"message":"hello"`) || !strings.Contains(msg, `"instance"`) {
		t.Fatalf("msg = %q, %v", msg, err)
	}
	if c := lastCall(t, f); c.Path != "/logs/subscribe" || c.Auth != "Bearer "+testRenderKey {
		t.Fatalf("call = %+v", c)
	}
}

func TestRenderAPI_RetryAfter(t *testing.T) {
	a, f := newRenderAPI(t)
	var waits []time.Duration
	old := renderWait
	renderWait = func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	t.Cleanup(func() { renderWait = old })
	f.Fail429Once()
	owners, err := a.Owners(context.Background())
	if err != nil || len(owners) != 1 {
		t.Fatalf("owners = %v, %v", owners, err)
	}
	if len(waits) != 1 || waits[0] != time.Second {
		t.Fatalf("waits = %v", waits)
	}
	if got := f.Requests(); len(got) != 2 {
		t.Fatalf("requests = %v (one 429 and one retry)", got)
	}
}

func TestRenderAPI_RetryOnlyOnce(t *testing.T) {
	a, f := newRenderAPI(t)
	renderWaitStub(t)
	f.Fail429Once()
	f.Fail429Once() // two in a row: the second is returned
	_, err := a.Owners(context.Background())
	var ae *RenderAPIError
	if !errors.As(err, &ae) || ae.Status != 429 {
		t.Fatalf("err = %v", err)
	}
}

func renderWaitStub(t *testing.T) {
	old := renderWait
	renderWait = func(ctx context.Context, d time.Duration) error { return nil }
	t.Cleanup(func() { renderWait = old })
}

func TestRenderAPI_ErrorHidesKey(t *testing.T) {
	f := renderfake.New(t)
	f.RequireKey("the-real-key")
	a := &RenderAPI{Base: f.URL, Key: testRenderKey} // the fake echoes the header it was sent
	_, err := a.Owners(context.Background())
	var ae *RenderAPIError
	if !errors.As(err, &ae) || ae.Status != 401 {
		t.Fatalf("err = %v", err)
	}
	for _, s := range []string{err.Error(), ae.Message, fmt.Sprint(a), fmt.Sprintf("%+v %#v %v", a, a, *a)} {
		if strings.Contains(s, testRenderKey) {
			t.Fatalf("key leaked in %q", s)
		}
	}
	if !strings.HasPrefix(err.Error(), "Render API 401: ") {
		t.Fatalf("err = %q", err)
	}
}

func TestRenderAPI_EnvString(t *testing.T) {
	env := []RenderEnv{{"ALDER_SECRET", "hunter2"}}
	for _, s := range []string{fmt.Sprint(env), fmt.Sprintf("%v %+v %s", env, env[0], env[0])} {
		if strings.Contains(s, "hunter2") || !strings.Contains(s, "ALDER_SECRET") {
			t.Fatalf("string = %q", s)
		}
	}
}

func TestNewRenderAPI(t *testing.T) {
	t.Setenv("ALDER_RENDER_API", "")
	t.Setenv("RENDER_API_KEY", "k")
	a := NewRenderAPI()
	if a.Base != "https://api.render.com/v1" || a.Key != "k" || a.Client == nil {
		t.Fatalf("api = %#v", a)
	}
	t.Setenv("ALDER_RENDER_API", "http://127.0.0.1:1/")
	if NewRenderAPI().Base != "http://127.0.0.1:1" {
		t.Fatal("base not trimmed")
	}
}

func TestRenderAPI_Tail(t *testing.T) {
	a, f := newRenderAPI(t)
	t0 := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	var lines []renderfake.LogLine
	for i := 0; i < 10; i++ {
		lines = append(lines, renderfake.LogLine{Time: t0.Add(time.Duration(i) * time.Second), Instance: "i-1", Message: fmt.Sprintf("line %d", i)})
	}
	f.Logs(lines...)
	got, err := a.Tail(context.Background(), renderfake.OwnerID, "srv-1", 3)
	if err != nil || len(got) != 3 || got[0].Message != "line 7" || got[1].Message != "line 8" || got[2].Message != "line 9" {
		t.Fatalf("tail = %+v, %v", got, err)
	}
	c := lastCall(t, f)
	if c.Query.Get("direction") != "backward" || c.Query.Get("startTime") != "" || c.Query.Get("limit") != "3" {
		t.Fatalf("call = %+v", c)
	}
}

// stubRender serves canned bodies by request path+query-free path, for malformed answers.
func stubRender(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *RenderAPI {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h))
	t.Cleanup(srv.Close)
	return &RenderAPI{Base: srv.URL, Key: testRenderKey}
}

func TestRenderAPI_MalformedPages(t *testing.T) {
	ctx := context.Background()
	full := func(item string) string { // a full page of 100 owners, last one carrying `item`
		var b []string
		for i := 0; i < 99; i++ {
			b = append(b, fmt.Sprintf(`{"cursor":"c%d","owner":{"id":"o%d"}}`, i, i))
		}
		return "[" + strings.Join(append(b, item), ",") + "]"
	}
	for name, body := range map[string]string{
		"full page without cursor":  full(`{"owner":{"id":"x"}}`),
		"empty cursor":              full(`{"cursor":"","owner":{"id":"x"}}`),
		"item without its key":      `[{"cursor":"c0"}]`,
		"item that does not decode": `[{"cursor":"c0","owner":"nope"}]`,
	} {
		a := stubRender(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
		if owners, err := a.Owners(ctx); err == nil || !strings.Contains(err.Error(), "bad response") {
			t.Errorf("%s: owners=%d err=%v", name, len(owners), err)
		}
	}
	// the env-var list feeds a replace-all PUT: a bad item is an error, never a skipped variable
	a := stubRender(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"cursor":"c0","envVar":{"key":"A","value":"1"}},{"cursor":"c1"}]`)
	})
	if env, err := a.EnvVars(ctx, "srv-1"); err == nil {
		t.Errorf("env = %v, want an error", env)
	}
}

func TestRenderAPI_PagingPastFirstPage(t *testing.T) {
	ctx := context.Background()
	// 100 owners then 1 more: both pages are read.
	a := stubRender(t, func(w http.ResponseWriter, r *http.Request) {
		var b []string
		if r.URL.Query().Get("cursor") == "" {
			for i := 0; i < 100; i++ {
				b = append(b, fmt.Sprintf(`{"cursor":"c%d","owner":{"id":"o%d"}}`, i, i))
			}
		} else if r.URL.Query().Get("cursor") == "c99" {
			b = append(b, `{"cursor":"c100","owner":{"id":"o100"}}`)
		}
		io.WriteString(w, "["+strings.Join(b, ",")+"]")
	})
	owners, err := a.Owners(ctx)
	if err != nil || len(owners) != 101 || owners[100].ID != "o100" {
		t.Fatalf("owners = %d, %v", len(owners), err)
	}
	// a service past the first page of 20
	b, f := newRenderAPI(t)
	for i := 0; i < 25; i++ {
		f.AddService(fmt.Sprintf("shop-%02d", i), "img:b1", 1)
	}
	f.AddService("shop", "img:b2", 1)
	svc, ok, err := b.ServiceByName(ctx, "shop")
	if err != nil || !ok || svc.ImagePath != "img:b2" {
		t.Fatalf("svc = %+v ok=%v err=%v", svc, ok, err)
	}
	var withCursor int
	for _, c := range f.Calls() {
		if c.Query.Get("cursor") != "" {
			withCursor++
		}
	}
	if withCursor == 0 {
		t.Fatal("never followed a cursor")
	}
}

func TestRenderAPI_ErrorRedactsBeforeTruncating(t *testing.T) {
	key := testRenderKey
	// the key straddles byte 500; a multi-byte rune sits at the cut as well
	msg := strings.Repeat("a", 490) + key + strings.Repeat("é", 50)
	a := stubRender(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"message": msg})
	})
	_, err := a.Owners(context.Background())
	var ae *RenderAPIError
	if !errors.As(err, &ae) {
		t.Fatal(err)
	}
	if strings.Contains(ae.Message, key) || strings.Contains(ae.Message, key[:10]) {
		t.Fatalf("key leaked: %q", ae.Message)
	}
	if !utf8.ValidString(ae.Message) || !strings.HasSuffix(ae.Message, "...") {
		t.Fatalf("message = %q", ae.Message)
	}
}
