; nvim-treesitter indent captures.
[
  (binding)
  (fake_entry)
  (test_decl)
  (type_decl)
  (record)
  (record_update)
  (block)
  (list)
  (record_type)
  (match_expr)
  (arm)
  (lambda)
  (if_expr)
  (fake_body)
  (deploy_record)
  (deploy_list)
  (silo_decl)
  (parenthesized_expression)
  (element)
  (html_for)
  (html_if)
  (html_match)
  (interpolation)
  (record_pattern)
  (list_pattern)
  ; an operator-led continuation line is one level in
  (pipe_expr)
  (default_expr)
  (or_expr)
  (and_expr)
  (comparison_expr)
  (additive_expr)
  (multiplicative_expr)
] @indent.begin

["}" "]" ")"] @indent.branch @indent.end
(end_tag) @indent.branch @indent.end
(if_expr "else" @indent.branch)
(html_if "else" @indent.branch)

(comment) @indent.auto
(ERROR) @indent.auto
