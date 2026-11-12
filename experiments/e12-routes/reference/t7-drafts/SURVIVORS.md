# Survivors: reference/t7-drafts

The three equivalent mutants of the blog (book 10, "Mutants that are the same program"), as in
experiments/e9-feedback/reference/t7-drafts/SURVIVORS.md; no input can tell any of them from the app.

- `Posts.Index Index.load:33:17 operator flipped` (`params.page < 1` → `<= 1`): the two differ only at page 1, where both branches give 1.
- `Posts.Index Index.load:33:48 operator flipped` (`params.page > lastPage` → `>=`): the two differ only at `lastPage`, where both branches give `lastPage`.
- `Posts.Index Index.load:42 limit dropped` (`Db.limit (perPage + 1)` → no limit): `load` keeps `List.take perPage` and sets `hasMore` from the length, so reading every post gives the same page, only slower.
