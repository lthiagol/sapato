# 13 — Skill screen: sidebar + details + files

Spec: §9.3 (new layout), §9.5 (exec glyph), §4 (file walk).

## Deliver
- `ops.SkillFiles(dir) ([]File, error)` with `File{Path string; Size int64; Exec bool}` using the same walk as hashing (`hashdir.Files`); `ops.LockEntry(agentDir, skill) (target.Entry, bool, error)` (or equivalent) for `installed_at`/version.
- Skill screen per §9.3: bordered sidebar (focused, Accent border) with marks/cursor, details pane with info, cursor-agent section, FILES tree (dirs as `name/`, indent 2 per level, exec glyph, human sizes right-aligned), scrolling with `pgup`/`pgdn`, narrow (<70 cols) stacked layout. Keys and actions unchanged.
- Remove the old centered skill-block code paths that are no longer used (and their now-obsolete tests), keeping the main-screen and dialog centering.

## Done when
- Tests: files tree for fixture skills 00001 (1 file), 00003 (nested references/guides), 00004 (two exec scripts with glyph), 00005 (binary size), 00006 (dot-paths/symlink absent); human size formatting table test; orphaned skill lists installed files with the `installed in <agent>` title; agent section shows `installed <date>` from the lock for a managed agent and nothing for not-installed; sidebar width ≥ 26 and focused border uses Accent; stacked layout below 70 cols; nothing exceeds width at 60×20, 80×24, 120×40, 200×60; pgup/pgdn scroll the details when the tree is taller than the body; existing skill-screen action/force tests still pass.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
