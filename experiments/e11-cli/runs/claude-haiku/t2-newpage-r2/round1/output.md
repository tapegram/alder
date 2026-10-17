The app's existing admin pages fail `alder check`, so the page generator refused to write. I'll regenerate the admin pages first, then retry the page.

```alder-cli
# the generated admin fails `alder check` (Notice not imported, al-dialog unknown): regenerate it from the generator
alder regen admin --all
# the app must check before a new page can be generated
alder check
# the profile page: requires a user, its Model, load and view left for filling
alder new page Profile /profile --requires user --blank
```
