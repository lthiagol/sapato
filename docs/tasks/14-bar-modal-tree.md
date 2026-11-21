# 14 — Bar redesign, modal confirms, tree view

Spec: §9.1 (bottom bar, rewritten), §9.4 (modal confirmations), §9.3 item 5 (tree connectors).

## Deliver
- Bottom bar per §9.1: rule + 2 rows, no badge, no background; row 1 centered action hints; row 2 global hints left, colored summary right. Hint style (key Accent bold, desc Subtext, ` · ` Overlay). Split every screen/mode's hints into action vs global per §9.1. `t` theme works on the main and skill screens.
- Remove mode-badge code and `ModeColor` if now unused (and their tests); keep mode internally.
- Modal confirms per §9.4: render the underlying screen, strip its styles and re-color it Overlay, then splice a centered box (ANSI-aware) over it; Accent border for plain, Red for force; title in border. All existing confirm/force flows and their tests keep working (update assertions that relied on the old full-screen confirm layout).
- FILES tree with `tree(1)` connectors per §9.3 item 5 (unicode/nerd: `├── └── │`, ascii: `|-- `--  |`), connectors Overlay; sizes/exec glyph unchanged.

## Done when
- Tests: bar has a rule + exactly 2 rows at 80/120/200 for every screen/mode; row 1 action hints centered (±1); row 2 summary right-aligned at the edge and globals at the left; at 60 cols the summary survives and globals are cut with `…`; zero counts render dimmed; no badge text (`MAIN`, `SKILL`) in the bar.
- Modal: for plain, force, remove and discard confirms the box is centered (±1), width within bounds, border color Accent vs Red, the underlying screen text is still present outside the box and carries no non-Overlay color; `n`/`esc` returns to the exact previous screen.
- Tree: exact expected lines for fixture 00003 and 00006 (unicode) and 00003 (ascii), including `│   ` continuation and `└── ` on last children.
- Nothing exceeds width at 60×20, 80×24, 120×40, 200×60.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
