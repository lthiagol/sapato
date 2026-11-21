# 04 — update, diff

Spec sections: 7 (`update`, `diff`).

## Deliver
- `ops.Update` (reuses install path; default all skills × enabled agents; skips modified unless Force).
- `ops.Diff` returning per-file added/removed/changed plus unified text diff for text files (binary files: "binary differs"). Implement a small LCS line diff; no new dependency.
- CLI `update`, `diff`.

## Done when
- Tests for update selection (outdated only, modified skipped/forced, dry-run) and diff output for add/remove/change/binary.
