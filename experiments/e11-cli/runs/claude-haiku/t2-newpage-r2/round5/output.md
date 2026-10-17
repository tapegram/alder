I can't make progress on T2 with `alder` commands alone, so I'm not sending a script.

Every command that writes is refused while the app fails `alder check`, and the failures come from the generated admin code, not from the profile page:

1. **`Notice` (in `Admin/Todo/Bulk.alder` and `Admin/User/Bulk.alder`).** The generated code calls `Notice.with`, but no `Notice` module exists in the app, and it isn't in the standard library. Making one with `alder new decl` would still leave the `import Notice` lines missing, and no command adds imports.
2. **`<al-dialog>` (in `Admin/Todo/Show.alder` and `Admin/User/Show.alder`).** It isn't a built-in element (Appendix C), so it needs an entry in `elements/custom-elements.json`, which no command writes.

`alder regen admin --all` regenerates the same two problems, so it can't repair them either. Round 4 was empty, so nothing has been written since round 0.

To unblock, one of these has to happen outside `alder`:

- Hand-edit the two Bulk files (add a `Notice` module with `with`, plus its import) and the two Show files (remove the dialog markup, or add the element declaration), then rerun.
- Or, if the admin isn't wanted for this task, delete the `app/Admin/` directory by hand so `alder check` passes, and I'll continue with the profile page.

Tell me which you prefer, or whether I may edit files directly for this task. Once the app checks, I'll send the script for T2: the profile page, its `load` and rename logic, the Index link, and the tests.
