package rt

import "embed"

// Files is the runtime as the compiler ships it: every Go source file of the module's rt/ tree
// (not the tests, not testdata), the Datastar script with its VERSION, the `alder-ui` design
// system (al/: its stylesheet, its Custom Elements Manifest, which the checker reads through
// internal/elements, its VERSION, its Geist fonts, and al.go, which embeds the served files for rt/web), and the module files of the runtime's own module "alder"
// (dist/go.mod.txt, dist/go.sum.txt: a directory holding a file named go.mod would be another
// module, which go:embed refuses). buildkit.WriteRuntime writes it next to every generated app,
// so a build needs no checkout of the repository.
//
// The list is explicit because a pattern cannot leave the _test.go files out; a test
// (TestFiles_Complete) checks it against `git ls-files rt`, so a file added to the runtime
// without being listed here fails. This file itself is not listed: the written copy of package
// rt does not embed anything.
//
//go:embed core.go file.go nav.go show.go tether.go
//go:embed backup/bucket.go backup/lock_other.go backup/lock_unix.go backup/restore.go backup/s3.go backup/ship.go backup/silos.go backup/snapshot.go
//go:embed al/al.go al/al.css al/al.js al/custom-elements.json al/VERSION al/fonts/Geist-Variable.woff2 al/fonts/GeistMono-Variable.woff2 al/fonts/OFL.txt
//go:embed datastar/datastar.go datastar/datastar.js datastar/VERSION
//go:embed db/encode.go db/libsql.go db/migrate.go db/sqlite.go db/store.go db/stores.go db/stores_turso.go db/table.go db/turso.go
//go:embed eff/eff.go eff/run.go
//go:embed fuzz/brands.go fuzz/choices.go fuzz/combinators.go fuzz/derive.go fuzz/draw.go fuzz/literal.go fuzz/shrink.go
//go:embed html/diff.go html/html.go html/parse.go html/state.go
//go:embed selector/selector.go
//go:embed std/date.go std/dict.go std/effects.go std/file.go std/html.go std/list.go std/maybe.go std/num.go std/result.go std/text.go
//go:embed testing/browser.go testing/mutate.go testing/property.go testing/steps.go testing/testing.go
//go:embed web/decode.go web/request.go web/router.go web/devpanel.go web/devpanel_ui.go web/session.go
//go:embed dist/go.mod.txt dist/go.sum.txt
var Files embed.FS
