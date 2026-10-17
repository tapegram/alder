```alder-cli
# Remove the broken, unreferenced Bulk pages first (they use the missing Notice module).
alder remove page Admin.Todo.Bulk
alder remove page Admin.User.Bulk
# Then try the Show pages that use the undeclared <al-dialog>.
alder remove page Admin.Todo.Show
alder remove page Admin.User.Show
```
