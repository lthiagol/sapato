# 16 — Compatibility metadata: vault, ops, CLI

Spec: §3 Compatibility, §6 `incompatible` + `compatible` field, §7 `install`, `compat`, `remove`, "Compatibility in other commands", §11 manifest `compatible`.

## Deliver
- `internal/vault`: parse `[skills.<name>]` with `compatible` (reject empty list, bad agent names, unknown keys). Expose `Compatible(skill, agent) bool` and the per-skill list. Targeted, atomic write of one skill's table that keeps every other byte of `sapato.toml` (spec §3).
- `internal/ops`: `SetCompat(v, skill, agents []string, opts)` (nil/all = delete table; skill must be in vault). `Status`: `Incompatible` state + `Compatible` on every row (json `compatible`). Install: refusal `incompatible` overridable by force; skipped+reported for globs/`--all`. Sync: skip incompatible, never fail. Update: unchanged. Remove: delete the table. Doctor: the three findings in §7. Export writes `compatible` in the manifest; tarball import writes it into the vault.
- `internal/cli`: `sapato compat <skill> [agent…] [--all]`, `--dry-run`. `status --json` shows `compatible`.
- Do not touch the TUI beyond what is needed to compile (task 17 does the TUI). If the TUI needs an icon for the new state to compile, map it to the not-installed icon for now.

## Done when
- Tests (table-driven): manifest parse (valid, empty list, bad name, unknown key); targeted write keeps comments/assignments/other skill tables byte-for-byte and replaces/removes only the one table; SetCompat set/all/unknown skill/dry-run; status incompatible + compatible=false on installed copy; install refusal, force, glob skip, `--all` skip; sync skip; remove deletes table; doctor's three findings; export manifest round-trip into a fresh vault; CLI `compat` print/set/--all.
- Uses fixtures/`t.TempDir()` only. Gate passes.
