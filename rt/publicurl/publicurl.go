// Package publicurl sets the served executable's public base (rt.PublicURL, Url.absolute's)
// from its environment while the program initialises. Only the generated main of a served app
// imports it: the test runner, alder eval and the REPL keep rt.DefaultPublicURL.
//
// Go initialises packages in import-path order, each once its imports are done, so this one
// ("alder/rt/publicurl", which imports only alder/rt) runs before package "app": a top-level
// constant of the app built with Url.absolute sees the configured base, not the default.
package publicurl

import (
	"os"

	"alder/rt"
)

// Err is a bad ALDER_PUBLIC_URL (rt.ParsePublicURL's error): main reports it and exits 1
// before it serves. rt.PublicURL then stays the default.
var Err error

// refine is set when the listen address has port 0: the base's port is the one Listen picks.
var refine bool

func init() {
	base, r, err := rt.ResolvePublicURL(os.Getenv)
	if err != nil {
		Err = err
		return
	}
	rt.PublicURL, refine = base, r
}

// Listened is called by main with the bound address once it listens: with a port-0 address and
// no ALDER_PUBLIC_URL the base becomes http:// and that address (rt.ListenPublicURL). A value
// computed before (a top-level constant) keeps port 0.
func Listened(addr string) {
	if refine {
		rt.PublicURL = rt.ListenPublicURL(addr)
	}
}
