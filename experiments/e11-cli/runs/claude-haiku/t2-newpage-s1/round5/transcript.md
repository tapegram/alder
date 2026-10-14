### Round 5

```text
$ alder fill --decl Todos.Index.view - <<'EOF'
<Layout.Page title="Todos">
    <h1>Todos</h1>

    <al-card>
      <form submit={Submit}>
        <al-input field="title" label="What needs doing?" />
        <al-input field="due" label="Due" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>

    <nav>
      {for f in [All, Active, Completed] ->
        <a href={Todos.Index { filter: f }} aria-current={if model.filter == f then Just "page" else Nothing}>
          {Todo.filterLabel f}
        </a>}
      <a href={Profile {}}>Profile</a>
    </nav>

    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
    </ul>

    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF
edited app/Todos/Index.alder
--- a/app/Todos/Index.alder
+++ b/app/Todos/Index.alder
@@ -51,31 +51,32 @@
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Todos">
-    <h1>Todos</h1>
+      <h1>Todos</h1>
 
-    <al-card>
-      <form submit={Submit}>
-        <al-input field="title" label="What needs doing?" />
-        <al-input field="due" label="Due" />
-        <al-button type="submit">Add</al-button>
-      </form>
-    </al-card>
+      <al-card>
+        <form submit={Submit}>
+          <al-input field="title" label="What needs doing?" />
+          <al-input field="due" label="Due" />
+          <al-button type="submit">Add</al-button>
+        </form>
+      </al-card>
 
-    <nav>
-      {for f in [All, Active, Completed] ->
-        <a href={Todos.Index { filter: f }} aria-current={if model.filter == f then Just "page" else Nothing}>
-          {Todo.filterLabel f}
-        </a>}
-    </nav>
+      <nav>
+        {for f in [All, Active, Completed] ->
+          <a href={Todos.Index { filter: f }} aria-current={if model.filter == f then Just "page" else Nothing}>
+            {Todo.filterLabel f}
+          </a>}
+        <a href={Profile {}}>Profile</a>
+      </nav>
 
-    <ul id="todos">
-      {for todo in model.todos -> <Item todo={todo} />}
-    </ul>
+      <ul id="todos">
+        {for todo in model.todos -> <Item todo={todo} />}
+      </ul>
 
-    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>
+      <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>
 
-    <al-button variant="outline" click={App.Logout}>Log out</al-button>
-  </Layout.Page>
+      <al-button variant="outline" click={App.Logout}>Log out</al-button>
+    </Layout.Page>
 
 Item : { todo: Todo } -> Html Msg
 Item props =
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
