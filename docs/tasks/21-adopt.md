# 21 — Adopt: identical copies, globs, grouped multi-select dialog

Spec: §7 `adopt` row, §9.4 `ADOPT`.

## Deliver
- `ops.Adopt`: after adopting from one agent, record the lock in every other configured agent whose unmanaged copy of that name has the identical hash. Return actions for each.
- CLI: `adopt <skill…> -a <id>` with globs over that agent's unmanaged skills (glob/literal rules as §7).
- TUI ADOPT: grouped by agent, filter row (`tab`/`shift+tab`/`←→`), `space` marks, `enter` adopts marked or cursor, `conflict` tag for same name with different hashes, refuse marking two rows of the same name, one summary notification, force confirm on `ErrNeedsForce`.

## Done when
- Tests: identical copy in a second agent becomes synced; a copy with a different hash stays unmanaged in its agent; CLI glob adopt; TUI grouping order, filter switching, mark + enter adopts all marked, conflict tag, duplicate-name mark refused, summary notification. Gate passes.
