package metrics

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
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

// FromEnv is the Collector ALDER_METRICS asks for: nil (no endpoint) when it is unset or empty,
// one answering every scrape when it is "open", else one answering only a scrape that carries
// the value (spaces trimmed) as its bearer token, which must be at least MinToken bytes. The
// error never contains the value.
func FromEnv() (*Collector, error) { return fromEnv(os.Getenv) }

func fromEnv(getenv func(string) string) (*Collector, error) {
	v := strings.TrimSpace(getenv(EnvVar))
	switch {
	case v == "":
		return nil, nil
	case v == "open":
		return New(""), nil
	case len(v) < MinToken:
		return nil, errToken
	}
	return New(v), nil
}

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
