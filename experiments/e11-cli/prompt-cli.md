====================  HOW TO ANSWER: ONE SCRIPT OF `alder` COMMANDS  ====================

You build the app by running `alder` commands (the generators, `alder edit`, `alder remove`,
`alder fill`; the reference's command table lists them). You never write or show a file: where the
task says to output files, send a script instead; the files are what your commands write.

Your answer is ONE fenced block tagged `alder-cli` (a block tagged `sh` is accepted too) holding the
script. Text outside the block is ignored; a second script block is refused.

- One command per line, each starting with `alder`. A line starting with `#` is a comment; blank
  lines are skipped. Nothing else is a command: no `cd`, `cat`, `echo`, `mkdir`, no pipes, `&&`,
  `>` or shell variables. A line that is not an `alder` command is refused.
- Quote an argument holding spaces in '…' or "…"; the quotes are taken as they are, with no escapes
  inside (`alder fill 'Member."row is the row of the user in that team"' -`).
- A command that reads its standard input (`alder fill <address> -`) takes it from a here-document:
  end the command's line with `<<'EOF'`, put the body on the lines after it, and end it with a line
  that is exactly `EOF`.
- The commands run in order, in the app's directory: the harness created it with
  `alder new <app> --bare` (or it is the app shown after this page, when there is one). Do not pass
  a directory and do not run `alder new <app>`.
- `--json` is allowed on any command that has it; you see its output. `alder check`, `alder test`
  and `alder eval` may be run mid-script. `alder run` (a server) is not allowed. At most 80 commands
  a round.
- The harness stops at the first command that fails (exits non-zero) and shows you everything:
  every command it ran, its output, and the failure in full. Then it runs `alder check`,
  `alder build` and `alder test` on the app. If every command ran and those pass, you are done.
  Otherwise your next prompt carries that transcript, and you send the next script, continuing from
  the app as the commands left it (do not repeat the commands that succeeded).

Example: the first ten commands of another app (its tables, a declaration and its test):

```alder-cli
# the users, and logging in
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new auth --users User

# the tables
alder new table Team name:Text.NonEmpty owner:User.UserId@ref=User --no-timestamps
alder new table Member team:Team.TeamId@ref=Team user:User.UserId@ref=User --no-timestamps
alder new type Invite "Status = Pending | Accepted | Declined"
alder new table Invite team:Team.TeamId@ref=Team email:Text.Email status:Status=Pending --no-timestamps

# a user's Member row in a team
alder new decl Member row "Team.TeamId -> User.UserId -> Query (Maybe Member)"
alder fill Member.row - <<'EOF'
\team user -> Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first
EOF
alder new test Member "row is the row of the user in that team"
alder fill 'Member."row is the row of the user in that team"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let _ <- Db.insert Member { team: blue, user: bob }
  let _ <- Db.insert Member { team: red, user: ann }
  let want <- Db.insert Member { team: red, user: bob }
  let found <- row red bob
  Task.succeed (Maybe.map (\m -> m.id) found == Just want)
}
EOF
```

The example shows the format, not your task's solution: use your task's names.
