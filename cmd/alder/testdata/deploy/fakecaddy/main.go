// fakecaddy is Caddy as the Ssh target of `alder deploy` uses it, for the deploy tests
// (cmd/alder/deploy_caddy_test.go): the admin API (GET /config/…, GET /id/<id>, and the
// POST, PUT and PATCH calls that set a route) and a public listener that reverse-proxies every
// request to the upstream of the last route set. It logs every admin call to standard error.
// No TLS, no host matching, no persistence: a restart forgets the route, as a real Caddy
// restarted without --resume does.
//
//	fakecaddy <admin host:port> <public host:port>
package main

import (
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)

var (
	mu   sync.Mutex
	dial string
	ids  = map[string]bool{}
)

var (
	dialRe = regexp.MustCompile(`"dial":"([^"]+)"`)
	idRe   = regexp.MustCompile(`"@id":"([^"]+)"`)
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: fakecaddy <admin host:port> <public host:port>")
	}
	admin := http.NewServeMux()
	admin.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		log.Printf("admin %s %s %s", r.Method, r.URL.Path, b)
		if r.Method == http.MethodGet {
			if id, ok := strings.CutPrefix(r.URL.Path, "/id/"); ok {
				if ids[id] {
					w.Write([]byte("{}"))
					return
				}
				http.Error(w, "unknown id", http.StatusNotFound)
				return
			}
			w.Write([]byte("null"))
			return
		}
		if m := dialRe.FindSubmatch(b); m != nil {
			dial = string(m[1])
		}
		for _, m := range idRe.FindAllSubmatch(b, -1) {
			ids[string(m[1])] = true
		}
	})
	go func() { log.Fatal(http.ListenAndServe(os.Args[1], admin)) }()
	public := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		mu.Lock()
		d := dial
		mu.Unlock()
		u, _ := url.Parse("http://" + d)
		pr.SetURL(u)
		pr.Out.Host = pr.In.Host
	}}
	log.Fatal(http.ListenAndServe(os.Args[2], public))
}
