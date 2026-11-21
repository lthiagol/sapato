package ops

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
