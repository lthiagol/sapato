# 18 — Import sources, discovery and --fix (ops + CLI)

Spec: §7 `import` row and §7.1 (all of it). TUI is task 19 — do not touch it beyond compiling.

## Deliver
- `internal/ops` import: source-kind order of §7.1; raw `.md` URL fetch (15s timeout, 1 MiB cap, 200 only, GitHub blob → raw rewrite) — make the HTTP client/base injectable so tests use `httptest`, never the network; local `SKILL.md` file; skill discovery (depth ≤ 4, skip dot-dirs/symlinks, stop at a found skill) with `--subdir`, `--all`, the several-candidates error listing relative paths.
- A discovery function the TUI can call without importing: given a source, return candidates (relative path, name hint, frontmatter name, validity/problems, whether `--fix` could repair it), plus a cleanup for temp dirs. Import should reuse it.
- `--fix` + `--description`: repair only frontmatter `name`/`description` (yaml.v3 node edit keeps other keys and order, body untouched; create frontmatter if missing), in the vault copy only; name-hint normalization per §7.1. Error without `--fix` suggests it when it would help.
- CLI flags: `--all`, `--fix`, `--description`. `--name` with `--all` is a usage error.

## Done when
- Tests: each source kind (tarball, raw URL via httptest incl. non-200, >1 MiB, GitHub blob rewrite as a pure function, git via a local bare repo or file:// as existing tests do, local file, local dir); discovery: root skill, one nested, several (error lists paths), `--all` all-or-nothing (one invalid → nothing written), `--subdir`, depth limit, dot-dirs skipped; `--fix`: no frontmatter, missing name, name mismatch, uppercase/space hint normalization, missing description with and without `--description`, extra frontmatter keys preserved, source unchanged, too-long description still refused; hint suggestion text. Gate passes.
