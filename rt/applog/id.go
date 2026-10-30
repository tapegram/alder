package applog

import (
	"crypto/rand"
	"encoding/binary"
	"net/http"
	"strconv"
	"sync/atomic"
)

// idSeed is the process's random start for request ids, read from crypto/rand once.
var idSeed = func() uint32 {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint32(b[:])
}()

var idCount atomic.Uint32

// NewID is a request id: 8 lowercase hex characters, the process's random seed plus a counter.
// Unique within a process (until 2^32 requests), cheap, and not a secret.
func NewID() string {
	n := idSeed + idCount.Add(1)
	var b [8]byte
	s := strconv.AppendUint(b[:0], uint64(n), 16)
	if len(s) < 8 {
		var p [8]byte
		k := copy(p[8-len(s):], s)
		for i := 0; i < 8-k; i++ {
			p[i] = '0'
		}
		return string(p[:])
	}
	return string(s)
}

// IDHeader is the request and response header carrying the request id.
const IDHeader = "X-Request-Id"

// RequestID is r's id: its X-Request-Id when that is 1 to 64 bytes of [A-Za-z0-9._-] (a
// platform's own id, which then ties its log to the app's), else a new one (NewID). Anything else
// is replaced, never echoed.
func RequestID(r *http.Request) string {
	if v := r.Header.Get(IDHeader); validID(v) {
		return v
	}
	return NewID()
}

func validID(v string) bool {
	if len(v) < 1 || len(v) > 64 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
