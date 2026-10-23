package deploy

import (
	"fmt"
	"strconv"
	"strings"
)

// The files `alder deploy init` writes into an app (chapter 16), for a platform that builds the
// image itself from the repository: a Dockerfile that builds alder from a pinned ref and the
// app with it, a .dockerignore, and the platform's own file. The fly.toml and the .dockerignore
// are the same templates the Fly target writes into its image build context (writeContext).

// InitRefPlaceholder stands in for alder's ref when the compiler records none.
const InitRefPlaceholder = "REPLACE_WITH_ALDER_REF"

// Dockerignore keeps the platform file and local state out of the image build.
const Dockerignore = "fly.toml\nrender.yaml\nbuild/\n.git/\n*.db\n*.db-shm\n*.db-wal\n"

// InitSpec says what to write: Target "render" or "fly", the app's name (its directory's: alder
// names the binary after it), alder's git Ref ("" for the marked placeholder), and whether the
// app has tables (Disk: a persistent disk or volume for the SQLite file).
type InitSpec struct {
	Target string
	App    string
	Ref    string
	Disk   bool
}

// InitFile is one file to write.
type InitFile struct{ Name, Body string }

// InitDataDir is where the disk or volume is mounted, and the database lives in it.
const (
	InitDataDir = "/data"
	InitDB      = InitDataDir + "/app.db"
	initPort    = 10000
)

// InitFiles are the files `alder deploy init <target>` writes.
func InitFiles(s InitSpec) ([]InitFile, error) {
	if s.Ref == "" {
		s.Ref = InitRefPlaceholder
	}
	files := []InitFile{{"Dockerfile", initDockerfile(s)}, {".dockerignore", Dockerignore}}
	switch s.Target {
	case "render":
		files = append(files, InitFile{"render.yaml", renderYAML(s)})
	case "fly":
		files = append(files, InitFile{"fly.toml", FlyToml(s.App, "", true, s.Disk)})
	default:
		return nil, fmt.Errorf("unknown target %q: render or fly", s.Target)
	}
	return files, nil
}

func initDockerfile(s InitSpec) string {
	mark := ""
	if s.Ref == InitRefPlaceholder {
		mark = "# REPLACE the ref below: this alder records no version of itself. Use a release tag or a\n# full commit of https://github.com/tapegram/alder.\n"
	}
	return `# Written by alder deploy init (book chapter 16): ` + s.App + ` on ` + map[string]string{"render": "Render", "fly": "Fly"}[s.Target] + `'s Docker runtime.
#
# Alder is built from source at a pinned ref. To upgrade alder, change ALDER_REF (a release tag or a
# full commit).
` + mark + `FROM golang:1.27 AS build
ARG ALDER_REF=` + s.Ref + `
ENV GOTOOLCHAIN=local CGO_ENABLED=0
RUN git init /alder && cd /alder \
 && git remote add origin https://github.com/tapegram/alder.git \
 && git fetch --depth 1 origin "$ALDER_REF" && git checkout FETCH_HEAD \
 && go build -o /usr/local/bin/alder ./cmd/alder
# alder names the binary after the directory: build/` + s.App + `/` + s.App + `
WORKDIR /` + s.App + `
COPY . .
RUN alder build

FROM gcr.io/distroless/static-debian12
COPY --from=build /` + s.App + `/build/` + s.App + `/` + s.App + ` /app/` + s.App + `
COPY --from=build /` + s.App + `/build/` + s.App + `/static /app/static
WORKDIR /app
ENTRYPOINT ["/app/` + s.App + `"]
`
}

func renderYAML(s InitSpec) string {
	var b strings.Builder
	b.WriteString("# Written by alder deploy init (book chapter 16): " + s.App + " as one Docker web service.\n")
	b.WriteString("# Commit it, then in Render: New > Blueprint, and connect this repository.\n")
	b.WriteString("services:\n  - type: web\n    name: " + s.App + "\n    runtime: docker\n")
	if s.Disk {
		b.WriteString("    plan: starter # a persistent disk needs a paid plan\n")
	} else {
		b.WriteString("    plan: starter\n")
	}
	b.WriteString("    branch: main\n    dockerfilePath: ./Dockerfile\n    healthCheckPath: /_alder/health\n    autoDeployTrigger: commit\n")
	b.WriteString("    envVars:\n")
	b.WriteString("      - key: ALDER_ADDR\n        value: \":" + strconv.Itoa(initPort) + "\"\n")
	b.WriteString("      - key: PORT\n        value: \"" + strconv.Itoa(initPort) + "\"\n")
	b.WriteString("      - key: ALDER_SECRET\n        generateValue: true\n")
	if s.Disk {
		b.WriteString("      - key: ALDER_DB\n        value: " + InitDB + "\n")
	}
	b.WriteString("      - key: TZ\n        value: UTC\n")
	if s.Disk {
		b.WriteString("    # SQLite lives on this disk. A disk means one instance and a short pause on each deploy\n")
		b.WriteString("    # (chapter 16, \"Why no disk mode\").\n")
		b.WriteString("    disk:\n      name: data\n      mountPath: " + InitDataDir + "\n      sizeGB: 1\n")
	}
	return b.String()
}

// FlyToml is a fly.toml of flyApp. With serve false it is the stub the Fly target writes into
// its image build context (the app and region, the Dockerfile: the machine itself is made with
// the Machines API); with serve true it is the file `alder deploy init fly` writes: the
// service on FlyPort with its health check and, when volume, the volume SQLite lives on.
func FlyToml(flyApp, region string, serve, volume bool) string {
	var b strings.Builder
	if !serve {
		b.WriteString("# Written by alder deploy for the image build only; the machine is made with the Machines API.\n")
	} else {
		b.WriteString("# Written by alder deploy init (book chapter 16). Then: fly launch --no-deploy --copy-config,\n")
		b.WriteString("# fly secrets set ALDER_SECRET=$(openssl rand -hex 32)")
		if volume {
			b.WriteString(", fly volumes create data --size 1")
		}
		b.WriteString(", fly deploy.\n")
	}
	b.WriteString("app = " + strconv.Quote(flyApp) + "\n")
	if region != "" {
		b.WriteString("primary_region = " + strconv.Quote(region) + "\n")
	}
	b.WriteString("\n[build]\n  dockerfile = \"Dockerfile\"\n")
	if !serve {
		return b.String()
	}
	b.WriteString("\n[env]\n  ALDER_ADDR = \":" + strconv.Itoa(FlyPort) + "\"\n")
	if volume {
		b.WriteString("  ALDER_DB = \"" + InitDB + "\"\n")
	}
	b.WriteString("\n[http_service]\n  internal_port = " + strconv.Itoa(FlyPort) + "\n  force_https = true\n  auto_stop_machines = \"off\"\n  min_machines_running = 1\n")
	b.WriteString("\n  [[http_service.checks]]\n    method = \"GET\"\n    path = \"/_alder/health\"\n    interval = \"15s\"\n    timeout = \"5s\"\n    grace_period = \"10s\"\n")
	if volume {
		b.WriteString("\n[mounts]\n  source = \"data\"\n  destination = \"" + InitDataDir + "\"\n")
	}
	return b.String()
}
