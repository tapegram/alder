# E9 prompt sizes

Built at commit `5689a70` with `python3 experiments/build_context.py > experiments/e9-feedback/prompt-full.md`;
sized with `python3 experiments/build_context.py --size` (2026-10-12). Claude only (option A), so no Ollama
`--count` and no `reference` prompt.

| Prompt | File | Characters | Bytes (`wc -c`) | len/4 | Used by |
|---|---|---:|---:|---:|---|
| `full` (chapters 3–10 + example) | `prompt-full.md` | 250,290 | 250,619 | 62,572 | Claude (all 50 cells) |

The prompt grew since E8 (223,634 → 250,290 characters): M9's mutation-testing material in chapter 10.
