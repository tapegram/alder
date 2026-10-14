// This go.mod only keeps the app out of the Go module around it: `go vet ./...` and
// `go test ./...` there would otherwise compile adapters/, whose import of "app" (the
// generated package) resolves only in build/orders, where `alder build` copies it.
// `alder new` wrote it because it found that module's go.mod above this directory.
module orders

go 1.27
