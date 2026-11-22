```
    ███████╗ █████╗ ██████╗  █████╗ ████████╗ ██████╗
    ██╔════╝██╔══██╗██╔══██╗██╔══██╗╚══██╔══╝██╔═══██╗
    ███████╗███████║██████╔╝███████║   ██║   ██║   ██║
    ╚════██║██╔══██║██╔═══╝ ██╔══██║   ██║   ██║   ██║
    ███████║██║  ██║██║     ██║  ██║   ██║   ╚██████╔╝
    ╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝  ╚═╝   ╚═╝    ╚═════╝
Skills Administration, Packaging & Agent Tool Organization
```

Keep your agent skills in one portable folder (the *vault*) and install them into Claude, Cursor, pi, OpenCode and `.agents` from a TUI or the CLI. Installs are copies; Sapato tracks what it installed so it can update, diff and clean up safely.

## Quick start

```sh
make build                 # bin/sapato
make test                  # go test ./...
make run                   # TUI against a disposable copy of the mocked data
make run ARGS="status"     # any CLI command against the mocked data
make mock-reset            # throw away changes to the mocked data
```

`make run` works in `.work/mock/` with a fake `HOME`. It never touches `testdata/` or your real agent dirs.

Real use:

```sh
sapato init ~/sapato-vault
sapato import ./my-skill
sapato install my-skill -a claude -a cursor
sapato             # TUI
```

## Config

`~/.config/sapato/config.toml` (or `--config <path>`). Create a commented starter with `sapato config init`; the TUI offers to create it (and the vault) on first launch. Without a config, the built-in agents below are used.

```toml
vault = "~/sapato-vault"

[ui]
theme = "catppuccin-mocha"   # catppuccin-{latte,frappe,macchiato,mocha}, dracula, dracula-alucard, nord, nord-light
icons = "nerd"               # nerd | unicode | ascii (use unicode if glyphs show as boxes)

[agents]                     # name = path; order here = order in the UI
claude   = "~/.claude/skills"
cursor   = "~/.cursor/skills"
pi       = "~/.pi/agent/skills"
opencode = "~/.config/opencode/skills"
agents   = "~/.agents/skills"
# codex  = "~/.codex/skills"                                         # comment a line to stop managing it
# work   = { path = "~/work/.claude/skills", reads = ["~/.agents/skills"] }
```

Every agent stores skills the same way (`<path>/<skill>/…`), so supporting a new agent is one line. `reads` lists extra dirs an agent also reads, so `doctor` can flag skills it would see twice (built-ins already know theirs). Two agents can't share a path.

## Vault format

A plain folder: commit it, sync it, put it in your dotfiles. No machine-specific state lives in it.

```
<vault>/
  sapato.toml
  skills/
    <name>/
      SKILL.md        # required
      scripts/ references/ assets/ ...   # anything else
```

`sapato.toml`:

```toml
format = 1

[assignments]                 # optional; used by `sapato sync`
claude = ["code-review", "obsidian-*"]
agents = ["*"]
```

A skill is valid when:

1. `skills/<name>/SKILL.md` exists.
2. It starts with YAML frontmatter between `---` lines.
3. Frontmatter has non-empty `name` and `description`.
4. `name` equals the folder name.
5. `name` matches `^[a-z0-9]+(-[a-z0-9]+)*$`, max 64 chars.
6. `description` is at most 1024 chars.

Optional `metadata.version` is shown but never trusted. Versions are tracked by a content hash (dot-files and symlinks excluded). Invalid skills are never installed.

## TUI

Run `sapato` (or `make run` for the mocked data).

- **Main screen:** status grid of every skill. `↑↓`/`jk` move, `enter` open a skill, `/` filter, `r` refresh, `a` adopt an unmanaged skill, `n` new skill (paste a `SKILL.md`, `ctrl+s` saves), `I` import, `t` theme picker (live preview, not saved), `L` log, `?` help, `q` quit.
- **Skill screen:** agent sidebar (status per agent) and a details pane with version, hash, description, the selected agent's install info and a FILES tree (sizes, executables marked). `space` marks agents, then `i` install, `u` uninstall, `U` update, `d` diff, `a` adopt, `A` install everywhere, `x` export, `D` remove from vault, `pgup`/`pgdn` scroll details, `esc` back.
- Anything that would overwrite or delete local edits asks a separate, red **force** confirmation.
- Results show as notifications in the bottom-right (success/info 4s, errors 10s). `L` opens the session log.

## Status

Each install writes `<agent dir>/.sapato-lock.json`. Status compares the vault, the lock and the installed copy:

| glyph | status          | meaning |
|-------|-----------------|---------|
| `·`   | `not-installed` | in the vault, not in this agent |
| `✖`   | `invalid`       | in the vault but fails validation |
| `✔`   | `synced`        | installed and matching the vault |
| `↑`   | `outdated`      | the vault has newer content |
| `✎`   | `modified`      | the installed copy was edited locally |
| `∅`   | `orphaned`      | installed, but gone from the vault |
| `◆`   | `unmanaged`     | in the agent dir, not installed by Sapato |
| `!`   | `missing`       | in the lock, but the folder is gone |

## Commands

| command | does |
|---------|------|
| `config` | print the resolved config |
| `config init [--force]` | write a commented starter config to the `--config` path |
| `init [path]` | create a vault |
| `list` | list vault skills and validity |
| `status [skill…] [-a id…] [--json]` | skill × agent status |
| `install <skill…> -a id… \| --all` | copy skills into agents |
| `uninstall <skill…> -a id…` | remove managed skills |
| `update [skill…] [-a id…]` | re-install outdated skills |
| `diff <skill> -a id` | installed copy vs vault |
| `import <path\|git-url\|tarball> [--subdir d] [--name n]` | add skills to the vault |
| `new [file]` | create a skill from a complete `SKILL.md` (file or stdin) |
| `export <skill…> [-o file]` | write a `.skill.tar.gz` (one or more skills) |
| `adopt <skill> -a id` | pull an unmanaged agent skill into the vault |
| `remove <skill>` | delete a skill from the vault |
| `sync [--prune]` | apply `[assignments]` |
| `doctor` | invalid skills, broken locks, drift, skills an agent sees twice |
| `tui` (or no command) | interactive UI (`?` for keys) |

Global flags: `--config`, `--dry-run`, `--theme`, `--icons`. Skill args accept globs.

Safety rules:
- Sapato never deletes or overwrites a folder it didn't install.
- Overwriting or removing a locally modified copy needs `--force`. In the TUI it needs a separate force confirmation.
- When a glob matches an invalid or unmanaged skill, that skill is skipped. A skill you name exactly makes the command fail instead.

Full spec: [`docs/spec.md`](docs/spec.md).
