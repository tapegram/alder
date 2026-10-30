; Minimal highlights for the tree-sitter CLI (`tree-sitter highlight`, the docs-site follow-up).
; Standard capture names. General first: when several patterns capture the same node, the CLI
; (tree-sitter-highlight) uses the last one, as Neovim does.
["module" "import" "as" "type" "brand" "where" "page" "msg" "form" "table" "capability" "fake"
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
(page_decl path: (string) @string.special)
(binding name: (_) @function)
(signature name: (_) @function)
(scratch_header "in" @keyword.import)
(watch ">" @punctuation.special)
