# Task T1: add a priority to todos

Modify the example app so that every todo has a priority.

- Add `type Priority = Low | Normal | High` to the `Todo` module.
- Add a `priority` field to the `Todo` table, defaulting to `Normal`.
- Let the user choose a priority when creating a todo on the Index page
  and when editing on the Show page.
- Show the priority next to each todo in the list, and sort the list so
  High comes first, then Normal, then Low (newest first within a priority).

Output every file you change, in full, each preceded by a line
`--- file: app/<path> ---`. Do not output files you did not change. No
explanation, only files.
