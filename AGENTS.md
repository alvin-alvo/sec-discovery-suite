# Agent instructions
Read ALL files in docs/ (01 to 05) before writing code. They are the source of truth.
- Work one phase at a time; do not build ahead of the phase you are given.
- Do not add a dependency to agent/ without measuring binary size (scripts/check-size.sh).
- Never put mock data in production code paths. Sample data must be flagged is_sample=true.
- Never store, log, or transmit private-key material or TLS payload bytes.
- After each task, report exact commands run and exact outputs (sizes, test results).
