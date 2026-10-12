# Survivors: reference-alt/t10-teams

One equivalent mutant; no input can tell it from the app.

- `Invites.Index load:21 where dropped` (`Db.where (Team.id.in …)` → every team): the query only narrows which teams are read; `teamName` looks each invite's team up by id, so reading every team shows the same names, only slower.
