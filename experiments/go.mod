// This go.mod makes experiments/ a module of its own, so that Go files inside run output
// (an adapter copied into a round by --base, a model-emitted .go file) are not part of the
// alder module and never reach `go vet ./...` or `go test ./...` at the repository root.
// The harness is Python; nothing here is built with Go.
module alder/experiments

go 1.27
