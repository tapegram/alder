### Round 0

```text
$ alder new type Slug 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
wrote app/Slug.alder
[exit 0]
$ alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False
app/Post.alder:8:9: error[E0201]: module `Slug` is not imported
    slug: Slug @unique,
          ^^^^
  fix: import Slug

alder new table: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 2 of your script: `alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
