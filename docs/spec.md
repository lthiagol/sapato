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

[ui]
theme = "catppuccin-mocha"      # see §9.5; default catppuccin-mocha
icons = "nerd"                  # nerd | unicode | ascii; default nerd
```

The global `--icons` and `--theme` flags override `[ui]` for one run. Unknown theme or icon set → error listing the valid names.

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
| `not-installed` | in vault (valid), not in target |
| `invalid`    | in vault but fails validation, not in target |
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

Glob vs literal: a skill named literally that hits a hard refusal (invalid on install, unmanaged on uninstall) fails the whole call. A skill matched only by a glob in that situation is skipped and reported (`skip <skill> in <agent>: <reason>`). Refusals that `--force` can override (modified; unmanaged on install) always fail the call without `--force`, glob or not. Errors read `<skill> in <agent>: <status>`.

| command | behavior |
|---------|----------|
| `sapato config` | print resolved config (vault + agents with paths, enabled) |
| `sapato init [path]` | create a vault (`sapato.toml` + `skills/`) at path or configured vault; fail if `sapato.toml` exists |
| `sapato list` | list vault skills: name, version, hash (short), valid/invalid + reason |
| `sapato status [skill…] [-a …]` | table of skill × agent status; `--json` |
| `sapato install <skill…> -a …` | copy vault skill to target; record lock. Refuses if target folder exists and is `unmanaged` or `modified` unless `--force`. Invalid skills are never installed |
| `sapato uninstall <skill…> -a …` | remove managed skill folder + lock entry. Refuses `unmanaged` (never delete what we don't own) and `modified` unless `--force` |
| `sapato update [skill…] [-a …]` | re-install every `outdated` skill (default: all skills, all agents). `modified` skipped unless `--force` (reported as skips, never call failures, even for literal names) |
| `sapato diff <skill> -a <id>` | unified diff vault vs installed copy (file list + per-file text diff) |
| `sapato import <path-or-git-url-or-tarball> [--name n]` | copy a skill folder into the vault (git URLs: shallow clone to temp dir, optional `--subdir`; `*.tar.gz`/`*.tgz`: see §11). Validates first; refuses if name exists unless `--force` |
| `sapato new [file]` | create a vault skill from a complete `SKILL.md` (file or stdin). Name comes from frontmatter; must be valid; refuses an existing name unless `--force` |
| `sapato export <skill…> [-o file]` | write a tarball (§11). Default file: `<name>-<version or 8-char hash>.skill.tar.gz` for one skill, `skills-<date>.skill.tar.gz` for several. Invalid skills refused |
| `sapato adopt <skill> -a <id>` | copy an `unmanaged` target skill into the vault, then record it in that target's lock (it becomes `synced`) |
| `sapato remove <skill>` | delete skill from vault. Refuses if installed anywhere unless `--force` (installed copies become `orphaned`) |
| `sapato sync` | apply `[assignments]`: install/update assigned skills; uninstall managed skills no longer assigned (`--prune`, off by default) |
| `sapato doctor` | report: invalid vault skills, corrupt locks, duplicate exposure via read overlaps (same skill visible to one agent from two dirs), missing/orphaned/modified entries |
| `sapato tui` (also bare `sapato`) | interactive UI |

Notes: `remove` counts an agent as "installed" if status is synced/outdated/modified in any configured agent (enabled or not); `missing` does not count. `adopt --force` overwrites a same-named vault skill. `import` takes the skill name from frontmatter (`--name` must match it).

`sync` rules: acts only on agents with an `[assignments]` key (others never pruned); a disabled agent's assignments are reported as skips; literal/glob and refusal rules match install/update/uninstall; plans everything (dry-run) before applying. `--force`/`--prune` are CLI flags.

`doctor` severities: `error` = invalid vault skill, corrupt lock (exit 1); `warn` = modified, missing, orphaned, overlap. Findings go to stdout.

Overlaps (§2) are about fixed directories the agent itself reads (e.g. cursor always reads `~/.claude/skills`, wherever sapato's `claude` target is configured). Resolve those extra-read dirs from the home dir, and compare each extra-read dir to that agent's own path (and to its other read dirs), symlinks resolved.

Exit codes: 0 ok, 1 error, 2 usage error. Errors go to stderr.

## 8. Install mechanics

- Copy into `<target>/.<name>.sapato-tmp`, then remove the old folder (if any) and rename into place. Then update lock.
- Preserve file mode bits. Skip symlinks and dot-paths (same rule as hashing).
- Create target dir if missing.

## 9. TUI (Bubble Tea)

All mutations go through `ops`, exactly like the CLI. Force-overridable refusals (`ops.ErrNeedsForce`, update skip `modified`) always get a distinct force confirmation; plain destructive ops get a plain confirmation.

### 9.1 Layout (every screen)

1. **Header**, centered: the ANSI Shadow `SAPATO` art (exact text in `internal/tui/art.go`, see task 10) in the theme accent, then the tagline `Skills Administration, Packaging & Agent Tool Organization` in subtext. If the terminal is shorter than 30 rows or narrower than the art, use a one-line header: `sapato · <tagline>` (tagline dropped if it doesn't fit).
2. **Rule** (full width, overlay color).
3. **Screen body** (below).
4. **Bottom bar** (full width, 2 rows, Mantle background, no powerline glyphs anywhere):
   - Row 1: mode badge (` MAIN ` etc.: bold, Base text on the mode color, §9.4), a space, then the counts in subtext (`15 skills · 6 invalid · 4 need attention`).
   - Row 2 (1-col left pad, like row 1): the context key hints in overlay color, joined with ` · `. The global keys (`esc back` where it applies, `L log`, `? help`, `q quit`) are always shown at the end of the row; if the row is too narrow, the screen-specific hints before them are truncated with `…`.
   - Row 1 keeps the counts in every mode; FILTER appends ` · / <query>` after them.
   - The bar shows no action messages; those are notifications (§9.6).

### 9.2 Main screen (mode `MAIN`)

- Grid: a column-header row between two rules, then the skill rows in a scrolling viewport. Columns: `SKILL` (width = longest name, capped; overflow truncated with `…`), then one column per enabled agent (width = max(len(id), 3) + 2; icon centered). The whole grid block is horizontally centered.
- Cursor is per row (skill). No column cursor. Selected row uses the surface color background and a `▶`/`>` marker.
- Status icon per cell (§9.5) colored by status. Empty cell (pair absent everywhere) = blank.
- Keys: `↑↓`/`jk` move, `g`/`G` top/bottom, `pgup`/`pgdn`, `enter` open skill screen, `/` filter (substring; `esc` clears), `r` refresh, `a` adopt picker, `n` new skill, `I` import tarball (path prompt), `t` theme picker, `?` help, `q` quit.
- Bar counts (row 1): `N skills · N invalid · N need attention` (attention = outdated + modified + orphaned + missing).

### 9.3 Skill screen (mode `SKILL`)

- Layout: two bordered panes (rounded border, Overlay color; the focused sidebar's border in Accent), filling the body width with a 1-col outer margin. Pane titles sit in the top border (` AGENTS `, ` <skill name> `).
- **Sidebar (left)**: width = widest agent row + borders, at least 26. One row per enabled agent: mark (§9.5), cursor marker, agent id, status icon + short status word (`not inst.` for not-installed). Cursor per agent row. `space` toggles a mark; actions apply to marked rows, or to the cursor row if none marked.
- **Details (right)**, takes the remaining width; scrolls with `pgup`/`pgdn` if taller than the body:
  1. `version <v|->   hash <short>   <valid icon> valid|invalid`
  2. description, wrapped to the pane, max 3 lines (`…` when cut)
  3. `vault  <path>` (or `vault  not in vault` with the value dimmed); labels share one column width; each problem in red if invalid
  4. blank line, then the cursor agent's section: `<agent>  <icon> <status>` + ` · installed <YYYY-MM-DD HH:MM>` (local time) from the lock when managed, then the installed path on the next line (indented)
  5. blank line, then `FILES (N)` and a tree of the skill's files using the hashing walk (§4: no dot-paths, no symlinks): a root `SKILL.md` first, then directories as `name/` before files, each sorted by name, children indented 2 spaces per level; each file shows the exec glyph (§9.5) if executable and a right-aligned human size (`B`, `KiB`, `MiB`, one decimal above 1 KiB). Files come from the vault copy; if the skill isn't in the vault, from the cursor agent's installed copy, titled `FILES (N, installed in <agent>)`; if neither exists, `no files`.
- Narrow terminals (< 70 cols): sidebar stacked above details, both full width.
- All widths via `lipgloss.Width`; long values truncated with `…`; nothing exceeds the terminal.
- Keys: `i` install, `u` uninstall, `U` update, `d` diff (pager), `a` adopt (only on `unmanaged`), `A` install into all enabled agents, `x` export (path prompt, default name per §7), `D` remove from vault (confirm; force confirm if installed anywhere), `esc` back, `?` help, `q` quit.
- Rows for skills not in the vault (orphaned/unmanaged/missing only) still open; actions that need the vault are disabled with an error notification.

### 9.4 Dialogs and modes

Every dialog (ADOPT, NEW, IMPORT, EXPORT, THEME, HELP, confirm/force confirm) renders as one horizontally centered block, like the grid: block width = widest line, at least 60, at most the terminal width; long values truncated with `…`. Dialogs with an input (NEW, IMPORT, EXPORT) are two thirds of the terminal wide (at least 60, at most the terminal) and the input fills the block.

- `ADOPT`: list of every `unmanaged` (skill, agent, path) across enabled agents; `enter` adopts (force confirm on `ErrNeedsForce`), `esc` back.
- `NEW`: multi-line textarea to paste a complete `SKILL.md`. Live validation line under it (valid, or the first problem). `ctrl+s` saves via `ops.Create`; `esc` cancels (confirm if non-empty).
- `IMPORT` / `EXPORT`: single-line path input with default; `enter` runs; results in the bar.
- `THEME`: list of themes; moving the cursor previews live; `enter` keeps it for this session, `esc` restores the previous one. Never writes the config file.
- `HELP`: keys for the current screen + icon/status legend.
- Mode badge colors: MAIN=blue/accent, SKILL=green, FILTER=yellow, ADOPT=mauve/purple, NEW=peach/orange, IMPORT/EXPORT=teal/cyan, THEME=pink, HELP=overlay.

### 9.5 Themes and icons

Themes (palettes in `docs/themes.md`): `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha` (default), `dracula`, `dracula-alucard`, `nord`, `nord-light`. Each theme fills the same semantic roles; UI code uses roles only. `NO_COLOR` disables color (layout unchanged).

Icon sets (per status, see §6):

| status | nerd | unicode | ascii |
|---|---|---|---|
| synced | `\uf058` | `✔` | `ok` |
| outdated | `\uf0aa` | `↑` | `up` |
| modified | `\uf040` | `✎` | `mod` |
| not-installed | `\uf10c` | `·` | `.` |
| invalid | `\uf057` | `✖` | `x` |
| orphaned | `\uf127` | `∅` | `orp` |
| unmanaged | `\uf059` | `◆` | `?` |
| missing | `\uf071` | `!` | `!` |

Row marker: nerd `\uf054`, unicode `▶`, ascii `>`. Mark: nerd `\uf14a`, unicode `■`, ascii `*`. Executable file: nerd `\uf013`, unicode `⚙`, ascii `x`. Info notification: nerd `\uf05a`, unicode `ℹ`, ascii `i`. Icon sets carry no separators.

### 9.6 Notifications

- Every action result, error and info message (install, diff refusal, refresh, theme kept, export written, …) becomes a notification: `kind` (success | error | info), title (short: the op, e.g. `installed`, `error`), message, timestamp.
- Shown as a stack of rounded-border boxes in the bottom-right corner of the body, directly above the bar, 1-col right margin, drawn over the body. Box width = min(48, terminal width / 2), at least 24; message wrapped inside, max 4 lines (`…` when cut). Top border carries ` <icon> <title> ` (icon = synced icon for success, invalid icon for error, info icon); border + title colored Green / Red / Accent.
- At most 3 visible; newest at the bottom; older ones drop off the top. Success/info expire after 4s, errors after 10s (driven by `tea.Tick`, testable with an injectable clock).
- `L` (on every screen, outside text inputs) opens `LOG` mode: a centered dialog (§9.4 rules) listing every notification this session, oldest first, `HH:MM:SS <icon> <title>: <message>`, scrollable, max 200 kept. `esc` closes. LOG badge color = Overlay.
- Notifications never write to disk.

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

## 11. Skill tarballs

- gzip-compressed tar. Top-level entries are skill folders `<name>/…`; one archive may hold several skills. Nothing else at top level (a `sapato-export.json` manifest with `{"format":1,"skills":{name:hash}}` is written on export and ignored on import if absent).
- Export: files per hashing rules (§4: no dot-paths, no symlinks), modes preserved (0755 for exec, 0644 otherwise), dirs 0755, deterministic order, mtime zero.
- Import safety: reject absolute paths, `..` components, symlinks/hardlinks/devices, entries outside a top-level skill folder, total uncompressed size > 50 MiB, > 10k entries. Extract to a temp dir, validate every skill, then copy all-or-nothing into the vault (existing names refused unless `--force`; `--name` only allowed for single-skill archives).

## 12. Out of scope (for now)

Symlink install mode, project-scoped targets (`./.claude/skills`), remote vaults/registries.
