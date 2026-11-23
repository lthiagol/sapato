package ops

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/target"
	"sapato/internal/vault"
)

// snapshot records every entry under dir with its mode and content, so a test
// can check that a call changed nothing.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := make(map[string]string)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			snap[rel] = "dir " + info.Mode().String()
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		snap[rel] = info.Mode().String() + " " + string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return snap
}

// stateOf returns the status of skill in agent.
func stateOf(t *testing.T, v *vault.Vault, agent config.Agent, skill string) State {
	t.Helper()
	rows, err := Status(v, []config.Agent{agent}, Filter{Skills: []string{skill}})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows for %s, want 1", len(rows), skill)
	}
	return rows[0].Status
}

// vaultSkill returns the vault skill called name.
func vaultSkill(t *testing.T, v *vault.Vault, name string) vault.Skill {
	t.Helper()
	skills, err := v.Skills()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("vault has no skill %s", name)
	return vault.Skill{}
}

func TestInstallFreshAndReinstall(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")

	actions, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00004"}, Options{})
	if err != nil {
		t.Fatalf("fresh install: %v", err)
	}
	want := []Action{{Op: OpInstall, Skill: "mocked-skill-00004", Agent: "claude", Path: filepath.Join(agent.Path, "mocked-skill-00004")}}
	if !reflect.DeepEqual(actions, want) {
		t.Errorf("actions = %+v, want %+v", actions, want)
	}
	if got := stateOf(t, v, agent, "mocked-skill-00004"); got != Synced {
		t.Errorf("after fresh install status = %s, want synced", got)
	}

	actions, err = Install(v, []config.Agent{agent}, []string{"mocked-skill-00001"}, Options{})
	if err != nil {
		t.Fatalf("reinstall over synced: %v", err)
	}
	if len(actions) != 1 {
		t.Errorf("reinstall actions = %+v, want one", actions)
	}
	if got := stateOf(t, v, agent, "mocked-skill-00001"); got != Synced {
		t.Errorf("after reinstall status = %s, want synced", got)
	}
}

func TestInstallRefusesConflicts(t *testing.T) {
	tests := []struct {
		name      string
		prep      func(t *testing.T, dir string)
		force     bool
		skill     string
		wantErr   string
		wantAfter State
	}{
		{
			name:    "modified without force is refused",
			skill:   "mocked-skill-00003",
			wantErr: "mocked-skill-00003 in claude: modified (use --force)",
		},
		{
			name:    "modified matched by glob is refused without force",
			skill:   "mocked-skill-0000[3]",
			wantErr: "mocked-skill-00003 in claude: modified (use --force)",
		},
		{
			name:      "modified with force is overwritten",
			skill:     "mocked-skill-00003",
			force:     true,
			wantAfter: Synced,
		},
		{
			name: "unmanaged without force is refused",
			prep: func(t *testing.T, dir string) {
				writeTestFile(t, filepath.Join(dir, "mocked-skill-00004", "SKILL.md"), "mine")
			},
			skill:   "mocked-skill-00004",
			wantErr: "mocked-skill-00004 in claude: unmanaged (use --force)",
		},
		{
			name: "unmanaged with force is overwritten",
			prep: func(t *testing.T, dir string) {
				writeTestFile(t, filepath.Join(dir, "mocked-skill-00004", "SKILL.md"), "mine")
			},
			skill:     "mocked-skill-00004",
			force:     true,
			wantAfter: Synced,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			if tt.prep != nil {
				tt.prep(t, agent.Path)
			}
			before := snapshot(t, agent.Path)

			actions, err := Install(v, []config.Agent{agent}, []string{tt.skill}, Options{Force: tt.force})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				if actions != nil {
					t.Errorf("actions = %+v, want nil on refusal", actions)
				}
				if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
					t.Error("refused install changed the target")
				}
				return
			}
			if err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			if got := stateOf(t, v, agent, tt.skill); got != tt.wantAfter {
				t.Errorf("status after = %s, want %s", got, tt.wantAfter)
			}
		})
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRefusesInvalidAndUnknown(t *testing.T) {
	tests := []struct {
		name    string
		skills  []string
		agents  []config.Agent // nil uses the fixture claude target
		wantErr string
	}{
		{name: "invalid skill is refused", skills: []string{"invalid-name-mismatch"}, wantErr: "invalid skills are never installed"},
		{name: "unknown skill", skills: []string{"no-such-skill"}, wantErr: `no skill matches "no-such-skill"`},
		{name: "malformed glob", skills: []string{"["}, wantErr: "bad skill pattern"},
		{name: "no skills", skills: nil, wantErr: "no skills given"},
		{name: "no agents", skills: []string{"mocked-skill-00004"}, agents: []config.Agent{}, wantErr: "no agents selected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			agents := tt.agents
			if agents == nil {
				agents = []config.Agent{agent}
			}
			before := snapshot(t, agent.Path)
			_, err := Install(v, agents, tt.skills, Options{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
				t.Error("refused install changed the target")
			}
		})
	}
}

func TestInstallIsAllOrNothing(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before := snapshot(t, agent.Path)

	// mocked-skill-00004 is installable on its own, but the invalid skill in the
	// same call must stop the whole install.
	_, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00004", "invalid-name-mismatch"}, Options{})
	if err == nil {
		t.Fatal("Install() returned nil error")
	}
	if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
		t.Error("failed install changed the target")
	}
}

// summarize renders actions as sorted "op skill" lines, with ": reason" on skips.
func summarize(actions []Action) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		line := a.Op + " " + a.Skill
		if a.Op == OpSkip {
			line += ": " + a.Reason
		}
		out = append(out, line)
	}
	slices.Sort(out)
	return out
}

func TestInstallSkipsInvalidMatchedByGlob(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")

	actions, err := Install(v, []config.Agent{agent}, []string{"*"}, Options{Force: true})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	want := []string{
		"install mocked-skill-00001",
		"install mocked-skill-00002",
		"install mocked-skill-00003",
		"install mocked-skill-00004",
		"install mocked-skill-00005",
		"install mocked-skill-00006",
		"skip Invalid-Uppercase: invalid",
		"skip invalid-long-description: invalid",
		"skip invalid-missing-description: invalid",
		"skip invalid-name-mismatch: invalid",
		"skip invalid-no-frontmatter: invalid",
		"skip invalid-no-skill-md: invalid",
	}
	if got := summarize(actions); !reflect.DeepEqual(got, want) {
		t.Errorf("actions = %v, want %v", got, want)
	}
	if got := stateOf(t, v, agent, "mocked-skill-00003"); got != Synced {
		t.Errorf("status after = %s, want synced", got)
	}
	if _, err := os.Stat(filepath.Join(agent.Path, "invalid-name-mismatch")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("invalid skill was installed (err %v)", err)
	}
}

func TestInstallGlob(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")

	actions, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-0000[45]"}, Options{})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	var names []string
	for _, a := range actions {
		names = append(names, a.Skill)
	}
	if want := []string{"mocked-skill-00004", "mocked-skill-00005"}; !reflect.DeepEqual(names, want) {
		t.Errorf("installed %v, want %v", names, want)
	}
}

func TestInstallKeepsModeBits(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	// A umask of 022 would strip the group and other write bits at create time.
	wide := filepath.Join(vaultSkill(t, v, "mocked-skill-00004").Dir, "references", "wide.md")
	writeTestFile(t, wide, "wide")
	if err := os.Chmod(wide, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00004"}, Options{}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	src := vaultSkill(t, v, "mocked-skill-00004").Dir
	files, err := hashdir.Files(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		info, err := os.Stat(filepath.Join(agent.Path, "mocked-skill-00004", filepath.FromSlash(f.Rel)))
		if err != nil {
			t.Fatalf("%s not installed: %v", f.Rel, err)
		}
		if info.Mode().Perm() != f.Mode {
			t.Errorf("%s mode = %v, want %v", f.Rel, info.Mode().Perm(), f.Mode)
		}
	}
	if info, _ := os.Stat(filepath.Join(agent.Path, "mocked-skill-00004", "scripts", "run.sh")); info.Mode().Perm()&0o111 == 0 {
		t.Error("run.sh lost its exec bit")
	}
}

func TestInstallSkipsDotPathsAndSymlinks(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	if _, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00006"}, Options{}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	dest := filepath.Join(agent.Path, "mocked-skill-00006")
	for _, rel := range []string{".DS_Store", ".hidden", "references/.cache", "link.md"} {
		if _, err := os.Lstat(filepath.Join(dest, filepath.FromSlash(rel))); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s was installed (err %v)", rel, err)
		}
	}
	for _, rel := range []string{"SKILL.md", "references/api.md"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s not installed: %v", rel, err)
		}
	}
	if got := stateOf(t, v, agent, "mocked-skill-00006"); got != Synced {
		t.Errorf("status = %s, want synced: the copy must hash like the vault skill", got)
	}
}

func TestInstallLock(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before, err := target.ReadLock(agent.Path)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now().UTC().Add(-time.Second)
	if _, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00002", "mocked-skill-00004"}, Options{}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	after, err := target.ReadLock(agent.Path)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"mocked-skill-00002", "mocked-skill-00004"} {
		s := vaultSkill(t, v, name)
		entry := after.Skills[name]
		if entry.Hash != s.Hash || entry.Version != s.Version {
			t.Errorf("%s entry = %+v, want hash %s version %q", name, entry, s.Hash, s.Version)
		}
		if entry.InstalledAt.Before(start) {
			t.Errorf("%s installed_at = %v, want after %v", name, entry.InstalledAt, start)
		}
	}
	if got := after.Skills["mocked-skill-00002"].Version; got != "1.2.0" {
		t.Errorf("reinstalled version = %q, want 1.2.0", got)
	}
	for _, name := range []string{"mocked-skill-00001", "mocked-skill-00003", "mocked-skill-00098", "mocked-skill-00099"} {
		if !reflect.DeepEqual(after.Skills[name], before.Skills[name]) {
			t.Errorf("%s entry changed: %+v -> %+v", name, before.Skills[name], after.Skills[name])
		}
	}
}

func TestInstallCreatesMissingTargetDir(t *testing.T) {
	v := fixtureVault(t)
	agent := config.Agent{ID: "pi", Path: filepath.Join(t.TempDir(), "new", "skills")}
	if _, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00001"}, Options{}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got := stateOf(t, v, agent, "mocked-skill-00001"); got != Synced {
		t.Errorf("status = %s, want synced", got)
	}
}

func TestInstallDryRunChangesNothing(t *testing.T) {
	v := fixtureVault(t)
	claude := fixtureAgent(t, "claude")
	absent := config.Agent{ID: "pi", Path: filepath.Join(t.TempDir(), "absent")}
	before := snapshot(t, claude.Path)

	actions, err := Install(v, []config.Agent{claude, absent}, []string{"mocked-skill-00006", "mocked-skill-00001"}, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(actions) != 4 {
		t.Errorf("planned %d actions, want 4: %+v", len(actions), actions)
	}
	if after := snapshot(t, claude.Path); !reflect.DeepEqual(after, before) {
		t.Error("dry run changed the target")
	}
	if _, err := os.Stat(absent.Path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry run created %s (err %v)", absent.Path, err)
	}
}

func TestInstallCleansUpTempDirOnFailure(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before := snapshot(t, agent.Path)

	orig := copyFile
	t.Cleanup(func() { copyFile = orig })
	calls := 0
	copyFile = func(src, dst string, mode fs.FileMode) error {
		calls++
		if calls == 2 {
			return errors.New("disk full")
		}
		return orig(src, dst, mode)
	}

	_, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00006"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v, want disk full", err)
	}
	if calls != 2 {
		t.Fatalf("copyFile called %d times, want 2: the failure must come midway", calls)
	}
	if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
		t.Errorf("failed install left changes behind:\n%v", after)
	}
}

func TestPlaceSkillThenIgnoresBackupCleanupFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove read-only folders")
	}
	root := t.TempDir()
	src := filepath.Join(root, "new")
	dest := filepath.Join(root, "skills", "demo")
	backup := filepath.Join(root, "skills", ".demo.sapato-old")
	for dir, body := range map[string]string{src: "new", dest: "old"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Once the old copy is set aside, make its folder unremovable. Cleanups run
	// last in first-registered order, so the TempDir cleanup runs after this one.
	lockBackup := func() error {
		t.Cleanup(func() { _ = os.Chmod(backup, 0o755) })
		return os.Chmod(backup, 0o500)
	}

	if err := placeSkillThen(src, dest, lockBackup); err != nil {
		t.Fatalf("placeSkillThen() error = %v, want nil: the new copy is in place", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("dest holds %q, want the new copy", data)
	}
}
