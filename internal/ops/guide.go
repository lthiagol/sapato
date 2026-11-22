package ops

import (
	"fmt"
	"strings"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/vault"
)

// guideSkill is the name of the vault skill that GuideInstall writes.
const guideSkill = "sapato"

// guideTemplate is the body of the Guide. {{vault}} is the vault path.
const guideTemplate = `---
name: sapato
description: Create, edit, import, check, scope and install agent skills in a sapato skill vault with the sapato CLI. Use when asked to write, add, import or install a skill for this vault, or to check or fix its skills.
---

# sapato

Sapato keeps a skill vault, the source of truth for the skills on this machine, and installs copies of its skills into each configured agent's skills directory. Edit skills in the vault, not in an agent's copy, then update the installed copies.

## Vault

Vault path: {{vault}}

Layout:

    sapato.toml                 format = 1, [assignments], [skills.<name>] compatibility tables
    skills/<name>/SKILL.md      one folder per skill
    skills/<name>/...           optional scripts/, references/, assets/

Folders in skills/ that start with a dot are ignored.

## Skill rules

A skill is valid only if all of these hold:

1. skills/<name>/SKILL.md exists and is a regular file.
2. SKILL.md starts with YAML frontmatter between --- lines.
3. The frontmatter has non-empty name and description strings.
4. name equals the folder name.
5. name matches ^[a-z0-9]+(-[a-z0-9]+)*$ and is at most 64 characters.
6. description is at most 1024 characters.
7. Optional metadata.version (a string), shown only.

Write the description so it says when to use the skill. Put the instructions in the body.

## Compatibility

A skill works with every agent unless sapato.toml has a [skills.<name>] table with compatible = [...].

    sapato compat <skill>               print the compatible agents
    sapato compat <skill> <agent>...    set exactly these agents
    sapato compat <skill> --all         every agent again (removes the table)

Installing into an incompatible agent is refused unless --force. --all and sync skip such pairs.

## Create

    sapato new --name <name> --description "<text>" [--compatible a,b]
    sapato new <file>                   add a complete SKILL.md; the name comes from its frontmatter
    sapato new < SKILL.md               the same, read from stdin

Both refuse a name that is already in the vault unless --force is given. The --name form writes the template: fill in the body after it.

## Import

    sapato import <source> [--name n] [--subdir dir] [--all] [--fix] [--description text] [--force]

Sources: a local skill folder, a local SKILL.md file, a raw https URL ending in .md, a git URL, or a .tar.gz or .tgz tarball. A source holding several skills needs --subdir (one of them) or --all (every one, all or nothing). --fix repairs an invalid name or description in the vault copy; --description supplies a missing description. The source is never modified. --fix does not apply to tarballs.

## Install and check

    sapato install <skill>... -a <agent>      copy into agents (-a repeats; --all means every agent)
    sapato update [skill...] [-a <agent>]     re-install outdated copies
    sapato uninstall <skill>... -a <agent>
    sapato sync                               apply [assignments] in sapato.toml
    sapato list                               vault skills and whether they are valid
    sapato status [skill...] [-a <agent>] [--json]
    sapato diff <skill> -a <agent>            vault copy against the installed copy
    sapato doctor                             invalid, modified, missing, orphaned and overlapping entries
    sapato adopt <skill>... -a <agent>        bring an unmanaged agent skill into the vault

Installing refuses an unmanaged or modified folder unless --force is given. Skill names accept globs. Add --dry-run to any mutating command to preview it.

## Configured agents

Agents in order, with their skills directories:

`

// Guide returns the SKILL.md that teaches an agent to work with the vault at
// vaultPath and the configured agents, listed in the order given (spec §7). The
// skill is valid by the rules of spec §3.
func Guide(vaultPath string, agents []config.Agent) []byte {
	var b strings.Builder
	b.WriteString(strings.Replace(guideTemplate, "{{vault}}", vaultPath, 1))
	if len(agents) == 0 {
		b.WriteString("No agents are configured.\n")
	}
	for _, a := range agents {
		fmt.Fprintf(&b, "- %s: %s\n", a.ID, a.Path)
	}
	return []byte(b.String())
}

// GuideInstall writes the Guide as the vault skill sapato, in skills/sapato
// (spec §7). A folder that already holds exactly that content is left alone and
// returned as a skip. A folder with other content is refused unless opts.Force is
// set. A dry run changes nothing.
func GuideInstall(v *vault.Vault, agents []config.Agent, opts Options) (Action, error) {
	content := Guide(v.Path, agents)
	same, err := guideCurrent(v.SkillDir(guideSkill), content)
	if err != nil {
		return Action{}, fmt.Errorf("guide: %w", err)
	}
	if same {
		return Action{Op: OpSkip, Skill: guideSkill, Path: v.SkillDir(guideSkill), Reason: "already up to date"}, nil
	}
	action, err := place(v, guideSkill, content, opts, nil)
	if err != nil {
		return Action{}, fmt.Errorf("guide: %w", err)
	}
	return action, nil
}

// guideCurrent reports whether the folder dest holds exactly the skill made of
// content: the same content hash as a folder holding only that SKILL.md.
func guideCurrent(dest string, content []byte) (bool, error) {
	present, err := pathExists(dest)
	if err != nil || !present {
		return false, err
	}
	stage, cleanup, err := stageSkillFile(guideSkill, content)
	if err != nil {
		return false, err
	}
	defer cleanup()
	want, err := hashdir.Hash(stage)
	if err != nil {
		return false, err
	}
	have, err := hashdir.Hash(dest)
	if err != nil {
		return false, err
	}
	return have == want, nil
}
