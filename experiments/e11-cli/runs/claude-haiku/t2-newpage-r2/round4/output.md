I can't finish this with `alder` commands from the current state. Every command that writes refuses while the app has errors, and the errors are in the admin scaffold that `alder new admin` generated, which I can't fix with the commands I have:

1. **`Notice` is not imported** in `app/Admin/Todo/Bulk.alder` and `app/Admin/User/Bulk.alder`. Fixing it needs an `import` line. `alder fill` on `update` would replace the body, but it refuses while the app has errors.
2. **`<al-dialog>` is unknown** in `app/Admin/Todo/Show.alder` and `app/Admin/User/Show.alder`. The built-in `alder-ui` manifest has no `al-dialog`, and declaring one needs `elements/custom-elements.json`, which no command writes.
3. **`alder regen admin --all` regenerates the same errors**, so it can't clear them.
4. **`alder remove page`** on the broken Bulk or Show pages is refused because the app has errors, and removing a page that is linked from elsewhere is refused anyway.

The guard refuses every command until the app checks, and nothing I can run changes the app to a checking state. The profile page needs a working app to generate into, so I won't send a script of commands that would all fail at the first step.

To unblock this, one of these needs to happen outside `alder`:

- add `import Notice` (or remove the `Notice.with` calls) in both Bulk modules, and
- remove the `<al-dialog>` blocks from both Show pages, or declare `al-dialog` in `elements/custom-elements.json`.

Once `alder check` passes, the profile task is a short script: `alder new page Profile /profile --requires user --blank`, then `alder new form` for the name change with a `Text.NonEmpty` field, a `msg` for logout (reusing `App.Logout`), the counts in `load` using `Db.mine`, the link from the Index page, and the tests. I'll send that script once the app checks.

```alder-cli
# Blocked: the app does not check (see the explanation above), so every command here would be refused.
# Nothing is run. Fix the admin scaffold first, then send the profile script.
```
