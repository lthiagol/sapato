# 02 — Targets, lock file, status

Spec sections: 5, 6, 7 (`status`).

## Deliver
- `internal/target`: `ReadLock(dir)`, `WriteLock(dir, Lock)` (atomic, only if changed), `Scan(dir)` → installed skill folder names (ignore dot-dirs; missing dir → empty).
- `internal/ops`: `Status(vault, agents, filter) ([]Row, error)` producing every status in spec §6 with correct precedence.
- `sapato status [skill…] [-a id…] [--json]`: table (rows skills, columns agents) and JSON form.

## Done when
- Each status value has a test that builds the scenario in temp dirs. Lock atomic write + no-op-when-unchanged tested. Corrupt lock returns an error naming the file.

## Notes
- Fixtures `testdata/targets/*` encode every status; build status tests on them (via `testfixtures.CopyTree`).
- Lock entries for unversioned skills write `"version": ""` (no omitempty), matching the fixtures.
