# Markdown style for Alder docs

The book (`book/*.md`) and the other docs (`README.md`, the `examples/`
READMEs, `tui/`, `tree-sitter-alder/`, the scaffold's README) are read on
GitHub today. Later they will feed a docs site written in Alder. So write
for the rendered page, and keep the structure semantic: headings,
lists and tables rather than visual tricks.

The goal is pages that scan. A reader should see the shape of a section
before reading a sentence of it.

## Paragraphs

- Keep a paragraph to one idea: about three sentences, or five wrapped
  lines. Split where the topic shifts.
- Lead with the rule, then give the detail. Put the exception in the next
  paragraph, not in a parenthesis inside the rule.
- Wrap prose at about 80 columns. Never leave a single line far past it.
  An inline link can push one line over; that is fine.

## Headings

- Give each topic its own heading. A `##` section that runs past a screen
  or two with no `###` is a sign it needs them.
- Never rename or remove an existing heading. Other chapters, the site and
  `experiments/build_context.py` refer to headings by their exact text.
  Add new `###`/`####` headings under them instead.
- Name headings after the thing (`### The path`, `#### E0003`), not after
  a sentence.

## Lists

- Use a list when there are three or more parallel items: rules,
  conditions, steps or options.
- When an item runs past two lines, put a blank line between the items
  (a "loose" list). GitHub then spaces them like paragraphs.
- Start a long item with its term in bold (`- **The method** is …`).
- Never let an item grow past about six lines. Move it to its own
  `###`/`####` subsection with paragraphs and a code block.
- Use numbered lists only for real sequences: steps, or the order things
  run in.

## Tables

- Use tables only for short cells: a word, a code or a short phrase.
- When a cell needs more than a sentence, use one entry per row instead:
  a `####` heading (for example `#### E0003 — type mismatch`), then
  bullets for the cases and a **Fix:** line.
- Align nothing by hand. GitHub ignores it, and it churns diffs.

## Code and examples

- Never change a fenced code block's content or its info string
  (`alder`, `alder fragment`, `alder error=…`, `sh`, …). `book/book_test.go`
  extracts, compiles and runs them. Moving a block is fine.
- Put a blank line before and after every code block, list and table.
- Put a command or a long piece of code in a fenced block, not inline.
- Keep inline code to names and short expressions.

## Asides and edge cases

- Put an edge case that interrupts the main rule in a blockquote callout
  under it: `> **`as` inside a default.** …`. Use a bold lead-in that says
  what it is about.
- Put a limitation or a link to an open issue in a callout
  (`> **Not yet:** …`) rather than in the middle of a sentence.

## What stays the same

- The content. These are formatting changes: no fact, rule, example or
  diagnostic is dropped. The book teaches everything, internals included.
- The wording, except where splitting a sentence or a paragraph needs
  a connecting word.
- Cross-references such as ("Rules", below) and links between chapters.
