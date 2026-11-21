# 06b — Hardening (carry-overs from 05/06 reviews)

Spec sections: 2, 7 (sync/doctor notes, overlaps).

## Deliver
1. **Overlap uses fixed extra-read dirs.** `internal/config` exposes, per agent, the extra dirs it reads (spec §2 table, `~` expanded with the given home), independent of configured target paths. `ops.Doctor` reports an overlap when a skill is present in an agent's own path and in one of its extra-read dirs, where "same dir" compares `filepath.EvalSymlinks` results (fall back to `filepath.Clean` if the dir doesn't exist). Keep `ops` free of `$HOME`: paths come in through config.
2. **Duplicate target paths.** `config.Load` errors if two enabled agents resolve to the same path (after Clean). Test it.
3. **Import `--force` rollback.** `internal/ops/import.go` uses the same set-aside/restore mechanism as adopt (`placeSkillThen` or equivalent), so a failure never leaves the vault without the old copy. Test it.

## Done when
- Tests for each item above, including: overlap detected when `claude.path` is overridden but the default `~/.claude/skills` (under the test home) contains the same skill; symlinked dirs not reported as overlap with themselves.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
