((raw_element
  tag: (tag_name) @_tag
  (raw_text) @injection.content)
  (#eq? @_tag "script")
  (#set! injection.language "javascript"))

((raw_element
  tag: (tag_name) @_tag
  (raw_text) @injection.content)
  (#eq? @_tag "style")
  (#set! injection.language "css"))

((comment) @injection.content
  (#set! injection.language "comment"))
