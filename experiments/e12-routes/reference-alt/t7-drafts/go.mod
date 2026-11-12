// This go.mod only keeps the example app out of the alder module: `go vet ./...` and
// `go test ./...` at the repository root would otherwise compile adapters/, whose import of
// "app" (the generated package) resolves only in build/blog, where `alder build` copies it.
module blog

go 1.27
