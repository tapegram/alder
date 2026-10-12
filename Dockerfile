# The alder container image, ghcr.io/tapegram/alder:<version>: Go's official image plus the
# alder binary. It is for CI: `alder check`, `alder test` and `alder build` need Go on PATH
# (alder generates Go and runs go build), and this image has both.
#
#   docker run --rm -v "$PWD":/app -w /app ghcr.io/tapegram/alder:0.1.0 alder test
#
# GoReleaser builds it (.goreleaser.yaml, `dockers_v2:`) with the release's linux binary in the
# build context. The first build in a fresh container downloads modernc.org/sqlite from Go's
# module proxy; mount a module cache (GOMODCACHE, /go/pkg/mod) to keep later builds offline.
FROM golang:1.27

# GoReleaser lays the build context out as <os>/<arch>/alder, one per platform.
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/alder /usr/local/bin/alder

# Use the image's Go, never download another toolchain.
ENV GOTOOLCHAIN=local

WORKDIR /app
CMD ["alder", "version"]
