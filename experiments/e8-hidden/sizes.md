# E8 prompt sizes

Built at commit `8c0526f` with `python3 experiments/build_context.py [--reference]`;
sized with `python3 experiments/build_context.py --reference --size --count ollama:qwen3-coder:30b` (num_ctx 65536)
and `python3 experiments/build_context.py --size --count ollama:qwen3-coder:30b --num-ctx 98304`
(Ollama's `prompt_eval_count` for the prompt alone, one `/api/chat` call, measured 2026-10-11).

| Prompt | File | Characters | Bytes (`wc -c`) | len/4 | Ollama `prompt_eval_count` (qwen3-coder:30b) | Used by |
|---|---|---:|---:|---:|---:|---|
| `full` (chapters 3–10 + example) | `prompt-full.md` | 223,634 | 223,844 | 55,908 | 60,968 | Claude (all cells) |
| `reference` (`--reference`) | `prompt-reference.md` | 152,931 | 153,077 | 38,232 | 41,990 | Qwen (T5, T6), at `num_ctx` 65,536 |

Both prompts grew since E7 (`full` 117,945 → 223,634 characters; `reference` 77,663 → 152,931): the book
gained chapters' worth of M5 material (the browser of chapter 10 §9, migrations, `fake`).
The `reference` prompt alone (41,990 tokens) no longer fits the 32,768 window E7 used for it, so Qwen runs at
`num_ctx` 65,536 (see `RUN.md`, deviations). The `full` prompt (60,968 tokens) would leave under 5k tokens of a
65,536 window, which is why Qwen gets `reference`.
