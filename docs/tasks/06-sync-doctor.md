# 06 — sync, doctor

Spec sections: 3 (`[assignments]`), 7 (`sync`, `doctor`), 2 (overlaps).

## Deliver
- `ops.Sync(opts{Prune, Force, DryRun})`: expand globs in assignments; install not-installed, update outdated; with Prune uninstall managed skills not assigned. Unknown agent ids in assignments → error.
- `ops.Doctor` returning findings with severity (error/warn) and message. Overlap check uses the read-overlap table from spec §2 resolved against configured paths.
- CLI commands; `doctor` exits 1 if any error-severity finding.

## Done when
- Tests for sync plan (incl. prune on/off, globs) and every doctor finding type.

## Carry-over from 05
- `internal/ops/adopt.go:183-185`: with `--force`, a failed lock write must not leave the vault with neither copy. Check the lock is writable (or stage the old vault copy) before `placeSkill`; add a test.
- `internal/ops/import.go`: resolve a local `src` with `filepath.EvalSymlinks` so `sapato import ~/link` works; add a test.
- `internal/ops/import.go:91`: set `GIT_PROTOCOL_FROM_USER=0` in the clone env.
