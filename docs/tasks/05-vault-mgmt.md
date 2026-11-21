# 05 — import, adopt, remove

Spec sections: 7 (`import`, `adopt`, `remove`).

## Deliver
- `ops.Import(src, opts)`: local path, or git URL (`https://`, `git@`, ends `.git`) via `git clone --depth 1` into a temp dir, optional `Subdir`, optional `Name` (must match frontmatter `name`, else error). Validate before copy.
- `ops.Adopt(skill, agent)`, `ops.Remove(skill, force)`.
- CLI commands.

## Done when
- Tests for each (git import tested with a local repo created in the test via `git init`; skip if `git` missing). Refusals tested.

## Carry-over from 04
- `internal/ops/diff.go:129`: compare only the exec bit (`mode&0o111 != 0`), matching the hash rule (spec §4), so `diff` and `status` agree. Add a test.
