### Round 4

```text
$ alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
  <button click={Create}>New order</button>
  <ul id="orders">
    {for o in model.orders ->
      <li>
        <a href={Orders.Show { id: o.id }}>Order</a>
        <span class="status">{Order.statusText o.status}</span>
        <span class="total">{Text.fromInt o.total}</span>
      </li>}
  </ul>
</Layout.Page>
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -38,7 +38,7 @@
     <ul id="orders">
       {for o in model.orders ->
         <li>
-          <a href={Orders.Index {}}>Order</a>
+          <a href={Orders.Show { id: o.id }}>Order</a>
           <span class="status">{Order.statusText o.status}</span>
           <span class="total">{Text.fromInt o.total}</span>
         </li>}
app/Orders/Index.alder:67:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Auth.Login - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF
alder fill: `Auth.Login` names no module of this app: write `Module.decl`, such as `Home.update`
[exit 1]
```

The harness stopped at line 16 of your script: `alder fill Auth.Login - --hole ?afterLogin <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Orders/Index.alder:67:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:31:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:40:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                         ^^^^^^^^^^^^^^^
app/Orders/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
