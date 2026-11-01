# alder-tui

A terminal UI for the Alder dev server. It lives in its own Go module (`alder/tui`) so the
Charm libraries never reach the `alder` binary or its `go.mod`. It drives the `alder` binary
and reads its JSON feeds; it never parses `alder`'s human-readable output.

```
alder-tui [--addr host:port] [--no-run] [--version] [dir]
```

It finds `alder` through `ALDER_BIN`, else `PATH`, and warns when the two versions differ:
install them together (`brew upgrade alder` installs both).

Build and test from this directory: `go build ./cmd/alder-tui`, `go test ./...`.
