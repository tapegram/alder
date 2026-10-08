# Experiments

Each experiment lives in `experiments/<id>-<name>/` with:

- `prompt.md`: the exact context given to the model (built by `build_context.py`)
- `tasks/`: one file per task
- `runs/<model>/<task>.md`: raw model output
- `scores.md`: hand scores, one row per (model, task)
- `findings.md`: what was learned, and what changed in the book as a result

## Method

1. The model receives the reference chapters (3–9) and the full to-do
   example as context, then one task. No compiler, no retries, one shot.
2. Output is scored by hand against the error taxonomy below. Each
   distinct error is counted once per output. **PASS** = compiles as
   written. **PARTIAL** = 1–2 errors, each a one-line fix a compiler
   message would point at. **FAIL** = 3+ errors or a structural
   misunderstanding.
3. Anything the model did that is *reasonable but not Alder* is recorded as
   a design signal, separately from errors. These are the most valuable
   outputs: they show where the language fights the model's prior.

## Error taxonomy

| Code | Meaning | Examples |
|---|---|---|
| SYN | invented or wrong syntax | `case ... of`, `let ... in`, `fn`, semicolons, `=` in records, missing commas in `match` |
| LIB | hallucinated library | `List.mapIndexed`, `String.fromInt`, `Db.query`, `Task.andThen` |
| TYP | type misuse | `Int` in interpolation, `Maybe` used as value, brand constructed outside its module, `==` across brand and base, tuple |
| EFF | effect misuse | `Db.*` outside an effect block, write in `load`, missing `let x <-`, missing `Task.succeed`, `Task` in `Query` |
| ROUTE | page/msg/form wiring | `href` with a string, msg params missing, `update` arm missing, `form` field mismatch, `Params`/`Msg` written by hand |
| HTML | markup rules | `onclick`, unclosed tag, non-Text in braces, `<input>` inside `<form submit>` |
| STRUCT | module structure | wrong module name, missing import, unqualified cross-module name, `Todo.Todo` |
| ANN | missing annotation | top-level without signature |

## Models

| Name | How run |
|---|---|
| claude-haiku, claude-sonnet, claude-opus | subagent with the prompt + task, fresh context |
| qwen3-coder:30b, qwen3:8b | Ollama, `run_ollama.py`, `num_ctx` 32768, temperature 0 |

## Tasks

Tasks are chosen to exercise different things:

- **T1 modify**: add a field across table, form, view (touches TYP, ROUTE, HTML).
- **T2 new page**: a new page module from scratch (touches STRUCT, EFF, ROUTE).
- **T3 new app**: a small app from scratch (everything).
