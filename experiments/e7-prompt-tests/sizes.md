# E7 prompt sizes

Built at commit `1de748d` (`git rev-parse --short HEAD`) with `python3 experiments/build_context.py [--reference]`;
sized with `python3 experiments/build_context.py [--reference] --size --count ollama:qwen3-coder:30b`
(Ollama's `prompt_eval_count` for the prompt alone, one `/api/chat` call at `num_ctx` 65536, measured 2026-10-10).

| Prompt | File | Characters | Bytes (`wc -c`) | len/4 | Ollama `prompt_eval_count` (qwen3-coder:30b) | Run at `num_ctx` |
|---|---|---:|---:|---:|---:|---:|
| `full` (chapters 3–10 + example) | `prompt-full.md` | 117,945 | 118,051 | 29,486 | 32,525 | 65,536 |
| `reference` (`--reference`: human-facing sections dropped) | `prompt-reference.md` | 77,663 | 77,753 | 19,415 | 21,527 | 32,768 |

The `reference` prompt is under the 26,000-token limit (21,527), so the drop list was not extended.
The `full` prompt alone (32,525 tokens) would not fit E6's 32,768 window with any output; it runs at 65,536.
`reference` drops 40,282 characters (34.2%) and 10,998 Ollama tokens (33.8%).
