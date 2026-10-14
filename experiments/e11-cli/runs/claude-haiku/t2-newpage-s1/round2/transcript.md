### Round 2

```text
$ alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -6,7 +6,7 @@
 
 page "/profile" { } requires user
 
-type Model = { name: Text, email: Text, total: Int, done: Int }
+type Model = { name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
 
 load : Params -> Session -> Query Model
 load params session = {
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <p>{model.email}</p>
  <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
  <al-card>
    <h2 slot="header">Rename</h2>
    <form submit={SaveName}>
      <Errors form />
      <al-input field="name" label="Name" value={model.name} />
      <Errors field="name" />
      <al-button type="submit">Save name</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Todos</a>
</Layout.Page>
EOF
app/Profile.alder:47:9: error[E0028]: `<al-input field="name">` already shows the errors of `name`; remove `<Errors field="name" />`
          <Errors field="name" />
          ^^^^^^^^^^^^^^^^^^^^^^
app/Profile.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 6 of your script: `alder fill --decl Profile.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
