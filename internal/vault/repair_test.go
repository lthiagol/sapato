package vault

import (
	"strings"
	"testing"
)

func TestRepair(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		skillName   string
		description string
		want        string
	}{
		{
			name:        "no frontmatter gets a new one and keeps the body",
			in:          "# Fresh\n",
			skillName:   "fresh",
			description: "A fresh skill.",
			want:        "---\nname: fresh\ndescription: A fresh skill.\n---\n# Fresh\n",
		},
		{
			name:        "empty frontmatter is filled",
			in:          "---\n---\nbody\n",
			skillName:   "fresh",
			description: "A fresh skill.",
			want:        "---\nname: fresh\ndescription: A fresh skill.\n---\nbody\n",
		},
		{
			name:        "missing keys are appended and other keys keep their order",
			in:          "---\nlicense: MIT\nmetadata:\n    version: 1.0\n---\n\n# Body\n",
			skillName:   "fresh",
			description: "A fresh skill.",
			want:        "---\nlicense: MIT\nmetadata:\n  version: 1.0\nname: fresh\ndescription: A fresh skill.\n---\n\n# Body\n",
		},
		{
			name:        "wrong name is replaced where it stands",
			in:          "---\nname: Wrong Name\ndescription: Kept.\n---\n",
			skillName:   "fresh",
			description: "Not used.",
			want:        "---\nname: fresh\ndescription: Kept.\n---\n",
		},
		{
			name:        "empty description takes the given one",
			in:          "---\nname: fresh\ndescription: \"\"\n---\n",
			skillName:   "fresh",
			description: "Filled.",
			want:        "---\nname: fresh\ndescription: Filled.\n---\n",
		},
		{
			name:        "non-string description takes the given one",
			in:          "---\nname: fresh\ndescription: 5\n---\n",
			skillName:   "fresh",
			description: "Filled.",
			want:        "---\nname: fresh\ndescription: Filled.\n---\n",
		},
		{
			name:        "present description is kept over the given one",
			in:          "---\nname: fresh\ndescription: Kept.\n---\n",
			skillName:   "fresh",
			description: "Not used.",
			want:        "---\nname: fresh\ndescription: Kept.\n---\n",
		},
		{
			name:      "missing description stays missing when none is given",
			in:        "---\nname: old\n---\n",
			skillName: "fresh",
			want:      "---\nname: fresh\n---\n",
		},
		{
			name:        "name that needs quoting is quoted",
			in:          "---\n---\n",
			skillName:   "a: b",
			description: "Quoted.",
			want:        "---\nname: 'a: b'\ndescription: Quoted.\n---\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Repair([]byte(tt.in), tt.skillName, tt.description)
			if err != nil {
				t.Fatalf("Repair() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Repair() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestRepairRefuses(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{name: "unclosed frontmatter", in: "---\nname: x\n", wantErr: "missing closing --- line"},
		{name: "invalid YAML", in: "---\nname: [\n---\n", wantErr: "invalid YAML"},
		{name: "frontmatter that is a list", in: "---\n- a\n- b\n---\n", wantErr: "not a YAML mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Repair([]byte(tt.in), "fresh", "A fresh skill.")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Repair() error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseFolder(t *testing.T) {
	const body = "---\nname: fresh-skill\ndescription: A fresh skill.\n---\n"
	tests := []struct {
		name         string
		data         string
		folder       string
		wantValid    bool
		wantName     string
		wantProblems []string
	}{
		{name: "matching folder is valid", data: body, folder: "fresh-skill", wantValid: true, wantName: "fresh-skill"},
		{name: "empty folder skips the folder rule", data: body, folder: "", wantValid: true, wantName: "fresh-skill"},
		{
			name:         "folder that differs from the name is a problem",
			data:         body,
			folder:       "other",
			wantName:     "fresh-skill",
			wantProblems: []string{`name "fresh-skill" does not match folder "other"`},
		},
		{
			name:         "name rule applies without a folder",
			data:         "---\nname: Fresh Skill\ndescription: A fresh skill.\n---\n",
			folder:       "",
			wantName:     "Fresh Skill",
			wantProblems: []string{`name "Fresh Skill" must match`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ParseFolder([]byte(tt.data), tt.folder)
			if s.Valid != tt.wantValid || s.Name != tt.wantName {
				t.Errorf("ParseFolder() valid=%v name=%q, want valid=%v name=%q", s.Valid, s.Name, tt.wantValid, tt.wantName)
			}
			if len(tt.wantProblems) > 0 && (len(s.Problems) != 1 || !strings.Contains(s.Problems[0], tt.wantProblems[0])) {
				t.Errorf("Problems = %v, want one containing %q", s.Problems, tt.wantProblems[0])
			}
		})
	}
}
