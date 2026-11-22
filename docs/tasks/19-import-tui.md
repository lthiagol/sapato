# 19 — IMPORT dialog: source input, file picker, multi-select, fix-up

Spec: §9.4 `IMPORT`, §9.2 key `I`. Depends on task 18's discovery/import API.

## Deliver
- IMPORT dialog with the source input (default: as today).
- `ctrl+o` file picker per §9.4 (bubbles `filepicker` if it fits the behaviour, else a small list in `internal/tui`). Start dir = input's dir if it exists, else the model's home dir (injected, never `$HOME` read in tests).
- `enter`: run discovery off the update loop (tea.Cmd); several candidates → multi-select list (`space`, `a`, `enter`); a candidate needing fix → fix-up form (problems, name prefilled with normalized hint, description) with `ctrl+s`; exists → force confirm; results as notifications; refresh.
- All widths per §9.4 dialog rules; nothing exceeds the terminal.

## Done when
- Tests: picker navigation (enter dir, backspace up, dot toggle, non-choosable file ignored, choose fills input); single-skill import; multi-candidate list marks and imports the marked only; none marked refused; fix-up imports with edited name/description; existing name → force confirm path; render at 60 and 120 columns fits. Temp dirs only. Gate passes.
