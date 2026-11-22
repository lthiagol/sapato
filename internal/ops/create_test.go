package ops

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/vault"
)

const newSkillMD = "---\nname: new-skill\ndescription: A new skill.\n---\n\n# New\n"

func TestCreate(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		opts      Options
		wantName  string
		wantErr   string
		wantForce bool
	}{
		{
			name:     "valid content is created under its frontmatter name",
			content:  newSkillMD,
			wantName: "new-skill",
		},
		{
			name:     "dry run plans the create and changes nothing",
			content:  newSkillMD,
			opts:     Options{DryRun: true},
			wantName: "new-skill",
		},
		{
			name:      "name already in the vault is refused",
			content:   "---\nname: mocked-skill-00004\ndescription: Again.\n---\n",
			wantErr:   "already in the vault (use --force)",
			wantForce: true,
		},
		{
			name:     "force replaces the vault skill",
			content:  "---\nname: mocked-skill-00004\ndescription: Again.\n---\n",
			opts:     Options{Force: true},
			wantName: "mocked-skill-00004",
		},
		{
			name:    "missing description is refused",
			content: "---\nname: new-skill\n---\n",
			wantErr: "frontmatter description must be a non-empty string",
		},
		{
			name:    "invalid name is refused",
			content: "---\nname: New_Skill\ndescription: A new skill.\n---\n",
			wantErr: "must match",
		},
		{
			name:    "name with a path part is refused",
			content: "---\nname: ../escape\ndescription: A new skill.\n---\n",
			wantErr: "must match",
		},
		{
			name:    "no frontmatter is refused",
			content: "# New\n",
			wantErr: "missing opening --- line",
		},
		{
			name:    "empty input is refused",
			content: "",
			wantErr: "invalid skill",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			v := fixtureVault(t)
			before := snapshot(t, v.Path)

			action, err := Create(v, []byte(tt.content), tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Create() error = %v, want one containing %q", err, tt.wantErr)
				}
				if got := errors.Is(err, ErrNeedsForce); got != tt.wantForce {
					t.Errorf("errors.Is(err, ErrNeedsForce) = %v, want %v", got, tt.wantForce)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a refused create")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			wantAction := Action{Op: OpCreate, Skill: tt.wantName, Path: v.SkillDir(tt.wantName)}
			if action != wantAction {
				t.Errorf("action = %+v, want %+v", action, wantAction)
			}
			if tt.opts.DryRun {
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a dry run")
				}
				return
			}

			got := vaultSkill(t, v, tt.wantName)
			if !got.Valid {
				t.Errorf("created skill is invalid: %v", got.Problems)
			}
			data, err := os.ReadFile(filepath.Join(v.SkillDir(tt.wantName), "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.content {
				t.Errorf("SKILL.md = %q, want the input %q", data, tt.content)
			}
			if left, _ := os.ReadDir(os.Getenv("TMPDIR")); len(left) != 0 {
				t.Errorf("staging temp dir not removed: %d entries left", len(left))
			}
		})
	}
}

func TestCreateForceFailureKeepsOldCopy(t *testing.T) {
	v := fixtureVault(t)
	before := snapshot(t, v.Path)
	content := "---\nname: mocked-skill-00004\ndescription: Replacement.\n---\n"

	renamePath = func(string, string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { renamePath = os.Rename })

	_, err := Create(v, []byte(content), Options{Force: true})
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("Create() error = %v, want the injected rename failure", err)
	}
	if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
		t.Error("failed forced create changed the vault")
	}
}

// scaffoldText is the SKILL.md that Scaffold writes for name new-skill and the
// description "A new skill.", as spec §7.2 lays it out.
const scaffoldText = `---
name: new-skill
description: A new skill.
---

# new-skill

<!-- One or two sentences: what this skill does. -->

## When to use

<!-- The situations that should trigger this skill. -->

## Steps

1. <!-- What the agent does, in order. -->

## Notes

<!-- Constraints, gotchas, files in this folder (scripts/, references/, assets/). -->
`

func TestScaffold(t *testing.T) {
	tests := []struct {
		name        string
		skill       string
		description string
		compatible  []string
		setup       func(t *testing.T, v *vault.Vault)
		opts        Options
		wantErr     string
		wantForce   bool
		wantText    string   // the SKILL.md written, when the call succeeds
		wantCompat  []string // the compatible list after the call
		wantToml    string   // a line the sapato.toml must hold, when set
	}{
		{
			name:        "template with every agent writes no table",
			skill:       "new-skill",
			description: "A new skill.",
			wantText:    scaffoldText,
		},
		{
			name:        "description needing quotes is YAML-quoted and still valid",
			skill:       "new-skill",
			description: `Use when: "x" # y`,
			wantText:    strings.Replace(scaffoldText, "description: A new skill.", `description: 'Use when: "x" # y'`, 1),
		},
		{
			name:        "name that reads as a YAML number is quoted",
			skill:       "123",
			description: "A new skill.",
			wantText:    strings.Replace(strings.ReplaceAll(scaffoldText, "new-skill", "123"), "name: 123", `name: "123"`, 1),
		},
		{
			name:        "compatible list writes the table",
			skill:       "new-skill",
			description: "A new skill.",
			compatible:  []string{"claude", "cursor"},
			wantText:    scaffoldText,
			wantCompat:  []string{"claude", "cursor"},
			wantToml:    "[skills.new-skill]\ncompatible = [\"claude\", \"cursor\"]\n",
		},
		{
			name:        "dry run plans the scaffold and changes nothing",
			skill:       "new-skill",
			description: "A new skill.",
			compatible:  []string{"claude"},
			opts:        Options{DryRun: true},
		},
		{
			name:        "name already in the vault is refused",
			skill:       "mocked-skill-00004",
			description: "Again.",
			wantErr:     "already in the vault (use --force)",
			wantForce:   true,
		},
		{
			name:        "force replaces the skill and drops its old table",
			skill:       "mocked-skill-00004",
			description: "Again.",
			setup: func(t *testing.T, v *vault.Vault) {
				setCompatFor(t, v, "mocked-skill-00004", "claude")
			},
			opts:     Options{Force: true},
			wantText: strings.NewReplacer("new-skill", "mocked-skill-00004", "A new skill.", "Again.").Replace(scaffoldText),
		},
		{
			name:        "empty description is refused",
			skill:       "new-skill",
			description: "",
			wantErr:     "frontmatter description must be a non-empty string",
		},
		{
			name:        "description over 1024 characters is refused",
			skill:       "new-skill",
			description: strings.Repeat("d", 1025),
			wantErr:     "max 1024",
		},
		{
			name:        "invalid name is refused",
			skill:       "New_Skill",
			description: "A new skill.",
			wantErr:     "must match",
		},
		{
			name:        "empty compatible list is refused",
			skill:       "new-skill",
			description: "A new skill.",
			compatible:  []string{},
			wantErr:     "compatible list is empty",
		},
		{
			name:        "invalid agent name in the compatible list is refused",
			skill:       "new-skill",
			description: "A new skill.",
			compatible:  []string{"Bad Name"},
			wantErr:     "agent name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			v := fixtureVault(t)
			if tt.setup != nil {
				tt.setup(t, v)
			}
			before := snapshot(t, v.Path)
			tomlBefore := tomlText(t, v)

			action, err := Scaffold(v, tt.skill, tt.description, tt.compatible, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Scaffold() error = %v, want one containing %q", err, tt.wantErr)
				}
				if got := errors.Is(err, ErrNeedsForce); got != tt.wantForce {
					t.Errorf("errors.Is(err, ErrNeedsForce) = %v, want %v", got, tt.wantForce)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a refused scaffold")
				}
				return
			}
			if err != nil {
				t.Fatalf("Scaffold() error = %v", err)
			}
			if want := (Action{Op: OpCreate, Skill: tt.skill, Path: v.SkillDir(tt.skill)}); action != want {
				t.Errorf("action = %+v, want %+v", action, want)
			}
			if tt.opts.DryRun {
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a dry run")
				}
				return
			}

			data, err := os.ReadFile(filepath.Join(v.SkillDir(tt.skill), "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.wantText {
				t.Errorf("SKILL.md =\n%s\nwant\n%s", data, tt.wantText)
			}
			if got := vaultSkill(t, v, tt.skill); !got.Valid {
				t.Errorf("scaffolded skill is invalid: %v", got.Problems)
			}
			if got := v.CompatAgents(tt.skill); !slices.Equal(got, tt.wantCompat) {
				t.Errorf("compatible = %q, want %q", got, tt.wantCompat)
			}
			if tt.wantToml != "" && !strings.Contains(tomlText(t, v), tt.wantToml) {
				t.Errorf("sapato.toml lacks %q:\n%s", tt.wantToml, tomlText(t, v))
			}
			if tt.compatible == nil && tt.setup == nil && tomlText(t, v) != tomlBefore {
				t.Errorf("sapato.toml changed without a compatible list:\n%s", tomlText(t, v))
			}
		})
	}
}

func TestEditorCommand(t *testing.T) {
	tests := []struct {
		name   string
		visual string
		editor string
		want   []string
	}{
		{name: "VISUAL wins over EDITOR", visual: "hx", editor: "nano", want: []string{"hx", "SKILL.md"}},
		{name: "EDITOR when VISUAL is unset", editor: "nano", want: []string{"nano", "SKILL.md"}},
		{name: "vi when neither is set", want: []string{"vi", "SKILL.md"}},
		{name: "blank VISUAL falls through to EDITOR", visual: "  ", editor: "nano", want: []string{"nano", "SKILL.md"}},
		{name: "editor arguments stay before the path", visual: "code --wait", want: []string{"code", "--wait", "SKILL.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EditorCommand(tt.visual, tt.editor, "SKILL.md"); !slices.Equal(got, tt.want) {
				t.Errorf("EditorCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
