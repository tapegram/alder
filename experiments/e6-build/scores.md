| Model | Task | r0 | r1 | r2 | Result | Codes |
|---|---|---|---|---|---|---|
| claude-haiku | t2-newpage | 0 (build ✓) | — | — | pass | — |
| claude-haiku | t3-newapp | 0 (build ✓) | — | — | pass | — |
| claude-haiku | t4-appmsg | 0 (build ✓) | — | — | pass | — |
| claude-haiku | t5-ownership | 5 | 0 (build ✓) | — | pass | r0: E0001×5 |
| claude-sonnet | t2-newpage | 0 (build ✓) | — | — | pass | — |
| claude-sonnet | t3-newapp | 1 | 0 (build ✓) | — | pass | r0: E0003 |
| claude-sonnet | t4-appmsg | 0 (build ✓) | — | — | pass | — |
| claude-sonnet | t5-ownership | 0 (build ✓) | — | — | pass | — |
| ollama-qwen3-coder-30b | t2-newpage | 1 | 0 (build ✓) | — | pass | r0: E0003 |
| ollama-qwen3-coder-30b | t3-newapp | 19 | 4 | 4 | fail | r0: E0002×5, E0003×8, E0009×2, E0201×2, E0202, E0304; r1: E0008, E0011, E0201, E0304; r2: E0011, E0201, E0304×2 |
| ollama-qwen3-coder-30b | t4-appmsg | 1 | 0 (build ✓) | — | pass | r0: E0202 |
| ollama-qwen3-coder-30b | t5-ownership | 1 | 7 | 4 | fail | r0: E0602; r1: E0002×3, E0201, E0202×3; r2: E0203×4 |
