package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// CookieName is the name of the cookie that carries the App session.
const CookieName = "alder_session"

// SessionCodec converts the App session record to and from the cookie payload. The generated
// code implements it for the App's session type with encoding/json: Default is the record with
// every field at its declared default, the value a request without a valid cookie sees.
//
// The cookie is signed, not encrypted: every session field is visible to the user (base64 of
// the JSON), so a Text.Secret value — or anything else the user must not read — must never be
// stored in the session.
type SessionCodec interface {
	Default() any
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
}

// Sign returns base64url(payload) + "." + base64url(HMAC-SHA256(key, payload)), unpadded.
func Sign(key, payload []byte) string {
	enc := base64.RawURLEncoding
	return enc.EncodeToString(payload) + "." + enc.EncodeToString(mac(key, payload))
}

// Verify checks a cookie made by Sign with key and returns its payload. It reports false for a
// malformed cookie or a signature that does not match (compared in constant time).
func Verify(key []byte, cookie string) ([]byte, bool) {
	p, s, ok := strings.Cut(cookie, ".")
	if !ok {
		return nil, false
	}
	enc := base64.RawURLEncoding
	payload, err := enc.DecodeString(p)
	if err != nil {
		return nil, false
	}
	sig, err := enc.DecodeString(s)
	if err != nil || !hmac.Equal(sig, mac(key, payload)) {
		return nil, false
	}
	return payload, true
}

func mac(key, payload []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(payload)
	return h.Sum(nil)
}

// minSecret is the shortest Options.Secret the router accepts; a shorter one (or none) means
// the development key.
const minSecret = 16

// devWarning is logged (level "warn") once by NewRouter when it falls back to DevKey.
const devWarning = "warning: ALDER_SECRET not set; using the development cookie key"

// DevKey is the fixed, public cookie key used when no secret (or one shorter than 16 bytes) is
// configured. Anyone can forge sessions signed with it: it is for development only.
func DevKey() []byte {
	return []byte("alder-development-cookie-key; not secret, never use in production")
}
