# 10 — TUI layout: header, grid, bottom bar, main + skill screens

Spec: §9.1, §9.2, §9.3 (themes/icons from task 08).

## Deliver
- `internal/tui/art.go` with exactly this art (6 lines, keep trailing spaces trimmed consistently):

```
███████╗ █████╗ ██████╗  █████╗ ████████╗ ██████╗ 
██╔════╝██╔══██╗██╔══██╗██╔══██╗╚══██╔══╝██╔═══██╗
███████╗███████║██████╔╝███████║   ██║   ██║   ██║
╚════██║██╔══██║██╔═══╝ ██╔══██║   ██║   ██║   ██║
███████║██║  ██║██║     ██║  ██║   ██║   ╚██████╔╝
╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝  ╚═╝   ╚═╝    ╚═════╝ 
```

- Header (with compact fallback), rules, centered grid with scrolling viewport, bottom bar with mode badge/message/right segment and powerline separators when nerd icons.
- Main screen keys per §9.2 (row cursor only; `a`, `n`, `I`, `t` may show "coming soon" in the bar until task 11).
- Skill screen per §9.3 with marks and all its actions (reuse the existing confirm/force-confirm flow and diff pager).
- Use `lipgloss.Width` for all width math (icons/box chars); never `len`.

## Done when
- Model tests: navigation, enter/esc between screens, marks, every skill-screen action incl. force flow on a fixture copy, filter.
- View tests (golden-ish, `NO_COLOR` + unicode icons): header full vs compact by size; grid centered (left padding equal ±1 to right); no line exceeds terminal width at 80×24, 120×40, 200×60; bar fits in one row at 80 cols.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.

## Carry-over from 09
- `internal/ops/archive.go:129`: skip pax global headers (`tar.TypeXGlobalHeader`) instead of refusing (still counted toward the entry cap), so `git archive` tarballs import. Test it.
- `internal/ops/archive.go:36`: `isArchive` case-insensitive (`.TGZ`, `.TAR.GZ`).
- `internal/ops/export_test.go:114`: `os.Chtimes` a file between the two exports so the determinism test proves mtimes are ignored.
