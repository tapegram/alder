# todos

The to-do list the book tours in chapter 2: log in, add, toggle, edit and delete your own
todos. `alder check`, `alder build` and `alder test` take this directory; chapter 11 explains
each command.

## Mutation score

`alder test --mutate examples/todos` (book 10, "Mutation testing"): 33 mutants.

- Before: 9 tests, 18 killed, 15 survived (score 0.55).
- After: 18 tests, 33 killed, 0 survived (score 1.00). `TestExample_TodosMutation` fails if
  the score drops below 1.00 or the number of mutants is not 33.

The nine tests that closed the gaps, each named for what it kills:

- `App "Logout clears the user and goes to log in"`: logout's `Redirect → Rerender`. Every Log
  out button of this app is on a page that `requires user`, so in the browser a Rerender of that
  page also ends at the login page. Only a test of `App.update` itself tells the two apart.
- `Todo "countOpen ignores done todos"` (now two open todos and one done, so `not` dropped gives
  1, not 2), `Todo "Active keeps only open todos"` and `Todo "each filter has its label"`: the
  swapped `match` arms and the dropped `not` of `matches` and `filterLabel`.
- `Todo "a user with todos cannot be deleted"`: `owner @ref User` changed to `@cascade`.
- `Todos.Index "Delete removes the todo"`: the delete replaced by `Task.succeed ()`.
- `Todos.Index "the newest todo is listed first"`: `Db.orderBy` reversed.
- `Todos.Show "a visitor is sent to log in from a todo"`: `requires user` dropped.
- `Todos.Show "saving changes the title and the due date"`: the save replaced, each updated
  field dropped, and the redirect after it.
- `Todos.Show "deleting from the todo's page"`: that page's delete, and its redirect.

No mutant survives. The run also skips 10 mutants that do not apply (and refuses none as
provably equivalent); neither count is part of the score.
