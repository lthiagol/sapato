# 06 — sync, doctor

Spec sections: 3 (`[assignments]`), 7 (`sync`, `doctor`), 2 (overlaps).

## Deliver
- `ops.Sync(opts{Prune, Force, DryRun})`: expand globs in assignments; install not-installed, update outdated; with Prune uninstall managed skills not assigned. Unknown agent ids in assignments → error.
- `ops.Doctor` returning findings with severity (error/warn) and message. Overlap check uses the read-overlap table from spec §2 resolved against configured paths.
- CLI commands; `doctor` exits 1 if any error-severity finding.

## Done when
- Tests for sync plan (incl. prune on/off, globs) and every doctor finding type.
