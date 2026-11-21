# 08 — Themes, icons, [ui] config

Spec: §2 (`[ui]`, `--theme`, `--icons`), §9.5. Palettes: `docs/themes.md`.

## Deliver
- `internal/config`: `[ui]` section (`theme`, `icons`) with defaults `catppuccin-mocha` / `nerd`; validation errors list valid names. Config exposes `UI{Theme, Icons string}`.
- Global CLI flags `--theme`, `--icons` overriding config (validated the same way).
- `internal/tui/theme`: `type Theme struct{ Name string; Base, Mantle, Surface, Overlay, Text, Subtext, Accent, Green, Yellow, Orange, Red, Purple, Cyan, Pink lipgloss.Color }`, `All() []Theme` (order: catppuccin-latte, -frappe, -macchiato, -mocha, dracula, dracula-alucard, nord, nord-light), `ByName(string) (Theme, bool)`. Values exactly from `docs/themes.md`. Helpers: `StatusColor(ops.State) lipgloss.Color`, `ModeColor(mode string) lipgloss.Color` per §9.4.
- `internal/tui/icons`: `type Set struct{ Status map[ops.State]string; Marker, Mark, SepRight, SepLeft string }`, `ByName("nerd"|"unicode"|"ascii")`. Values exactly from spec §9.5 (nerd separators ``/``; unicode/ascii use `│`/`|`). Write nerd glyphs as `\u` escapes in Go source.
- Wire the chosen theme/icon set into the existing TUI so current views use roles/icon sets instead of hardcoded colors/glyphs. Do not change layout yet (task 10).

## Done when
- Tests: every theme has every role non-empty and valid `#rrggbb`; theme names unique and match the list; hex spot-check of one value per theme against docs/themes.md; every icon set covers all 8 states; config defaults/overrides/invalid names; flag overrides.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
