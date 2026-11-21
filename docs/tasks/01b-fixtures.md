# 01b — Test fixtures: mocked vault + mock agent targets

Spec sections: 3, 4, 5, 6.

Fixtures are read-only. Tests that mutate must copy them into `t.TempDir()` first.

## Deliver

### `testdata/skill-db/` — a valid vault
- `sapato.toml`: `format = 1` and an `[assignments]` table (e.g. `claude = ["mocked-skill-0000[1-3]"]`, `agents = ["*"]`).
- Skills, each `skills/mocked-skill-0000N/` with frontmatter `name: mocked-skill-0000N`, a Lorem Ipsum `description`, and a body starting `# Mocked Skill 0000N` followed by Lorem Ipsum paragraphs:

| skill | shape |
|---|---|
| 00001 | minimal: `SKILL.md` only |
| 00002 | `metadata.version: "1.2.0"` |
| 00003 | multiple docs: `references/overview.md`, `references/api.md`, `references/guides/advanced.md`; SKILL.md links to them |
| 00004 | `scripts/run.sh` and `scripts/check.py`, both executable (mode 0755), plus `scripts/README.md` (0644) |
| 00005 | `assets/logo.png` (a real tiny valid PNG, binary) and `assets/template.txt` |
| 00006 | all of the above combined, plus dot-paths that must be excluded from hashing: `.DS_Store`, `.hidden/notes.md`, `references/.cache/tmp.md`, and a symlink `link.md -> SKILL.md` |

- Invalid skills (one rule violation each), in folders:
  - `invalid-no-frontmatter` (SKILL.md without `---` block)
  - `invalid-name-mismatch` (frontmatter `name: something-else`)
  - `Invalid-Uppercase` (folder and name uppercase)
  - `invalid-missing-description`
  - `invalid-long-description` (description of 1025 chars)
  - `invalid-no-skill-md` (folder with only `README.md`)
- Also in `skills/`: a plain file `stray.txt` and a dir `.trash/` (both must be ignored).

### `testdata/targets/` — mock agent skill dirs (for status tests in task 02)
- `claude/`: installed copies + `.sapato-lock.json` (format per spec §5) giving these statuses against `skill-db`:
  - `mocked-skill-00001` synced (copy identical, lock hash == vault hash)
  - `mocked-skill-00002` outdated (copy == lock hash, but older content than vault: e.g. version "1.1.0")
  - `mocked-skill-00003` modified (lock hash = vault hash, but copy has an edited file)
  - `mocked-skill-00099` orphaned (in lock + target, not in vault)
  - `mocked-skill-00098` missing (in lock, no folder)
  - `hand-made-skill` unmanaged (folder present, not in lock)
- `agents/`: only `mocked-skill-00001` synced, with lock.
- `cursor/`: empty except a `.gitkeep`.
- Lock hashes must be real `hashdir.Hash` values. Compute them with a throwaway `go run` program or test; don't hand-write them.

### `internal/testfixtures` package
- `Root() string`: absolute path to `testdata/` (via `runtime.Caller`).
- `CopyTree(t, src) string`: copies a fixture tree into `t.TempDir()` preserving modes and symlinks; returns the new path.
- A test `TestFixturesConsistent` asserting: every lock hash in `targets/*` equals `hashdir.Hash` of the copy (except the deliberate `modified` case, where it must differ) and the expected status relationship to the vault holds (vault hash equal/different as listed above). This test fails loudly if anyone edits fixtures without updating locks.

### Use the fixtures in existing tests
- `internal/vault`: a test that opens `testdata/skill-db` and asserts the exact list of skills, valid flags and the problem for each invalid skill.
- `internal/hashdir`: a test showing `mocked-skill-00006` hash is unchanged after removing its dot-paths and symlink (on a copy), covering nested dot path exclusion.
- Add a vault test for `SKILL.md` being a symlink (rule 1).

## Done when
- Fixtures exist exactly as listed; executable bits are set in the working tree.
- `go build ./... && go vet ./... && go test ./...` pass, `gofmt -l .` empty.
- `go run ./cmd/sapato --config <tmp config pointing vault to testdata/skill-db> list` shows 6 valid + 6 invalid skills (mention output in handoff).
