# Task T13: an activity feed that loads as it scrolls

Write a complete new Alder app (not the todo app) for an activity feed:

- There are no users and no login: anyone may read the feed and post to it.
- An entry has a text and the time it was created.
- `/feed` shows the ten newest entries, newest first, and a form to post
  an entry. A posted entry is stored, and the browser is sent back to
  `/feed`, where it is now the first entry.
- After the ten entries, the page has a sentinel: an element that, when it
  scrolls into view, asks for the next entries. Its answer shows the next
  ten entries, still newest first, and after them the next sentinel. The
  last entries have no sentinel after them, so scrolling stops asking.
  Every entry is shown once.
- An `App` module and a `Layout` module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/App.alder`: module `App`, with exactly `session { }` and no `login`.
- `app/Table/Entry.alder`: module `Table.Entry`, `type EntryId = brand Text`, and
  `table Entry { id: EntryId @id, text: Text.NonEmpty, created: Time @created }`.
- `app/Feed/Index.alder`: route module `Feed.Index`,
  `route Index = GET "/feed" { }`: the ten newest entries, newest first,
  each an `<li>` with the entry's text as its whole text, then the
  sentinel when more entries follow. `route More = GET "/feed/more" { after: Int }`,
  where `after` is the number of entries already shown: it answers the
  entries after the first `after` (the next ten, newest first, each an
  `<li>` as on `/feed`), then the next sentinel, which asks for `More`
  with the new number shown; none after the last entry. The first page's
  sentinel asks for `More { after: 10 }`. The sentinel is an element with
  `intersect=` (chapter 6, "Infinite scroll"), not an `<li>`, and is the
  only element with `intersect=`. Neither `/feed` nor `More`'s answer has
  any `<li>` but the entries' (no `<li>` for an empty feed or for "load
  more").
  `form NewEntry = { text: Text.NonEmpty }`, posted by
  `route Post form NewEntry`, which answers
  `Response.redirect (Feed.Index.Index {})`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test:
scrolling twice through 25 entries shows all 25, each once, newest first,
and leaves no sentinel; a feed of exactly ten entries has no sentinel;
posting an entry puts it first. `alder test` must pass.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
