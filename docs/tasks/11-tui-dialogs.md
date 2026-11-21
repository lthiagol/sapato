# 11 — TUI dialogs: adopt picker, new skill, import/export, theme picker

Spec: §9.4 (ops from tasks 05, 09).

## Deliver
- `ADOPT` picker, `NEW` textarea editor with live validation and `ctrl+s`, `IMPORT`/`EXPORT` path inputs, `THEME` picker with live preview and `esc` restore, `HELP` per screen with legend. Wire main-screen `a`, `n`, `I`, `t` and skill-screen `x`, `D`.
- All via `ops` (`Adopt`, `Create`, `Import`, `Export`, `Remove`) with the unforced-first / force-confirm flow.

## Done when
- Model tests for each dialog: open, cancel, success, error shown in bar, force flow where applicable; theme preview changes theme and `esc` restores; new-skill save creates the skill on a fixture copy; export→import via the dialogs on a fixture copy.
- No line exceeds terminal width at 80×24 for any dialog.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
