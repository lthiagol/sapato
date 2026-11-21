# 07 — TUI

Spec section: 9.

## Deliver
- `internal/tui` Bubble Tea app using `ops` only. `sapato` with no subcommand and `sapato tui` launch it.
- Matrix view, keybindings, confirm dialogs, diff pager, filter, help, status bar with last action result/error.

## Done when
- Model update logic unit-tested (key → command/state) without a real terminal. `go test ./...` passes.
