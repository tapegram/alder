### Round 3

```text
$ alder new decl Layout Shell "{ dark: Bool, title: Text, children: List (Html a) } -> Html a"
alder new decl: `Shell` is not a value name: a value is named by a lower-case identifier, such as `waiting`
[exit 1]
```

The harness stopped at line 2 of your script: `alder new decl Layout Shell "{ dark: Bool, title: Text, children: List (Html a) } -> Html a"`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
