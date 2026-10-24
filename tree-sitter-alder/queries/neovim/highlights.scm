; tree-sitter-alder/queries/neovim/highlights.scm — nvim-treesitter capture conventions.
; General first: in Neovim a later pattern wins over an earlier one for the same range.

(identifier) @variable
(constructor) @constructor
(comment) @comment @spell

(string) @string
(integer) @number
(float) @number.float
(negative_number "-" @number)
(boolean) @boolean
(unit) @constant.builtin
(hole) @variable.builtin
(wildcard) @character.special

; ---- keywords
["module" "import"] @keyword.import
["type" "brand" "where"] @keyword.type
["page" "msg" "form" "table" "capability" "fake" "session" "login" "test" "deploy" "silo"
 "choose" "requires" "role" "via" "let"] @keyword
["if" "then" "else" "match"] @keyword.conditional
["for" "in"] @keyword.repeat
"\\" @keyword.function

; ---- operators and punctuation
["|>" "??" "||" "&&" "==" "!=" "<" "<=" ">" ">=" "+" "-" "++" "*" "/" "%" "->" "<-" "="
 "|" ".."] @operator
["(" ")" "[" "]" "{" "}"] @punctuation.bracket
["," ":" "."] @punctuation.delimiter

; ---- names in declarations
(signature name: (identifier) @function)
(signature name: (constructor) @function)
(binding name: (identifier) @function)
(binding name: (constructor) @function)
(fake_entry name: (identifier) @function)
(parameters (identifier) @variable.parameter)
(lambda param: (identifier) @variable.parameter)
(html_for binder: (identifier) @variable.parameter)

; ---- types
(type_var) @type
(type_name (constructor) @module)
(type_name (constructor) @type .)
(type_decl name: (constructor) @type.definition)
(variant name: (constructor) @constructor)
(brand_type base: (constructor) @type)

; ---- modules and qualified names
(module_name (constructor) @module)
(qualified_name module: (constructor) @module)
(qualified_name name: (identifier) @function.call)

; ---- calls and fields
(application function: (identifier) @function.call)
(field_access field: (identifier) @variable.member)
(application function: (field_access field: (identifier) @function.method.call))
(field_decl name: (identifier) @property)
(field_init name: (identifier) @property)
(field_pattern name: (identifier) @property)
(deploy_field name: (identifier) @property)
(deploy_target kind: (constructor) @constructor)

; ---- attributes on fields and tables
(field_attribute "@" @attribute)
(field_attribute name: (identifier) @attribute)
(field_attribute argument: (type_name (constructor) @type))
(field_attribute argument: (identifier) @property)

; ---- page, requires, silos
(page_decl path: (string) @string.special.url)
(session_key) @variable.member
(role_clause role: (constructor) @constructor)
(silo_clause key: (identifier) @variable.member)
(silo_decl key: (identifier) @variable.member)
(silo_decl table: (constructor) @type)
(choose_decl key: (identifier) @variable.member)

; ---- HTML
(tag_name) @tag
(component_name (constructor) @module)
(component_name (constructor) @tag .)
(attribute_name) @tag.attribute
(attribute "=" @operator)
(start_tag ["<" ">"] @tag.delimiter)
(end_tag ["</" ">"] @tag.delimiter)
(self_closing_element ["<" "/" ">"] @tag.delimiter)
(raw_element ["<" "</" ">" "/"] @tag.delimiter)
(html_text) @spell
(interpolation ["{" "}"] @punctuation.special)
(html_for ["{" "}"] @punctuation.special)
(html_if ["{" "}"] @punctuation.special)
(html_match . "{" @punctuation.special)
; anchors skip anonymous nodes, so the closing brace is the second of the two "}"
(html_match "}" "}" @punctuation.special)

; ---- scratch files: the `in Module` header and `> expr` watches
(scratch_header "in" @keyword.import)
(watch ">" @punctuation.special)
