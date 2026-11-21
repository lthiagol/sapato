# 15 — Agents as data, `config init`, config output

Spec: §2 (rewritten: `[agents]` name = path, order, inline `reads`, defaults, no-config behavior), §7 (`config`, `config init`, sync assignment rule, `--all`), §9 (TUI first-launch prompt).

Cycle 1 built `config init` and `[ui]` output against the old agent format; cycles 2+ extend it to the new schema.

## Deliver
- `internal/config`: parse `[agents]` as `name = "path"` or inline `{ path, reads }`, preserving file order (go-toml/v2 decodes maps unordered — use its `unstable` parser or an ordered decode; no new dependency). Validate names, duplicate paths, unknown keys inside inline tables. No `[agents]` → built-in defaults in spec order; empty `[agents]` → no agents. Built-in names written as strings inherit built-in `reads`. Reject the old `[agents.<id>]` table form with a helpful error. Remove `Enabled` everywhere (config, ops, cli, tui) — every configured agent is active. Expose whether defaults were used and whether a config file existed.
- `sapato config`: vault, theme, icons, then agents in order with path and reads; a note line when built-in defaults are in use.
- `sapato config init`: template per spec §2 example (active built-ins, commented examples for a custom agent and an inline-table agent, `[ui]` comment listing valid names generated from the theme/icon packages). Same refusal/`--force`/`--dry-run` behavior as cycle 1. The generated file loads to exactly the defaults.
- `ops.Sync`/`ops.Doctor`: assignments naming unconfigured agents → skip with `not configured here` (sync) / info finding (doctor); never an error.
- TUI first launch with no config file: modal confirm `No config found. Create <path>?` (spec §2); `y` writes via the same code as `config init` and notifies; `n` continues. Not shown when a config file exists or `--config` points to an existing file.

## Done when
- Tests: ordered parse (order differs from alphabetical and is kept); string vs inline table; built-in `reads` inheritance and override; empty `[agents]`; missing `[agents]`; old form rejected; bad name; duplicate path; unknown inline key; init → load equals defaults; init refuse/force/dry-run; `sapato config` output incl. defaults note; sync/doctor with an unconfigured assignment agent; TUI prompt shown only without a config file, `y` writes a loadable file under a temp HOME, `n` writes nothing.
- All existing tests updated for the removal of `Enabled` (none deleted without replacement).
- Tests never touch the real home or `~/.config/sapato`.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
