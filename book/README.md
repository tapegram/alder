# The Alder Book

Alder is a small, strict language for building server-rendered web
applications. It is designed so that an AI agent (or a person) has exactly
one way to do each thing, and so that the compiler catches the mistakes
that normally show up as a 404, a 500, or a broken form.

This book is the single source of truth for the language. When the language
changes, this book changes first.

## Reading order

| Chapter | What it covers |
|---|---|
| [1. Why Alder](01-why.md) | Purpose, principles, what Alder is not |
| [2. A tour](02-tour.md) | A complete to-do app, explained |
| [3. Syntax](03-syntax.md) | Modules, declarations, expressions, blocks |
| [4. Types](04-types.md) | Records, unions, brands, refinements |
| [5. HTML](05-html.md) | Markup, components, attributes, events |
| [6. Pages, messages, forms](06-pages.md) | Routes and the request lifecycle |
| [7. Effects](07-effects.md) | `Query`, `Task`, capabilities |
| [8. Database](08-database.md) | Tables, reads, writes, how tables are stored, schema changes |
| [9. Standard library](09-stdlib.md) | Module-by-module reference |
| [10. Testing](10-testing.md) | Pure tests |
| [11. Tooling](11-tooling.md) | The compiler and its outputs |
| [12. Design rationale](12-rationale.md) | Why each decision was made, with experiment results |
| [13. How the compiler works](13-compiler.md) | Lexer to generated Go, with one small program followed through every stage |
| [A. Grammar](A-grammar.md) | The formal grammar |
| [B. Common mistakes](B-pitfalls.md) | The errors seen most often in experiments |

Chapters 3–9 are the **reference**: together they are what an agent is given
as context when writing Alder. They are kept short on purpose.

## Status

Alder is a design in progress. `alder check`, `alder dump` and `alder build` exist: the to-do app serves, and `alder build` produces a binary that runs it over HTTP against SQLite (chapter 11). The dev loop, `alder run`, arrives with M3d. The reference
describes the language as currently specified; the rationale chapter records
what experiments have shown and what is still open.
