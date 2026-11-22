# 20 — New-skill scaffold and wizard

Spec: §7 `new --name` row, §7.2 template, §9.4 `NEW`.

## Deliver
- `internal/ops`: `Scaffold(v, name, description, compatible []string, opts)` writes the §7.2 template (description YAML-quoted when needed), validates via the normal rules, refuses existing name unless force, writes the compat table when `compatible` is non-nil and not all agents (reuse task 16's write). Dry-run supported.
- CLI: `new --name --description [--compatible a,b] [--edit]`; file arg + `--name` → usage error; `--edit` runs `$VISUAL`/`$EDITOR`/`vi` (make the runner injectable for tests).
- TUI NEW: wizard per §9.4 (fields, live validation, `N/1024`, agent checkboxes), `tab` toggles to the existing paste mode, `ctrl+s` scaffolds, then the `Open … in <editor>?` modal; `y` → `tea.ExecProcess` (injectable), refresh after. `esc` confirm when anything typed.

## Done when
- Tests: Scaffold content exact (incl. a description needing quotes) and it validates; existing name refuse/force; compat table written / not written when all checked; CLI scaffold, usage error, `--edit` invokes the runner with the right path and editor precedence; TUI wizard field validation, tab toggle keeps paste text, ctrl+s creates, editor prompt y/n, esc confirm. Gate passes.
