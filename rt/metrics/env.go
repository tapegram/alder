package metrics

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// EnvVar is the variable FromEnv reads.
const EnvVar = "ALDER_METRICS"

// MinToken is the shortest token ALDER_METRICS accepts, in bytes.
const MinToken = 16

// errToken is FromEnv's refusal of a short token; it never holds the value.
var errToken = errors.New(`ALDER_METRICS: want "open" or a token of at least 16 characters`)

// DeployTargetEnv is set to "ssh" by an Ssh target's run script (alder deploy): the app is
// behind the box's proxy, which forwards every path.
const DeployTargetEnv = "ALDER_DEPLOY_TARGET"

// OpenWarning is the line the generated main logs at warn when ALDER_METRICS=open is accepted.
const OpenWarning = "ALDER_METRICS=open: GET /_alder/metrics answers anyone, unauthenticated (route patterns, step and table names, the build id, traffic): keep the port private or set a token"

// FromEnv is the Collector ALDER_METRICS asks for: nil (no endpoint) when it is unset or empty,
// one answering every scrape when it is "open", else one answering only a scrape that carries
// the value (spaces trimmed) as its bearer token, which must be at least MinToken bytes. The
// error never contains the value.
//
// "open" is refused on a deploy target whose one port is public: Render (RENDER set), Fly
// (FLY_APP_NAME set: its proxy answers https://<app>.fly.dev/_alder/metrics) and a box deployed
// by alder (ALDER_DEPLOY_TARGET=ssh: Caddy forwards every path). An open endpoint there would
// show anyone every route pattern (unlinked admin pages too), table names through step names,
// the build id and the traffic.
func FromEnv() (*Collector, error) { return fromEnv(os.Getenv) }

func fromEnv(getenv func(string) string) (*Collector, error) {
	v := strings.TrimSpace(getenv(EnvVar))
	switch {
	case v == "":
		return nil, nil
	case v == "open":
		if where := publicTarget(getenv); where != "" {
			return nil, fmt.Errorf(`ALDER_METRICS=open on %s: the endpoint would be public (route patterns, step and table names, the build id, traffic): set a token of at least 16 characters instead`, where)
		}
		return New(""), nil
	case len(v) < MinToken:
		return nil, errToken
	}
	return New(v), nil
}

// publicTarget names the deploy target the process runs on when its one port is public, else "".
func publicTarget(getenv func(string) string) string {
	switch {
	case getenv("RENDER") != "":
		return "Render"
	case getenv("FLY_APP_NAME") != "":
		return "Fly"
	case getenv(DeployTargetEnv) == "ssh":
		return "a box deployed by alder"
	}
	return ""
}

// Open reports whether c answers every scrape (ALDER_METRICS=open).
func (c *Collector) Open() bool { return c.auth.open }

// authorizer keeps a token's SHA-256 digest, never the token: a Collector printed with %v or
// %+v, or marshalled, shows no secret.
type authorizer struct {
	open   bool
	digest [sha256.Size]byte
}

func newAuthorizer(token string) authorizer {
	if token == "" {
		return authorizer{open: true}
	}
	return authorizer{digest: sha256.Sum256([]byte(token))}
}

// Authorized reports whether r may read the metrics: always for an open Collector, else when it
// carries `Authorization: Bearer <token>` (the scheme in any case). The comparison is of the
// SHA-256 digests in constant time: neither the token's bytes nor its length show in the timing.
func (c *Collector) Authorized(r *http.Request) bool {
	if c.auth.open {
		return true
	}
	h := r.Header.Get("Authorization")
	scheme, tok, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(tok)))
	return subtle.ConstantTimeCompare(got[:], c.auth.digest[:]) == 1
}
