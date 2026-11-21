package ops

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"sapato/internal/config"
)

// seqText returns the numbers from to to, one per line.
func seqText(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		b.WriteString(strconv.Itoa(i) + "\n")
	}
	return b.String()
}

func TestUnifiedDiff(t *testing.T) {
	ten := seqText(1, 10)
	twenty := seqText(1, 20)
	tests := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{
			name: "equal texts give no diff",
			old:  "a\nb\n",
			new:  "a\nb\n",
			want: "",
		},
		{
			name: "one change with three lines of context",
			old:  ten,
			new:  strings.Replace(ten, "\n5\n", "\nfive\n", 1),
			want: "--- A\n+++ B\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n",
		},
		{
			name: "changes with six unchanged lines between them share a hunk",
			old:  twenty,
			new:  strings.NewReplacer("\n5\n", "\nfive\n", "\n12\n", "\ntwelve\n").Replace(twenty),
			want: "--- A\n+++ B\n@@ -2,14 +2,14 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n 9\n 10\n 11\n-12\n+twelve\n 13\n 14\n 15\n",
		},
		{
			name: "changes with seven unchanged lines between them get two hunks",
			old:  twenty,
			new:  strings.NewReplacer("\n5\n", "\nfive\n", "\n13\n", "\nthirteen\n").Replace(twenty),
			want: "--- A\n+++ B\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n@@ -10,7 +10,7 @@\n 10\n 11\n 12\n-13\n+thirteen\n 14\n 15\n 16\n",
		},
		{
			name: "line added at the start",
			old:  "1\n2\n3\n4\n5\n",
			new:  "zero\n1\n2\n3\n4\n5\n",
			want: "--- A\n+++ B\n@@ -1,3 +1,4 @@\n+zero\n 1\n 2\n 3\n",
		},
		{
			name: "last line changed",
			old:  "1\n2\n3\n4\n5\n",
			new:  "1\n2\n3\n4\nx\n",
			want: "--- A\n+++ B\n@@ -2,4 +2,4 @@\n 2\n 3\n 4\n-5\n+x\n",
		},
		{
			name: "added text has no old lines",
			old:  "",
			new:  "x\ny\n",
			want: "--- A\n+++ B\n@@ -0,0 +1,2 @@\n+x\n+y\n",
		},
		{
			name: "removed text has no new lines",
			old:  "x\ny\n",
			new:  "",
			want: "--- A\n+++ B\n@@ -1,2 +0,0 @@\n-x\n-y\n",
		},
		{
			name: "missing final newline on both sides is marked",
			old:  "a\nb",
			new:  "a\nc",
			want: "--- A\n+++ B\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+c\n\\ No newline at end of file\n",
		},
		{
			name: "missing final newline on the new side only is marked",
			old:  "a\nb\n",
			new:  "a\nb",
			want: "--- A\n+++ B\n@@ -1,2 +1,2 @@\n a\n-b\n+b\n\\ No newline at end of file\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unifiedDiff("A", "B", tt.old, tt.new); got != tt.want {
				t.Errorf("unifiedDiff() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name string
		prep func(t *testing.T, vaultDir, installedDir string)
		want []FileChange
	}{
		{
			name: "identical copy has no changes",
			want: nil,
		},
		{
			name: "file only in the vault is added",
			prep: func(t *testing.T, vaultDir, _ string) {
				writeTestFile(t, filepath.Join(vaultDir, "assets", "extra.md"), "extra\n")
			},
			want: []FileChange{{
				Rel:  "assets/extra.md",
				Kind: ChangeAdded,
				Diff: "--- /dev/null\n+++ vault/assets/extra.md\n@@ -0,0 +1 @@\n+extra\n",
			}},
		},
		{
			name: "file only in the installed copy is removed",
			prep: func(t *testing.T, _, installedDir string) {
				writeTestFile(t, filepath.Join(installedDir, "assets", "notes.md"), "notes\n")
			},
			want: []FileChange{{
				Rel:  "assets/notes.md",
				Kind: ChangeRemoved,
				Diff: "--- installed/assets/notes.md\n+++ /dev/null\n@@ -1 +0,0 @@\n-notes\n",
			}},
		},
		{
			name: "text file with other content is changed",
			prep: func(t *testing.T, vaultDir, _ string) {
				p := filepath.Join(vaultDir, "SKILL.md")
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, p, strings.Replace(string(data), "# Mocked Skill 00005", "# Renamed skill", 1))
			},
			want: []FileChange{{
				Rel:  "SKILL.md",
				Kind: ChangeChanged,
				Diff: "--- installed/SKILL.md\n+++ vault/SKILL.md\n" +
					"@@ -3,7 +3,7 @@\n" +
					" description: Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.\n" +
					" ---\n" +
					" \n" +
					"-# Mocked Skill 00005\n" +
					"+# Renamed skill\n" +
					" \n" +
					" Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.\n" +
					" \n",
			}},
		},
		{
			name: "binary file with other bytes is changed without text",
			prep: func(t *testing.T, vaultDir, _ string) {
				writeTestFile(t, filepath.Join(vaultDir, "assets", "logo.png"), "\x89PNG\r\n\x1a\n\x00changed")
			},
			want: []FileChange{{Rel: "assets/logo.png", Kind: ChangeChanged, Binary: true}},
		},
		{
			name: "mode bits alone are a change",
			prep: func(t *testing.T, _, installedDir string) {
				if err := os.Chmod(filepath.Join(installedDir, "assets", "template.txt"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: []FileChange{{Rel: "assets/template.txt", Kind: ChangeChanged, Mode: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			if _, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00005"}, Options{}); err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			if tt.prep != nil {
				tt.prep(t, vaultSkill(t, v, "mocked-skill-00005").Dir, filepath.Join(agent.Path, "mocked-skill-00005"))
			}

			got, err := Diff(v, agent, "mocked-skill-00005")
			if err != nil {
				t.Fatalf("Diff() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Diff() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// TestDiffComparesExecBitOnly checks that diff counts a mode change only when
// the exec bit changes, as the content hash does, so diff and status agree.
func TestDiffComparesExecBitOnly(t *testing.T) {
	tests := []struct {
		name       string
		installed  fs.FileMode
		vault      fs.FileMode
		wantMode   bool
		wantSynced bool // status after the change, when the hash does not move
	}{
		{name: "group write alone is no change", installed: 0o644, vault: 0o664, wantSynced: true},
		{name: "owner-only file is no change", installed: 0o644, vault: 0o600, wantSynced: true},
		{name: "exec bit on the vault side is a change", installed: 0o644, vault: 0o755, wantMode: true},
		{name: "exec bit on the installed side is a change", installed: 0o744, vault: 0o644, wantMode: true},
		{name: "exec bit on both sides is no change", installed: 0o755, vault: 0o744},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			skill := "mocked-skill-00005"
			if _, err := Install(v, []config.Agent{agent}, []string{skill}, Options{}); err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			file := filepath.Join("assets", "template.txt")
			if err := os.Chmod(filepath.Join(agent.Path, skill, file), tt.installed); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(vaultSkill(t, v, skill).Dir, file), tt.vault); err != nil {
				t.Fatal(err)
			}

			changes, err := Diff(v, agent, skill)
			if err != nil {
				t.Fatalf("Diff() error = %v", err)
			}
			switch {
			case tt.wantMode && (len(changes) != 1 || !changes[0].Mode):
				t.Errorf("Diff() = %+v, want one mode change", changes)
			case !tt.wantMode && len(changes) != 0:
				t.Errorf("Diff() = %+v, want no change", changes)
			}
			if tt.wantSynced {
				if got := stateOf(t, v, agent, skill); got != Synced {
					t.Errorf("status = %s, want synced, as diff reports no change", got)
				}
			}
		})
	}
}

func TestDiffErrors(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		wantErr string
	}{
		{name: "skill not in vault", skill: "no-such-skill", wantErr: `diff: no skill "no-such-skill" in vault`},
		{name: "not installed in the agent", skill: "mocked-skill-00004", wantErr: "diff mocked-skill-00004 in claude: not installed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			_, err := Diff(v, agent, tt.skill)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
