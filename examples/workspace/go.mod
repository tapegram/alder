// This go.mod only keeps the example app out of the alder module: `go vet ./...` and
// `go test ./...` at the repository root would otherwise compile adapters/, whose import of
// "app" (the generated package) resolves only in build/workspace, where `alder build` copies it.
module workspace

go 1.27
