# 01 — Foundation: scaffold, config, vault, hash

Spec sections: 2, 3, 4, 10.

## Deliver
- `go.mod` (module `sapato`), `cmd/sapato/main.go`, cobra root in `internal/cli` with global `--config` and `--dry-run` flags.
- `internal/config`: `Load(path string, home string) (Config, error)` — `path==""` means `<home>/.config/sapato/config.toml`; missing file → defaults. Built-in agents table (spec §2) with overrides from `[agents.<id>]`; unknown agent ids in config are an error. `~` expansion. `Config` exposes `Vault string` and ordered `Agents []Agent{ID, Path, Enabled}`.
- `internal/hashdir`: `Hash(dir string) (string, error)` exactly per spec §4.
- `internal/vault`: `Open(path)` reads `sapato.toml` (format must be 1; parse `[assignments]` into `map[string][]string`), `Skills()` returns all skills sorted by name with `Name, Dir, Description, Version, Hash, Valid bool, Problems []string` per spec §3 rules. `Init(path)` creates a vault.
- Commands: `sapato config`, `sapato init [path]`, `sapato list`. Home dir comes from `os.UserHomeDir()` only inside `internal/cli`.

## Done when
- Tests cover: config defaults/overrides/unknown agent/`~` expansion; every validation rule (pass + fail); hash determinism, exec-bit and dot-path/symlink exclusion, sensitivity to content and rename; vault format errors; init refusing existing vault.
- `go build ./... && go vet ./... && go test ./...` pass, `gofmt -l .` empty.
