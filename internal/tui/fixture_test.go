package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/target"
	"sapato/internal/testfixtures"
	"sapato/internal/vault"
)

// fixtureModel loads the UI over fresh copies of the shared fixtures. It
// returns the model and the copied vault and claude dirs.
func fixtureModel(t *testing.T) (Model, string, string) {
	t.Helper()
	vaultDir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	claudeDir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "targets", "claude"))
	v, err := vault.Open(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	th, set := testLook(t)
	m := New(opsBackend{v}, []config.Agent{{ID: "claude", Path: claudeDir, Enabled: true}}, th, set, true)
	m, _ = settle(t, m, m.Init())
	return m, vaultDir, claudeDir
}

func TestFixtureStatesShowInMatrix(t *testing.T) {
	m, _, _ := fixtureModel(t)
	for skill, want := range map[string]ops.State{
		"hand-made-skill":     ops.Unmanaged,
		"mocked-skill-00001":  ops.Synced,
		"mocked-skill-00002":  ops.Outdated,
		"mocked-skill-00003":  ops.Modified,
		"mocked-skill-00004":  ops.NotInstalled,
		"mocked-skill-00098":  ops.Missing,
		"mocked-skill-00099":  ops.Orphaned,
		"invalid-no-skill-md": ops.Invalid,
	} {
		if got := m.cells[cell{skill, "claude"}]; got != want {
			t.Errorf("%s: state = %q, want %q", skill, got, want)
		}
	}
}

func TestFixtureAdoptCopiesIntoVault(t *testing.T) {
	m, vaultDir, _ := fixtureModel(t)
	m = press(t, at(t, m, "hand-made-skill", "claude"), "a")
	if _, err := os.Stat(filepath.Join(vaultDir, "skills", "hand-made-skill", "SKILL.md")); err != nil {
		t.Errorf("vault lacks the adopted skill: %v", err)
	}
	if got := m.cells[cell{"hand-made-skill", "claude"}]; got != ops.Synced {
		t.Errorf("after adopt: state = %q, want synced; result %q", got, m.result)
	}
}

func TestFixtureInstallCopiesNotInstalledSkill(t *testing.T) {
	m, _, claudeDir := fixtureModel(t)
	m = press(t, at(t, m, "mocked-skill-00004", "claude"), "i")
	if _, err := os.Stat(filepath.Join(claudeDir, "mocked-skill-00004", "SKILL.md")); err != nil {
		t.Errorf("install left no SKILL.md: %v", err)
	}
	if got := m.cells[cell{"mocked-skill-00004", "claude"}]; got != ops.Synced {
		t.Errorf("after install: state = %q, want synced; result %q", got, m.result)
	}
}

func TestFixtureUpdateOfModifiedCopy(t *testing.T) {
	m, _, claudeDir := fixtureModel(t)
	skillFile := filepath.Join(claudeDir, "mocked-skill-00003", "SKILL.md")
	before, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}

	declined := press(t, at(t, m, "mocked-skill-00003", "claude"), "U", "n")
	after, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || declined.mode != modeMatrix {
		t.Errorf("declined update changed the copy or left the prompt open (mode %v)", declined.mode)
	}

	accepted := press(t, at(t, m, "mocked-skill-00003", "claude"), "U", "y")
	if got := accepted.cells[cell{"mocked-skill-00003", "claude"}]; got != ops.Synced {
		t.Errorf("after forced update: state = %q, want synced; result %q", got, accepted.result)
	}
}

func TestFixtureUninstallOfOrphanedCopy(t *testing.T) {
	m, _, claudeDir := fixtureModel(t)
	waiting := press(t, at(t, m, "mocked-skill-00099", "claude"), "u")
	if waiting.mode != modeConfirm {
		t.Fatalf("mode = %v, want confirm", waiting.mode)
	}
	m = press(t, waiting, "y")
	if _, err := os.Stat(filepath.Join(claudeDir, "mocked-skill-00099")); !os.IsNotExist(err) {
		t.Errorf("orphaned copy still there: %v", err)
	}
	if slices.Contains(m.skills, "mocked-skill-00099") {
		t.Errorf("row for the removed skill is still listed")
	}
}

func TestFixtureUninstallOfModifiedCopyNeedsForce(t *testing.T) {
	m, _, claudeDir := fixtureModel(t)
	folder := filepath.Join(claudeDir, "mocked-skill-00003")
	intact := func(when string) {
		t.Helper()
		if _, err := os.Stat(folder); err != nil {
			t.Errorf("%s: folder gone: %v", when, err)
		}
		if lock, err := target.ReadLock(claudeDir); err != nil || lock.Skills["mocked-skill-00003"].Hash == "" {
			t.Errorf("%s: lock entry gone (err %v)", when, err)
		}
	}

	asked := press(t, at(t, m, "mocked-skill-00003", "claude"), "u")
	if asked.mode != modeConfirm || !asked.pending.act.force {
		t.Fatalf("mode = %v, force = %t; want the force prompt", asked.mode, asked.pending.act.force)
	}
	intact("after u")

	declined := press(t, asked, "n")
	if declined.mode != modeMatrix {
		t.Errorf("mode after n = %v, want matrix", declined.mode)
	}
	intact("after n")

	accepted := press(t, asked, "y")
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Errorf("modified copy still there after y on the force prompt: %v", err)
	}
	if lock, err := target.ReadLock(claudeDir); err != nil || lock.Skills["mocked-skill-00003"].Hash != "" {
		t.Errorf("lock entry still there after y (err %v)", err)
	}
	if accepted.result != "uninstall mocked-skill-00003 in claude" {
		t.Errorf("result = %q", accepted.result)
	}
}

func TestFixtureOpsRefusalIsShown(t *testing.T) {
	m, _, _ := fixtureModel(t)
	m = press(t, at(t, m, "mocked-skill-00098", "claude"), "i")
	if !m.resultErr || m.result == "" {
		t.Errorf("result = %q (err %t), want the ops error", m.result, m.resultErr)
	}
}
