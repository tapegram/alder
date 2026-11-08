; Minimal highlights for the tree-sitter CLI (`tree-sitter highlight`, the docs-site follow-up).
; Standard capture names. General first: when several patterns capture the same node, the CLI
; (tree-sitter-highlight) uses the last one, as Neovim does.
["module" "import" "as" "type" "brand" "where" "route" "form" "table" "capability" "fake"
 "session" "login" "test" "deploy" "theme" "silo" "choose" "requires" "role" "via" "let" "if" "then"
 "else" "match" "for" "in"] @keyword
(constructor) @constructor
[(integer) (float)] @number
(string) @string
(comment) @comment
(hole) @variable.builtin
(attribute_name) @attribute
(component_name (constructor) @tag)
(tag_name) @tag
(field_attribute) @attribute
(field_decl name: (identifier) @property)
(type_name (constructor) @type)
(route_decl path: (string) @string.special)
(route_method) @constructor
(binding name: (_) @function)
(signature name: (_) @function)
(qualified_decl_name route: (constructor) @module name: (identifier) @function)
(scratch_header "in" @keyword.import)
(watch ">" @punctuation.special)
