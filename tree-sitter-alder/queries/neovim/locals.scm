; Scopes, definitions and references (nvim-treesitter locals).
(block) @local.scope
(lambda) @local.scope
(arm) @local.scope
(html_for) @local.scope
(value_decl) @local.scope
(fake_entry) @local.scope

(let_stmt name: (identifier) @local.definition.var)
(bind_stmt name: (identifier) @local.definition.var)
(parameters (identifier) @local.definition.parameter)
(lambda param: (identifier) @local.definition.parameter)
(html_for binder: (identifier) @local.definition.var)
(arm pattern: (identifier) @local.definition.var)
(constructor_pattern argument: (identifier) @local.definition.var)
(field_pattern pattern: (identifier) @local.definition.var)
(list_pattern (identifier) @local.definition.var)
(rest_pattern name: (identifier) @local.definition.var)

(identifier) @local.reference
