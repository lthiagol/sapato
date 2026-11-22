# Sapato — agent conventions

Go CLI + Bubble Tea TUI for managing a skill vault. Spec: `docs/spec.md` (source of truth). Tasks: `docs/tasks/`.
Resuming? Read `docs/status.md` first (what is done, open nits, how the build/review loop runs).

## Rules

- Go module `sapato`. Go 1.27. Run before finishing: `gofmt -l .` (must print nothing), `go vet ./...`, `go test ./...`.
- **Never read or write real agent dirs** (`~/.claude`, `~/.cursor`, `~/.pi`, `~/.config/opencode`, `~/.agents`) or `~/.config/sapato`. Tests use `t.TempDir()` and pass paths explicitly.
- Shared fixtures: `testdata/skill-db/` (vault) and `testdata/targets/<agent>/` (mock agent dirs), helpers in `internal/testfixtures`. Prefer them in tests; copy with `testfixtures.CopyTree` before mutating. Never edit fixtures without keeping `TestFixturesConsistent` green.
- Do only what the current task asks. Don't implement future tasks.
- Packages under `internal/`. Keep `internal/cli` thin; logic lives in `internal/ops` and lower packages.
- Table-driven tests. Every exported function in `ops`, `vault`, `target`, `hashdir`, `config` has tests.
- Wrap errors with context: `fmt.Errorf("install %s: %w", name, err)`.
- Comments only where intent isn't obvious. No dead code, no TODO stubs for future tasks.
- Do not commit; the orchestrator commits after review.

## Workflow files (gitignored, under `.work/`)

- `.work/reviews/<task>-c<N>.md` — reviewer feedback for cycle N. A builder must address every item in the latest one.
- `.work/handoff/<task>-c<N>.md` — builder's summary for cycle N: what changed, what was verified, open doubts.
