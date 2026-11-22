# 22 — `sapato guide`

Spec: §7 `guide` row.

## Deliver
- `internal/ops`: `Guide(vaultPath string, agents []config.Agent) []byte` generating the `SKILL.md` (frontmatter `name: sapato`, a description saying when an agent should use it). Content per the spec row: vault path, layout, §3 rules, compat + `sapato compat`, create (`new --name …`, `new <file>`), import (§7.1 sources, `--fix`), install/check commands, configured agents in order. It must itself pass vault validation.
- `GuideInstall(v, agents, opts)`: write `skills/sapato/SKILL.md`; identical → no-op action; different → refuse unless force; dry-run.
- CLI `sapato guide [--install] [--force] [--dry-run]`.

## Done when
- Tests: output validates as a skill; contains the vault path and each agent name in order; install writes, re-install identical is a no-op, different content refused/forced, dry-run writes nothing; CLI prints to stdout. Gate passes.
