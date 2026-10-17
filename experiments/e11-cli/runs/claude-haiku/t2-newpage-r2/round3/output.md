The admin code the app came with does not check, and every generator refuses to write to an app that doesn't check. So I can't add the profile page until the broken admin pages are removed. This script removes them, leaves first. If a removal is refused, the next transcript will show why.

```alder-cli
# the admin came with errors (Notice not imported, <al-dialog> unknown, Model fields missing), and
# every generator refuses an app that does not check: remove the broken admin pages, leaves first
alder remove page Admin.Todo.Bulk
alder remove page Admin.User.Bulk
alder remove page Admin.Todo.Delete
alder remove page Admin.User.Delete
alder remove page Admin.Todo.New
alder remove page Admin.User.New
alder remove page Admin.Todo.Edit
alder remove page Admin.User.Edit
alder remove page Admin.Todo.Show
alder remove page Admin.User.Show
alder remove page Admin.Todo.Index
alder remove page Admin.User.Index
alder remove page Admin.Index
alder check
```
