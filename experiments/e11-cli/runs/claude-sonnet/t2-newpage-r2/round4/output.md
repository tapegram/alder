I can't get past the check: every generator and `fill` refuses to run while the four errors in the generated Admin pages remain, and none of my commands can fix them. This script runs `alder check --json` to see exactly what the checker reports.

```alder-cli
alder check --json
```

The errors are in `Admin.Todo.Bulk`, `Admin.User.Bulk` (`Notice` not imported) and `Admin.Todo.Show`, `Admin.User.Show` (undeclared `<al-dialog>`). Only a command that is allowed to write despite errors can clear them. Once the check is clean, I'll add the Profile page, its tests and the link from the Index page.
