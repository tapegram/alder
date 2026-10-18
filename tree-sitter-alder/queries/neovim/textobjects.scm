; nvim-treesitter-textobjects captures.
(value_decl) @function.outer
(value_decl binding: (binding body: (_) @function.inner))
(fake_entry) @function.outer
(fake_entry body: (_) @function.inner)
(test_decl) @function.outer
(test_decl body: (_) @function.inner)
(lambda) @function.outer
(lambda body: (_) @function.inner)

[
  (type_decl)
  (table_decl)
  (capability_decl)
  (page_decl)
  (msg_decl)
  (form_decl)
] @class.outer
(type_decl body: (_) @class.inner)
(table_decl fields: (_) @class.inner)
(capability_decl entries: (_) @class.inner)
(form_decl fields: (_) @class.inner)
(page_decl params: (_) @class.inner)

(parameters (identifier) @parameter.inner @parameter.outer)
(lambda param: (identifier) @parameter.inner @parameter.outer)
(application argument: (_) @parameter.inner)

(arm) @block.outer
(arm body: (_) @block.inner)
(block) @block.outer
(block result: (_) @block.inner)

(comment) @comment.outer
