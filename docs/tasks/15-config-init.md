# 15 — `sapato config init` and `[ui]` in `sapato config`

Spec: §2, §7 (`config`, `config init`).

## Deliver
- `sapato config` prints `theme:` and `icons:` lines after `vault:`.
- `sapato config init [--force]` (global `--config` and `--dry-run` respected): writes a commented starter TOML:
  - header comment (what the file is, link to `docs/spec.md` §2),
  - `vault = "~/sapato-vault"`,
  - `[ui]` with `theme`/`icons` and a comment listing every valid theme and icon set (generated from the theme/icon packages, not hard-coded twice),
  - one `[agents.<id>]` block per built-in agent with its default `path` and `enabled = true`, plus a comment naming the dirs that agent also reads (§2 overlaps).
  - Creates parent dirs (0755), file 0644; refuses if the file exists unless `--force`; `--dry-run` prints the content to stdout and writes nothing.
- The generated file must load with `config.Load` and resolve to exactly the defaults.

## Done when
- Tests: init writes to a temp `--config` path and `config.Load` of it equals defaults; refuses existing file; `--force` overwrites; `--dry-run` writes nothing and prints valid TOML; every theme/icon name appears in the comment; `sapato config` shows theme/icons (default and overridden).
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
