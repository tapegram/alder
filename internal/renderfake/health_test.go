package renderfake

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func do(t *testing.T, method, url, body string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer k")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestHealthRoundRobinAndDelayedSwitch(t *testing.T) {
	f := New(t)
	svc := f.AddService("shop", "ghcr.io/o/r:b1", 2)
	url := f.URL + "/svc/shop/_alder/health"
	for i := 0; i < 4; i++ {
		code, body := get(t, url)
		if code != 200 || body != `{"build":"b1","db":"ok"}` {
			t.Fatalf("%d %s", code, body)
		}
	}
	f.Script("live")
	f.DelaySwitch("shop", 1)
	do(t, "POST", f.URL+"/services/"+svc.ID+"/deploys", `{"imageUrl":"ghcr.io/o/r:b2"}`)
	ds := f.Service("shop").Deploys
	do(t, "GET", f.URL+"/services/"+svc.ID+"/deploys/"+ds[len(ds)-1].ID, "") // live: instance 0 moves, 1 is held
	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		_, body := get(t, url)
		seen[body]++
	}
	if seen[`{"build":"b2","db":"ok"}`] != 2 || seen[`{"build":"b1","db":"ok"}`] != 2 {
		t.Fatalf("seen = %v", seen)
	}
	f.ReleaseSwitch("shop")
	for i := 0; i < 4; i++ {
		if _, body := get(t, url); body != `{"build":"b2","db":"ok"}` {
			t.Fatalf("after release: %s", body)
		}
	}
	if f.HealthHits("shop") != 12 {
		t.Fatalf("hits = %d", f.HealthHits("shop"))
	}
}

func TestHealthUnknownService(t *testing.T) {
	f := New(t)
	if code, _ := get(t, f.URL+"/svc/nope/_alder/health"); code != 404 {
		t.Fatalf("unknown service: %d", code)
	}
}
