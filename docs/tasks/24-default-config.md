# 24 — Commented default config and vault manifest

Spec: §2.1 (new), §3.1 (new), §7 rows `config default`, `config init`, `init`.

## Deliver
- `internal/config`: one generator for the §2.1 text (replaces the current `Starter`): order vault → [agents] → [ui] (icons, theme), comment blocks exactly as §2.1, `reads` end-of-line comments aligned by computation, icon and theme lists generated from code. Theme metadata (light flag) lives with the theme list `config.ThemeNames` already mirrors; keep the two lists from drifting (a test comparing them exists or add one).
- CLI `sapato config default` (stdout, no writes). `config init` and its `--dry-run`, and the TUI first-launch config prompt, use the same generator.
- `internal/vault`: `Init` writes the §3.1 manifest exactly.
- `testdata/skill-db/sapato.toml` already carries the §3.1 comments (orchestrator edit, keep it).

## Done when
- Tests: `config default` output equals §2.1 byte-for-byte (golden string in the test); it loads to exactly the defaults; theme/icon comment lists match the registered names and the default/light marks; `config init` writes the same bytes; `--dry-run` prints them; TUI prompt writes the same bytes; `vault.Init` writes §3.1 byte-for-byte and the result opens with empty assignments; `sapato init` and a compat write after it keep every comment. Gate passes.
