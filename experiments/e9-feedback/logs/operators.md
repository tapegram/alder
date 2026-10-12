| class | operator | baseline killed/all | feedback killed/all |
|---|---|---|---|
| authorization | guard dropped | 50/62 (0.81) | 62/62 (1.00) |
| authorization | requires dropped | 17/70 (0.24) | 64/70 (0.91) |
| data | limit dropped | 0/5 (0.00) | 0/5 (0.00) |
| data | offset dropped | 5/5 (1.00) | 5/5 (1.00) |
| data | orderBy reversed | 14/52 (0.27) | 47/52 (0.90) |
| data | session put dropped | 21/50 (0.42) | 45/50 (0.90) |
| data | update field dropped | 0/10 (0.00) | 10/10 (1.00) |
| data | where dropped | 55/128 (0.43) | 127/138 (0.92) |
| data | write replaced | 66/95 (0.69) | 89/96 (0.93) |
| flow | arm swapped | 16/24 (0.67) | 20/23 (0.87) |
| flow | redirect → rerender | 80/145 (0.55) | 131/148 (0.89) |
| flow | reject → rerender | 18/51 (0.35) | 39/47 (0.83) |
| logic | bool flipped | 14/23 (0.61) | 19/25 (0.76) |
| logic | condition negated | 65/68 (0.96) | 69/70 (0.99) |
| logic | not dropped | 9/15 (0.60) | 15/17 (0.88) |
| logic | operator flipped | 171/217 (0.79) | 205/224 (0.92) |
| schema | cascade ↔ restrict | 5/60 (0.08) | 48/60 (0.80) |
| schema | default changed | 18/19 (0.95) | 17/18 (0.94) |

Rerun agrees with every summary's `killed`.
