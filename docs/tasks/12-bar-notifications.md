# 12 — Two-row bar, notifications, log

Spec: §9.1 (bar), §9.5 (icons: info, exec glyph, no separators), §9.6.

## Deliver
- Remove powerline separators everywhere (icon sets drop `SepRight`/`SepLeft`); bar is 2 rows per §9.1. Body height accounts for the extra row.
- Replace the bar message with notifications per §9.6: model keeps a notification list with timestamps; `tea.Tick` expiry; injectable clock (`now func() time.Time`) for tests. Every place that set the bar message now pushes a notification with the right kind/title.
- Overlay rendering: draw the stack over the body's bottom-right without breaking other lines (ANSI-aware: use lipgloss/`ansi` helpers to splice, never byte slicing).
- `L` opens LOG mode (centered dialog, scrollable, max 200, oldest first). Add `L log` to every screen's hints and help.
- Add the info and exec glyphs to the icon sets (exec used in task 13).

## Done when
- Tests: bar is exactly 2 rows and fits at 80/120/200 cols for every mode; no ``/`` in any rendered screen; success expires at 4s and error at 10s (fake clock); max 3 visible, newest last; long message wraps to ≤4 lines with `…`; toast box placed bottom-right above the bar with 1-col margin; lines with an overlaid toast keep their full width (no overflow) and the body text left of the toast is unchanged; LOG lists all, scrolls, caps at 200, `esc` closes; `L` ignored inside NEW textarea and path inputs.
- Existing tests updated (not deleted) where they asserted the old bar message.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
