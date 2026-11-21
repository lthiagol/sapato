# Sapato — Spec

**S**kills **A**dministration, **P**ackaging & **A**gent **T**ool **O**rganization.

Sapato manages a portable *skill vault* (a plain folder) and installs skills from it into coding agents' skill directories. It is a Go CLI with a Bubble Tea TUI on top. Every TUI action maps to a CLI command.

## 1. Concepts

- **Skill**: a folder containing `SKILL.md` (Agent Skills format) plus optional files (`scripts/`, `references/`, `assets/`, …).
- **Vault**: the folder holding the user's skills. Source of truth. Portable: no machine-specific state inside it.
- **Agent / target**: an agent's user-level skills directory. Sapato copies skills into it.
- **Managed skill**: a skill in a target that Sapato installed (recorded in the target's lock file).

## 2. Config — `~/.config/sapato/config.toml`

Override the path with the global `--config <path>` flag. No env vars. Missing config file = all defaults.

```toml
vault = "~/sapato-vault"        # default if omitted

[agents.claude]                 # every agent section is optional
path = "~/.claude/skills"       # override default path
enabled = true                  # default true
```

`~` is expanded to the user home dir in every path.

### Built-in agents

| id         | default path                  |
|------------|-------------------------------|
| `claude`   | `~/.claude/skills`            |
| `cursor`   | `~/.cursor/skills`            |
| `pi`       | `~/.pi/agent/skills`          |
| `opencode` | `~/.config/opencode/skills`   |
| `agents`   | `~/.agents/skills`            |

Known read overlaps (used by `doctor`): `cursor` also reads `~/.claude/skills`; `opencode` also reads `~/.claude/skills` and `~/.agents/skills`; `pi` also reads `~/.agents/skills`.

## 3. Vault format

```
<vault>/
  sapato.toml
  skills/
    <name>/
      SKILL.md
      ...
```

`sapato.toml`:

```toml
format = 1                      # required, integer; only 1 is supported

[assignments]                   # optional; desired state used by `sync`
claude = ["code-review", "obsidian-*"]   # skill names or globs (path.Match)
agents = ["*"]
```

### Validation rules (a skill failing any rule is *invalid*)

1. `skills/<name>/SKILL.md` exists and is a regular file.
2. `SKILL.md` starts with YAML frontmatter delimited by `---` lines.
3. Frontmatter has non-empty `name` and `description` strings.
4. `name` equals the folder name.
5. `name` matches `^[a-z0-9]+(-[a-z0-9]+)*$`, max 64 chars.
6. `description` max 1024 chars.
7. Optional `metadata.version` (string). Display only.

Entries in `skills/` that are not directories, or start with `.`, are ignored.

## 4. Content hash

`sha256` over the skill folder, deterministic:

- Walk all regular files, skip any path component starting with `.` (e.g. `.git`, `.DS_Store`) and skip symlinks.
- Sort by slash-separated relative path.
- For each file feed: `relpath + "\n" + ("x" if any exec bit else "-") + "\n" + <len as decimal> + "\n" + content`.
- Result string: `sha256:<hex>`.

## 5. Lock file — `<target>/.sapato-lock.json`

```json
{
  "version": 1,
  "skills": {
    "code-review": {
      "hash": "sha256:…",
      "version": "1.2.0",
      "installed_at": "2026-11-21T13:00:00Z"
    }
  }
}
```

Missing file = empty lock. Writes are atomic (write temp file in same dir, rename). Lock is only written when it changes.

## 6. Status

For each (skill, agent) pair, over the union of vault skills and target dirs:

| status       | meaning |
|--------------|---------|
| `not-installed` | in vault, not in target |
| `synced`     | in lock, target hash == lock hash == vault hash |
| `outdated`   | in lock, target hash == lock hash, vault hash differs |
| `modified`   | in lock, target hash != lock hash (local edits in target) |
| `orphaned`   | in lock, present in target, absent from vault |
| `unmanaged`  | in target, not in lock |
| `missing`    | in lock, folder gone from target |

`modified` takes precedence over `outdated`. Target subdirs starting with `.` are ignored.

## 7. Commands

Global flags: `--config`, `--dry-run` (on mutating commands: print plan, change nothing).
Agent selection: `-a/--agent <id>` repeatable, or `--all` (all enabled agents). Skill args accept globs.

| command | behavior |
|---------|----------|
| `sapato config` | print resolved config (vault + agents with paths, enabled) |
| `sapato init [path]` | create a vault (`sapato.toml` + `skills/`) at path or configured vault; fail if `sapato.toml` exists |
| `sapato list` | list vault skills: name, version, hash (short), valid/invalid + reason |
| `sapato status [skill…] [-a …]` | table of skill × agent status; `--json` |
| `sapato install <skill…> -a …` | copy vault skill to target; record lock. Refuses if target folder exists and is `unmanaged` or `modified` unless `--force`. Invalid skills are never installed |
| `sapato uninstall <skill…> -a …` | remove managed skill folder + lock entry. Refuses `unmanaged` (never delete what we don't own) and `modified` unless `--force` |
| `sapato update [skill…] [-a …]` | re-install every `outdated` skill (default: all skills, all agents). `modified` skipped unless `--force` |
| `sapato diff <skill> -a <id>` | unified diff vault vs installed copy (file list + per-file text diff) |
| `sapato import <path-or-git-url> [--name n]` | copy a skill folder into the vault (git URLs: shallow clone to temp dir, optional `--subdir`). Validates first; refuses if name exists unless `--force` |
| `sapato adopt <skill> -a <id>` | copy an `unmanaged` target skill into the vault, then record it in that target's lock (it becomes `synced`) |
| `sapato remove <skill>` | delete skill from vault. Refuses if installed anywhere unless `--force` (installed copies become `orphaned`) |
| `sapato sync` | apply `[assignments]`: install/update assigned skills; uninstall managed skills no longer assigned (`--prune`, off by default) |
| `sapato doctor` | report: invalid vault skills, corrupt locks, duplicate exposure via read overlaps (same skill visible to one agent from two dirs), missing/orphaned/modified entries |
| `sapato tui` (also bare `sapato`) | interactive UI |

Exit codes: 0 ok, 1 error, 2 usage error. Errors go to stderr.

## 8. Install mechanics

- Copy into `<target>/.<name>.sapato-tmp`, then remove the old folder (if any) and rename into place. Then update lock.
- Preserve file mode bits. Skip symlinks and dot-paths (same rule as hashing).
- Create target dir if missing.

## 9. TUI (Bubble Tea)

- Main view: matrix — rows = skills (vault ∪ found in targets), columns = enabled agents, cells = status glyph/color.
- Keys: arrows/hjkl move, `i` install, `u` uninstall, `U` update, `d` diff (pager), `a` adopt, `r` refresh, `/` filter, `?` help, `q` quit.
- Confirmation prompt before any destructive op; `--force`-type ops need explicit confirm.
- All mutations go through the same `ops` package as the CLI.

## 10. Code layout

```
cmd/sapato/main.go
internal/config     # config loading, defaults, ~ expansion
internal/vault      # vault + skill loading, frontmatter, validation
internal/hashdir    # content hash
internal/target     # agent registry, lock file, target scanning
internal/ops        # status, install, uninstall, update, diff, import, adopt, remove, sync, doctor
internal/cli        # cobra commands (thin: parse → ops → print)
internal/tui        # bubbletea app
```

Libraries: `github.com/spf13/cobra`, `github.com/pelletier/go-toml/v2`, `gopkg.in/yaml.v3`, `github.com/charmbracelet/bubbletea`, `bubbles`, `lipgloss`. Prefer stdlib otherwise.

`ops` functions take resolved paths (no global state, no reading `$HOME` directly) so tests run against `t.TempDir()`.

## 11. Out of scope (for now)

Symlink install mode, project-scoped targets (`./.claude/skills`), remote vaults/registries.
