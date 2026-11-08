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

- **Data** (`3`): the app's tables, by their Alder names, each with its row count, and the
  migrations applied, on the left; the selected table's rows on the right, a page of 100 at a
  time.

  The tables come from `GET /_dev/map.json` and the rows from the database file under
  `build/<app>/`, opened read-only for each read.

  An app with silos has one database per silo
  value, and `d` switches between them.

  `:` takes a statement (one that reads: it must begin
  with `SELECT`, `WITH` or `EXPLAIN`) and shows its rows in place of the table's. The footer
  names `sqlite3 build/<app>/dev.db`, with the statement when there is one.

- **Tests** (`4`): runs `alder test --json` when the tab is first opened and shows a summary
  line and a row per test (module, name, time, `ok` or `FAIL`).

  `enter` on a failure unfolds
  its sides, note, error and trace; `enter` on the unfolded failure runs that test alone. `w`
  watches the app's files and reruns the suite on a change; `m` runs the mutation tests
  (`--mutate`) and adds the score and the surviving mutants.

  A run is never killed: `r` or a
  change during one queues a rerun for when it ends. The footer names the command of the last
  run.

- **Routes** (`5`): the app's routes from `alder routes --json`, one row each (page or msg, path,
  `module.name`, `file:line`, and how many requests it answered this session). `enter` shows
  that route's requests in Requests; `o` opens a page with no parameter in the browser. The
  footer names `alder routes --json`.

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

Data:

| Key | |
|---|---|
| `j`/`k`, arrows | move between tables (left) or rows (right) |
| `enter`, `l` | move into a table's rows |
| `h`/`l`, arrows | in the rows, move between columns |
| `enter` | in the rows, show the row vertically with long text unfolded; `esc`, `h`, `left`, `enter` or `backspace` go back |
| `esc` | in the rows, clear a query's result, else go back to the tables |
| `[` / `]` | previous or next page of rows |
| `s` | sort by the column under the cursor; again for descending |
| `d` | next database (only when the app has silos) |
| `:` | type a read-only SQL statement; `enter` runs it, `esc` cancels |

Tests:

| Key | |
|---|---|
| `j`/`k`, arrows | move; `j` scrolls a long unfolded failure before moving on |
| `enter` | unfold the failure; again, rerun that test alone |
| `r` | run the suite (queued behind a run in flight) |
| `w` | start or stop watching the app's files |
| `m` | run the mutation tests |

Routes:

| Key | |
|---|---|
| `j`/`k`, arrows | move |
| `enter` | show the route's requests in Requests |
| `o` | open the page in the browser (a page with no parameter) |

## Developing

Build and test from this directory: `go build ./cmd/alder-tui`, `go test ./...`.

`TestLive_HelloAndFirstRequest` and `TestLive_Screens` (`integration_test.go`) run a real
`alder run` on a copy of `testdata/shelf`, a small app whose `/books/{id}` looks up each book's
loan on its own (an N+1 on purpose).

The first checks the feed and the dev endpoints against it;
the second checks what the Tests, Routes and Data screens read: `alder test --json` has a passing
test, `alder routes --json` lists `/books/{id}`, and the SQL name the map gives the `Book` table
is in the database file.

It runs when `alder` is on
`PATH` or `ALDER_BIN` names it, and is skipped under `-short`.

Build `alder` from the same
commit, since the test needs its dev feed:

```bash
go build -o "$TMPDIR/alder" ./cmd/alder        # from the repository root
cd tui && ALDER_BIN="$TMPDIR/alder" go test ./... -count=1 -race
```

CI's `tui` job does the same, and runs `alder check` and `alder test` in `testdata/shelf`.
