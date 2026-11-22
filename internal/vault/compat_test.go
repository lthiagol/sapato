package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenCompat(t *testing.T) {
	tests := []struct {
		name         string
		manifest     string
		wantErr      string
		wantCompat   map[string][]string
		wantCompatOf string // skill checked with CompatAgents
	}{
		{
			name:         "compatible list is read",
			manifest:     "format = 1\n\n[skills.code-review]\ncompatible = [\"claude\", \"cursor\"]\n",
			wantCompat:   map[string][]string{"code-review": {"claude", "cursor"}},
			wantCompatOf: "code-review",
		},
		{
			name:     "empty list is rejected",
			manifest: "format = 1\n\n[skills.code-review]\ncompatible = []\n",
			wantErr:  "compatible list is empty",
		},
		{
			name:     "bad agent name is rejected",
			manifest: "format = 1\n\n[skills.code-review]\ncompatible = [\"Claude\"]\n",
			wantErr:  "agent name \"Claude\" is invalid",
		},
		{
			name:     "unknown key is rejected",
			manifest: "format = 1\n\n[skills.code-review]\ncompatible = [\"claude\"]\nnote = \"hi\"\n",
			wantErr:  "unknown key \"note\"",
		},
		{
			name:     "sub-table is an unknown key",
			manifest: "format = 1\n\n[skills.code-review.extra]\nx = 1\n",
			wantErr:  "unknown key \"extra\"",
		},
		{
			name:     "compatible as a string is rejected",
			manifest: "format = 1\n\n[skills.code-review]\ncompatible = \"claude\"\n",
			wantErr:  "compatible must be a list of agent names",
		},
		{
			name:     "compatible with a non-string is rejected",
			manifest: "format = 1\n\n[skills.code-review]\ncompatible = [1]\n",
			wantErr:  "compatible must be a list of agent names",
		},
		{
			name:       "table without compatible key means every agent",
			manifest:   "format = 1\n\n[skills.code-review]\n",
			wantCompat: map[string][]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newVault(t, tt.manifest)
			v, err := Open(dir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Open() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if !reflect.DeepEqual(v.compat, tt.wantCompat) {
				t.Errorf("compat = %v, want %v", v.compat, tt.wantCompat)
			}
			if tt.wantCompatOf != "" {
				if got := v.CompatAgents(tt.wantCompatOf); !reflect.DeepEqual(got, tt.wantCompat[tt.wantCompatOf]) {
					t.Errorf("CompatAgents() = %v", got)
				}
			}
		})
	}
}

func TestCompatible(t *testing.T) {
	dir := newVault(t, "format = 1\n\n[skills.code-review]\ncompatible = [\"claude\"]\n")
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		skill, agent string
		want         bool
	}{
		{"code-review", "claude", true},
		{"code-review", "cursor", false},
		{"no-table", "cursor", true},
		{"no-table", "claude", true},
	}
	for _, tt := range tests {
		if got := v.Compatible(tt.skill, tt.agent); got != tt.want {
			t.Errorf("Compatible(%s, %s) = %v, want %v", tt.skill, tt.agent, got, tt.want)
		}
	}
	if got := v.CompatAgents("no-table"); got != nil {
		t.Errorf("CompatAgents(no-table) = %v, want nil", got)
	}
}

func TestEditCompat(t *testing.T) {
	const head = "# my vault\nformat = 1\n\n[assignments]\nclaude = [\"code-review\"]   # keep\n\n"
	tests := []struct {
		name    string
		input   string
		changes map[string][]string
		want    string
	}{
		{
			name:    "new table is appended and every other byte is kept",
			input:   head + "[skills.other]\ncompatible = [\"cursor\"]\n",
			changes: map[string][]string{"code-review": {"claude"}},
			want:    head + "[skills.other]\ncompatible = [\"cursor\"]\n\n[skills.code-review]\ncompatible = [\"claude\"]\n",
		},
		{
			name:    "replaced table moves to the end, the others stay",
			input:   "format = 1\n\n[skills.a]\ncompatible = [\"claude\"]\n\n[skills.b]\ncompatible = [\"cursor\"]\n",
			changes: map[string][]string{"a": {"agents"}},
			want:    "format = 1\n\n\n[skills.b]\ncompatible = [\"cursor\"]\n\n[skills.a]\ncompatible = [\"agents\"]\n",
		},
		{
			name:    "removed table is cut out, the one after it stays",
			input:   "format = 1\n\n[skills.a]\ncompatible = [\"claude\"]\n\n[skills.b]\ncompatible = [\"cursor\"]\n",
			changes: map[string][]string{"a": nil},
			want:    "format = 1\n\n\n[skills.b]\ncompatible = [\"cursor\"]\n",
		},
		{
			name:    "blank and comment lines after the removed table are kept",
			input:   "format = 1\n\n[skills.a]\ncompatible = [\"claude\"]\n\n# assignments for my laptop\n[assignments]\nclaude = [\"a\"]\n",
			changes: map[string][]string{"a": nil},
			want:    "format = 1\n\n\n# assignments for my laptop\n[assignments]\nclaude = [\"a\"]\n",
		},
		{
			name:    "same-line comment of the removed table goes, trailing comments at end of file stay",
			input:   "format = 1\n\n[skills.a]\ncompatible = [\"claude\"] # who\n# trailing note\n",
			changes: map[string][]string{"a": nil},
			want:    "format = 1\n\n# trailing note\n",
		},
		{
			name:    "removing the last table at end of file",
			input:   "format = 1\n[skills.a]\ncompatible = [\"claude\"]\n",
			changes: map[string][]string{"a": nil},
			want:    "format = 1\n",
		},
		{
			name:    "header with spaces, quotes and a comment is found",
			input:   "format = 1\n\n[ skills.\"a\" ]  # note\ncompatible = [\"claude\"]\n",
			changes: map[string][]string{"a": nil},
			want:    "format = 1\n\n",
		},
		{
			name:    "missing trailing newline gets one before the new table",
			input:   "format = 1\n[assignments]\nclaude = [\"x\"]",
			changes: map[string][]string{"a": {"claude"}},
			want:    "format = 1\n[assignments]\nclaude = [\"x\"]\n\n[skills.a]\ncompatible = [\"claude\"]\n",
		},
		{
			name:    "name that is not a bare key is quoted",
			input:   "format = 1\n",
			changes: map[string][]string{"Odd.Name": {"claude"}},
			want:    "format = 1\n\n[skills.\"Odd.Name\"]\ncompatible = [\"claude\"]\n",
		},
		{
			name:    "removing an absent table changes nothing",
			input:   head,
			changes: map[string][]string{"missing": nil},
			want:    head,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newVault(t, tt.input)
			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := v.EditCompat(tt.changes); err != nil {
				t.Fatalf("EditCompat() error = %v", err)
			}
			got, err := os.ReadFile(filepath.Join(dir, "sapato.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("sapato.toml =\n%q\nwant\n%q", got, tt.want)
			}
			reopened, err := Open(dir)
			if err != nil {
				t.Fatalf("reopen after edit: %v", err)
			}
			if !reflect.DeepEqual(reopened.compat, v.compat) {
				t.Errorf("in-memory compat %v differs from file %v", v.compat, reopened.compat)
			}
		})
	}
}

func TestEditCompatRejectsBadChangeAndKeepsFile(t *testing.T) {
	const input = "format = 1\n\n[skills.a]\ncompatible = [\"claude\"]\n"
	tests := []struct {
		name    string
		changes map[string][]string
		wantErr string
	}{
		{
			name:    "bad agent name",
			changes: map[string][]string{"b": {"Bad Name"}},
			wantErr: "agent name \"Bad Name\" is invalid",
		},
		{
			name:    "empty list",
			changes: map[string][]string{"b": {}},
			wantErr: "compatible list is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newVault(t, input)
			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			err = v.EditCompat(tt.changes)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("EditCompat() error = %v, want containing %q", err, tt.wantErr)
			}
			got, err := os.ReadFile(filepath.Join(dir, "sapato.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != input {
				t.Errorf("sapato.toml changed to %q", got)
			}
		})
	}
}

func TestEditCompatKeepsFileMode(t *testing.T) {
	dir := newVault(t, "format = 1\n")
	path := filepath.Join(dir, "sapato.toml")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.EditCompat(map[string][]string{"a": {"claude"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestEditCompatDottedFormIsRefused(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"dotted key", "format = 1\n\n[skills]\nx.compatible = [\"claude\"]\n"},
		{"inline table", "format = 1\n\n[skills]\nx = { compatible = [\"claude\"] }\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newVault(t, tt.input)
			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			err = v.EditCompat(map[string][]string{"x": nil})
			if err == nil || !strings.Contains(err.Error(), "result does not match the change") {
				t.Fatalf("EditCompat() error = %v, want result mismatch", err)
			}
			got, err := os.ReadFile(filepath.Join(dir, "sapato.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.input {
				t.Errorf("sapato.toml changed to %q", got)
			}
		})
	}
}

func TestPlanCompat(t *testing.T) {
	const input = "format = 1\n\n[skills.a]\ncompatible = [\"claude\"]\n"
	tests := []struct {
		name      string
		input     string
		changes   map[string][]string
		wantErr   string
		wantAfter string // skill a's compatible list after Write, when no error
	}{
		{
			name:      "valid change is checked and applied by Write",
			input:     input,
			changes:   map[string][]string{"a": {"cursor"}},
			wantAfter: "cursor",
		},
		{
			name:    "dotted form is refused before anything is written",
			input:   "format = 1\n\n[skills]\na.compatible = [\"claude\"]\n",
			changes: map[string][]string{"a": nil},
			wantErr: "result does not match the change",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newVault(t, tt.input)
			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			edit, err := v.PlanCompat(tt.changes)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("PlanCompat() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PlanCompat() error = %v", err)
			}
			got, err := os.ReadFile(filepath.Join(dir, "sapato.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.input {
				t.Fatalf("PlanCompat() wrote the file")
			}
			if err := edit.Write(); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if got := v.CompatAgents("a"); !reflect.DeepEqual(got, []string{tt.wantAfter}) {
				t.Errorf("CompatAgents(a) after Write = %v, want [%s]", got, tt.wantAfter)
			}
		})
	}
}

func TestSkillsCarryCompatList(t *testing.T) {
	dir := newVault(t, "format = 1\n\n[skills.code-review]\ncompatible = [\"claude\"]\n")
	for _, name := range []string{"code-review", "other"} {
		skill := filepath.Join(dir, "skills", name)
		if err := os.MkdirAll(skill, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(skill, "SKILL.md"), "---\nname: "+name+"\ndescription: d\n---\n")
	}
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	skills, err := v.Skills()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, s := range skills {
		got[s.Name] = s.Compat
	}
	want := map[string][]string{"code-review": {"claude"}, "other": nil}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Skills() compat = %v, want %v", got, want)
	}
}
