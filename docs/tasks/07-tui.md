# 07 — TUI

Spec section: 9.

## Deliver
- `internal/tui` Bubble Tea app using `ops` only. `sapato` with no subcommand and `sapato tui` launch it.
- Matrix view, keybindings, confirm dialogs, diff pager, filter, help, status bar with last action result/error.

## Done when
- Model update logic unit-tested (key → command/state) without a real terminal. `go test ./...` passes.

## Notes
- Read existing `internal/ops` APIs first; the TUI calls `ops.Status`, `Install`, `Uninstall`, `Update`, `Diff`, `Adopt` exactly like the CLI (same options, incl. DryRun for confirmation previews). No business logic in `internal/tui`.
- Keep the model testable: pure `Update(msg)` transitions; ops calls behind an interface or function fields so tests can stub them, plus at least one test against a `testfixtures.CopyTree` copy.
- Glyphs/colors per status (all 8 from spec §6) with a legend in the help view. Respect `NO_COLOR`.

## Carry-over from 06b
- `internal/ops/install.go:209-212`: a failed `RemoveAll(backup)` after a successful commit must not fail the call; ignore it (the leftover is dot-prefixed).
