# alder-tui

A terminal UI for the Alder dev server. It lives in its own Go module (`alder/tui`) so the
Charm libraries never reach the `alder` binary or its `go.mod`. It drives the `alder` binary
and reads its JSON feeds; it never parses `alder`'s human-readable output.

```
alder-tui [--addr host:port] [--no-run] [--version] [dir]
```

It finds `alder` through `ALDER_BIN`, else `PATH`, and warns when the two versions differ:
install them together (`brew upgrade alder` installs both).

It attaches to the `alder run` already serving `dir` on the address (`--addr`, else
`ALDER_ADDR`, else `localhost:8080`), or starts one there and stops it when it quits; `--no-run`
only attaches. A dev server it starts never outlives it, even after `kill -9`: `alder run` gets
`ALDER_EXIT_ON_STDIN_EOF=1` and a pipe as its standard input.

## Screens

The top line is the status bar: the app, the build state, the address, alder's version and the
tabs. The bottom line is the footer: the current screen's keys, and on the right the real
command that prints the same data (`F` focuses it, then `y` copies it).

- **Requests** (`1`): the last minute's rate, p50, p95 and error share, and a table of the
  answered requests, newest at the bottom, with `n+1` and `slow` marks. The cursor follows the
  newest row until you move up.
- **Request detail** (`enter` on a request, in Requests or in Logs): its steps, its SQL (the
  statements of an N+1 folded into one row) and the lines it logged, from
  `GET /_dev/requests/<rid>`. Below 80 columns the sections stack.
- **Logs** (`2`): the app's log as `alder run` prints it, at a level you choose, filtered by a
  request id (from Requests' `l`) and a regexp; the strip under the status bar shows the
  filters in force.
- **Data**, **Tests**, **Routes** (`3`–`5`): not yet; the footer names the command to use
  meanwhile.

## Keys

Everywhere (except while a text input is open):

| Key | |
|---|---|
| `1`–`5`, `tab`, `shift-tab` | switch tabs |
| `p` | problems: the last build's diagnostics, or the app's exit (`enter` copies `file:line:col`) |
| `R` | restart the app (`POST /_dev/restart`) |
| `o` | open the app in the browser |
| `F` | focus the footer; then `y` copies its command |
| `?` | the keys of the current screen |
| `q` | quit |

Requests:

| Key | |
|---|---|
| `j`/`k`, arrows | move; moving up stops following, down onto the newest row follows again |
| `G` | follow the newest row again |
| `/` | filter the rows by their text (`enter` keeps it); `esc` clears it |
| `enter` | open the request |
| `l` | show the request's lines in Logs |

Request detail:

| Key | |
|---|---|
| `tab`, `shift-tab` | next or previous section (steps, SQL, logs) |
| `j`/`k`, arrows | move in the section |
| `enter` | unfold or fold the SQL row (each statement of an N+1 with its arguments) |
| `y` | copy the request id |
| `esc`, `left`, `backspace` | back |

Logs:

| Key | |
|---|---|
| `j`/`k`, arrows | move; moving up stops following, down onto the newest line follows again |
| `G` | follow the newest line again |
| `L` | next level: debug, info, warn, error (info at the start) |
| `/` | set the regexp (`enter` applies it; an invalid one shows its error and keeps the old one) |
| `x` | clear the request id and the regexp |
| `enter` | open the request of the line, if it has one |

## Developing

Build and test from this directory: `go build ./cmd/alder-tui`, `go test ./...`.

`TestLive_HelloAndFirstRequest` (`integration_test.go`) runs a real `alder run` on a copy of
`testdata/shelf`, a small app whose `/books/{id}` looks up each book's loan on its own (an N+1
on purpose), and checks the feed and the dev endpoints against it. It runs when `alder` is on
`PATH` or `ALDER_BIN` names it, and is skipped under `-short`. Build `alder` from the same
commit, since the test needs its dev feed:

```bash
go build -o "$TMPDIR/alder" ./cmd/alder        # from the repository root
cd tui && ALDER_BIN="$TMPDIR/alder" go test ./... -count=1 -race
```

CI's `tui` job does the same, and runs `alder check` and `alder test` in `testdata/shelf`.
