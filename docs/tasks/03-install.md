# 03 — install, uninstall

Spec sections: 7 (`install`, `uninstall`), 8.

## Deliver
- `ops.Install`, `ops.Uninstall` with `Force`, `DryRun` options, returning a list of planned/applied actions.
- CLI commands with `-a/--agent` (repeatable), `--all`, glob skill args, `--force`, `--dry-run`.

## Done when
- Tests: fresh install, reinstall over managed synced, refuse unmanaged/modified without force, invalid skill refused, mode bits preserved, dot-paths/symlinks skipped, temp dir cleaned up on failure, dry-run changes nothing, uninstall refuses unmanaged even with --force, lock updated correctly.
