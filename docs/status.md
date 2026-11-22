# Project status

Start here when resuming. Spec: `docs/spec.md`. Palettes: `docs/themes.md`. Task specs: `docs/tasks/`.

## Done

All tasks 01–15 are approved and committed (one commit per task). The CLI (§7) and TUI v2 (§9) are complete:

- **Vault and lock:** content hashing and per-agent `.sapato-lock.json`.
- **Commands:** install, uninstall, update, diff, adopt, import, new, export, remove, sync and doctor. Imports accept a skill folder, a git URL or a tarball.
- **Config:** agents are data. The `[agents]` table maps `name = path`, or an inline `{ path, reads }`. File order is kept. With no `[agents]`, the built-in defaults are used. `sapato config init` writes a commented starter file.
- **TUI:**
  - Main grid and skill screen. The skill screen has an agent sidebar, a details pane and a FILES tree.
  - Dialogs: adopt, new, import, export and the theme picker.
  - Modal y/n confirms drawn over a dimmed screen. Force confirms have a red border.
  - Two-row bottom bar, toast notifications and an `L` log.
  - 8 themes, with nerd, unicode and ascii icon sets.
  - On first launch with no config, it offers to create the config, then the vault.
- **Compatibility core (task 16):** `[skills.<name>] compatible` in `sapato.toml`, the `compat` command, the `incompatible` status, install and sync skips, doctor findings, and the compat list in tarball exports and imports.
- **Compatibility in the TUI (task 17):** the `incompatible` icon in all three sets, sidebar and details marks, the `compat` line, `c` to toggle the marked or cursor agents, and `A` skipping incompatible agents with a report.
- **Import sources and `--fix` (task 18):** local folder, `SKILL.md` file, raw `SKILL.md` URL, git, tarball. Discovery with `--subdir`, `--all` and a several-skills error. `--fix` and `--description` repair the vault copy. The ops discovery API is `ops.Discover` (for the TUI, task 19).
- **IMPORT dialog (task 19):** source input with a `ctrl+o` file picker, a multi-select list for several skills (`ops.ImportOptions.Pick`, all or nothing), a fix-up form for `--fix`, and the force prompt for names already in the vault.
- **New-skill scaffold and wizard (task 20):** `new --name --description [--compatible a,b] [--edit]` writes the §7.2 template. The TUI NEW wizard takes a name, a description and the agents, then `ctrl+s` scaffolds and offers to open the file in the editor. `tab` switches to the paste mode.
- **Adopt (task 21):** identical copies in other agents become synced too. `adopt <skill…> -a <id>` takes globs over that agent's unmanaged skills. The ADOPT dialog groups copies by agent, with a filter row, `space` marks, a `conflict` tag and one summary notification.

## Open nits (optional, from the last review)

- `internal/tui/dialogs_view.go`, confirm box: at 60 columns a long question is truncated. It could wrap to 2 lines instead.
- The phrase "has local edits" is written in both `internal/ops` (`install.go`, `uninstall.go`, `doctor.go`) and `internal/tui/model.go`.
- The empty-vault message on the main screen is hard-clipped at 60 columns, with no `…`.

## Not started

- Symlink install mode. Installs are copies for now; the user wants symlinks later.

## Behaviour notes

- `D` removes a skill from the vault only. Its installed copies become `orphaned`.
- Uninstalling an orphaned copy leaves that cell `absent`, shown blank in the grid. The skill row disappears once no agent has it.
- `make run` uses a disposable copy of the fixtures in `.work/mock` with a fake `HOME`. `make mock-reset` restores it.

## How work is done

The orchestrator (the main Claude session) designs the work, writes `docs/tasks/NN-*.md` and amends `docs/spec.md` first. Then it runs the build/review loop in herdr. Each project gets its own herdr workspace, named after the project.

- **Builders:** `claude --model claude-haiku-5-5 --effort high --permission-mode auto`. Use a fresh builder every cycle.
- **Reviewer:** `claude --model claude-opus-5-5 --effort medium --permission-mode auto`. Use one reviewer per task, kept across that task's cycles.
- **Cycle notes:** builders write `.work/handoff/<task>-c<N>.md` and reviewers write `.work/reviews/<task>-c<N>.md`. `.work/` is gitignored.
- **Cycle cap:** at most 5 cycles per task. If a task isn't approved after 5, escalate to the user.
- **Before committing:**
  - The orchestrator smoke-tests the real TUI in a split pane, using `make run`.
  - It then commits once per approved task and closes the builder and reviewer tabs.
- **Never touch the real agent dirs or `~/.config/sapato`.** The user has neither a sapato config nor a vault yet.
