# Survivors: reference-alt/t7-drafts

The three equivalent mutants of the blog (book 10, "Mutants that are the same program"); no input
can tell any of them from the app.

- `Posts.Index load:30:17 operator flipped` (`params.page < 1` → `<= 1`): the two differ only at page 1, where both branches give 1.
- `Posts.Index load:30:48 operator flipped` (`params.page > lastPage` → `>=`): the two differ only at `lastPage`, where both branches give `lastPage`.
- `Posts.Index load:38 limit dropped` (`Db.limit (perPage + 1)` → no limit): `load` keeps `List.take perPage` and sets `hasMore` from the length, so reading every post gives the same page, only slower.
