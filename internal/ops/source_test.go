package ops

import (
	"archive/tar"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const freshSkillMD = "---\nname: fresh-skill\ndescription: A fresh skill.\n---\n\n# Fresh\n"

// writeFiles writes each file of files under root, creating folders as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
}

// discoverRels returns the Rel of each candidate that Discover finds in src.
func discoverRels(t *testing.T, src, subdir string) ([]string, error) {
	t.Helper()
	cands, cleanup, err := Discover(src, subdir)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	rels := []string{}
	for _, c := range cands {
		rels = append(rels, c.Rel)
	}
	return rels, nil
}

func TestDiscover(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		subdir   string
		wantRels []string
		wantErr  string
	}{
		{
			name:     "root skill",
			files:    map[string]string{"SKILL.md": freshSkillMD},
			wantRels: []string{"."},
		},
		{
			name:     "one nested skill",
			files:    map[string]string{"skills/fresh-skill/SKILL.md": freshSkillMD, "README.md": "readme"},
			wantRels: []string{"skills/fresh-skill"},
		},
		{
			name:     "several skills are listed sorted",
			files:    map[string]string{"b/SKILL.md": freshSkillMD, "a/c/SKILL.md": freshSkillMD},
			wantRels: []string{"a/c", "b"},
		},
		{
			name:     "a root skill is the only candidate",
			files:    map[string]string{"SKILL.md": freshSkillMD, "nested/SKILL.md": freshSkillMD},
			wantRels: []string{"."},
		},
		{
			name:     "search stops at a found skill",
			files:    map[string]string{"outer/SKILL.md": freshSkillMD, "outer/inner/SKILL.md": freshSkillMD},
			wantRels: []string{"outer"},
		},
		{
			name:     "depth four is searched",
			files:    map[string]string{"l1/l2/l3/l4/SKILL.md": freshSkillMD},
			wantRels: []string{"l1/l2/l3/l4"},
		},
		{
			name:     "deeper than four is not searched",
			files:    map[string]string{"l1/l2/l3/l4/l5/SKILL.md": freshSkillMD},
			wantRels: []string{},
		},
		{
			name:     "dot folders are skipped",
			files:    map[string]string{".hidden/SKILL.md": freshSkillMD, "visible/.inner/SKILL.md": freshSkillMD},
			wantRels: []string{},
		},
		{
			name:     "no skill is no candidate",
			files:    map[string]string{"README.md": "readme"},
			wantRels: []string{},
		},
		{
			name:     "subdir is read alone",
			files:    map[string]string{"skills/fresh-skill/SKILL.md": freshSkillMD, "other/SKILL.md": freshSkillMD},
			subdir:   "skills/fresh-skill",
			wantRels: []string{"skills/fresh-skill"},
		},
		{
			name:     "subdir without a skill is still one candidate",
			files:    map[string]string{"empty/notes.txt": "x"},
			subdir:   "empty",
			wantRels: []string{"empty"},
		},
		{
			name:    "missing subdir is refused",
			files:   map[string]string{"SKILL.md": freshSkillMD},
			subdir:  "nope",
			wantErr: "no such file",
		},
		{
			name:    "subdir that is a file is refused",
			files:   map[string]string{"notes.txt": "x"},
			subdir:  "notes.txt",
			wantErr: "is not a folder",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "pack")
			writeFiles(t, root, tt.files)
			rels, err := discoverRels(t, root, tt.subdir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Discover() error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			if !slices.Equal(rels, tt.wantRels) {
				t.Errorf("candidates = %v, want %v", rels, tt.wantRels)
			}
		})
	}
}

func TestDiscoverSkipsSymlinks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pack")
	writeFiles(t, root, map[string]string{"real/SKILL.md": freshSkillMD})
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	rels, err := discoverRels(t, root, "")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if want := []string{"real"}; !slices.Equal(rels, want) {
		t.Errorf("candidates = %v, want %v", rels, want)
	}
}

func TestDiscoverCandidateProblems(t *testing.T) {
	tooLong := "---\nname: fresh-skill\ndescription: " + strings.Repeat("x", 1025) + "\n---\n"
	tests := []struct {
		name        string
		folder      string // the folder of the source, which is the name hint
		content     string // SKILL.md, or "" for none
		subdir      string // read alone; a folder with no SKILL.md is only a candidate this way
		wantValid   bool
		wantName    string
		wantProblem string
		wantFixable bool
	}{
		{name: "valid skill", folder: "fresh-skill", content: freshSkillMD, wantValid: true, wantName: "fresh-skill"},
		{
			name:        "missing description is fixable",
			folder:      "fresh-skill",
			content:     "---\nname: fresh-skill\n---\n",
			wantName:    "fresh-skill",
			wantProblem: "frontmatter description must be a non-empty string",
			wantFixable: true,
		},
		{
			name:        "missing name is fixable",
			folder:      "fresh-skill",
			content:     "---\ndescription: A fresh skill.\n---\n",
			wantProblem: "frontmatter name must be a non-empty string",
			wantFixable: true,
		},
		{
			name:        "missing frontmatter is fixable",
			folder:      "fresh-skill",
			content:     "# Fresh\n",
			wantProblem: "missing opening --- line",
			wantFixable: true,
		},
		{
			name:      "name that is not the folder is valid",
			folder:    "fresh-skill",
			content:   "---\nname: other\ndescription: A fresh skill.\n---\n",
			wantValid: true,
			wantName:  "other",
		},
		{
			name:      "folder with spaces is valid with a valid name",
			folder:    "My Skill",
			content:   "---\nname: my-skill\ndescription: A fresh skill.\n---\n",
			wantValid: true,
			wantName:  "my-skill",
		},
		{
			name:        "invalid name is fixable",
			folder:      "fresh-skill",
			content:     "---\nname: Bad Name\ndescription: A fresh skill.\n---\n",
			wantName:    "Bad Name",
			wantProblem: "must match",
			wantFixable: true,
		},
		{
			name:        "too long description is not fixable",
			folder:      "fresh-skill",
			content:     tooLong,
			wantName:    "fresh-skill",
			wantProblem: "description is 1025 characters",
		},
		{
			name:        "unclosed frontmatter is not fixable",
			folder:      "fresh-skill",
			content:     "---\nname: fresh-skill\n",
			wantProblem: "missing closing --- line",
		},
		{
			name:        "missing SKILL.md is not fixable",
			folder:      "fresh-skill",
			subdir:      ".",
			wantProblem: "SKILL.md is missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), tt.folder)
			if tt.content != "" {
				writeTestFile(t, filepath.Join(root, "SKILL.md"), tt.content)
			} else {
				writeTestFile(t, filepath.Join(root, "notes.md"), "x")
			}
			cands, cleanup, err := Discover(root, tt.subdir)
			defer cleanup()
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			if len(cands) != 1 {
				t.Fatalf("candidates = %d, want 1", len(cands))
			}
			c := cands[0]
			if c.Valid != tt.wantValid || c.Name != tt.wantName || c.Fixable != tt.wantFixable {
				t.Errorf("valid=%v name=%q fixable=%v, want valid=%v name=%q fixable=%v (problems %v)",
					c.Valid, c.Name, c.Fixable, tt.wantValid, tt.wantName, tt.wantFixable, c.Problems)
			}
			if c.Hint != tt.folder {
				t.Errorf("Hint = %q, want %q", c.Hint, tt.folder)
			}
			if tt.wantProblem != "" && !slices.ContainsFunc(c.Problems, func(p string) bool { return strings.Contains(p, tt.wantProblem) }) {
				t.Errorf("Problems = %v, want one containing %q", c.Problems, tt.wantProblem)
			}
		})
	}
}

func TestDiscoverLocalFile(t *testing.T) {
	tests := []struct {
		name     string
		folder   string
		file     string
		content  string
		wantName string
		wantOK   bool
	}{
		{name: "SKILL.md in a folder named after it", folder: "fresh-skill", file: "SKILL.md", content: freshSkillMD, wantName: "fresh-skill", wantOK: true},
		{name: "any markdown file is read as SKILL.md", folder: "fresh-skill", file: "notes.md", content: freshSkillMD, wantName: "fresh-skill", wantOK: true},
		{name: "upper-case .MD is read", folder: "fresh-skill", file: "NOTES.MD", content: freshSkillMD, wantName: "fresh-skill", wantOK: true},
		{name: "file in a folder with another name is valid", folder: "other-dir", file: "SKILL.md", content: freshSkillMD, wantName: "fresh-skill", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.folder, tt.file)
			writeTestFile(t, path, tt.content)
			cands, cleanup, err := Discover(path, "")
			defer cleanup()
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			if len(cands) != 1 || cands[0].Rel != "." || cands[0].Hint != tt.folder {
				t.Fatalf("candidates = %+v, want one root candidate with hint %q", cands, tt.folder)
			}
			if cands[0].Valid != tt.wantOK || cands[0].Name != tt.wantName {
				t.Errorf("valid=%v name=%q, want valid=%v name=%q", cands[0].Valid, cands[0].Name, tt.wantOK, tt.wantName)
			}
		})
	}
}

func TestDiscoverLocalFileRefusesSubdir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	writeTestFile(t, path, freshSkillMD)
	_, cleanup, err := Discover(path, "skills")
	defer cleanup()
	if err == nil || !strings.Contains(err.Error(), "does not apply to a single file") {
		t.Fatalf("Discover() error = %v, want the single-file subdir refusal", err)
	}
}

func TestDiscoverTarball(t *testing.T) {
	tests := []struct {
		name      string
		entries   []tarEntry
		wantRel   string
		wantValid bool
	}{
		{name: "skill in a folder with its name", entries: skillEntries("tar-skill"), wantRel: "tar-skill", wantValid: true},
		{
			name: "skill in a folder with another name is invalid",
			entries: []tarEntry{
				{name: "dir/", typeflag: tar.TypeDir},
				{name: "dir/SKILL.md", body: []byte(freshSkillMD)},
			},
			wantRel: "dir",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			cands, cleanup, err := Discover(makeTarball(t, tt.entries...), "")
			defer cleanup()
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			if len(cands) != 1 || cands[0].Rel != tt.wantRel || cands[0].Valid != tt.wantValid || cands[0].Fixable {
				t.Errorf("candidates = %+v, want one tarball skill at %q, valid=%v, not fixable", cands, tt.wantRel, tt.wantValid)
			}
		})
	}
}

func TestDiscoverRawURL(t *testing.T) {
	tests := []struct {
		name      string
		path      string // the path requested from the server
		status    int
		body      []byte
		src       string // the URL to discover; "" is the server URL plus path
		wantErr   string
		wantHint  string
		wantValid bool
	}{
		{
			name:      "raw SKILL.md is one skill named after its folder",
			path:      "/skills/fresh-skill/SKILL.md",
			status:    http.StatusOK,
			body:      []byte(freshSkillMD),
			wantHint:  "fresh-skill",
			wantValid: true,
		},
		{
			name:    "non-200 status is refused",
			path:    "/skills/fresh-skill/SKILL.md",
			status:  http.StatusNotFound,
			wantErr: "status 404",
		},
		{
			name:    "body over 1 MiB is refused",
			path:    "/skills/fresh-skill/SKILL.md",
			status:  http.StatusOK,
			body:    bytes.Repeat([]byte("x"), rawMaxBytes+1),
			wantErr: "larger than",
		},
		{
			name:      "GitHub blob URL is fetched from the raw file",
			path:      "/o/r/main/skills/fresh-skill/SKILL.md",
			status:    http.StatusOK,
			body:      []byte(freshSkillMD),
			src:       "https://github.com/o/r/blob/main/skills/fresh-skill/SKILL.md",
			wantHint:  "fresh-skill",
			wantValid: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serveRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))
			src := tt.src
			if src == "" {
				src = srv.URL + tt.path
			}
			cands, cleanup, err := Discover(src, "")
			defer cleanup()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Discover() error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			if len(cands) != 1 || cands[0].Hint != tt.wantHint || cands[0].Valid != tt.wantValid {
				t.Errorf("candidates = %+v, want one valid candidate with hint %q", cands, tt.wantHint)
			}
		})
	}
}

// serveRaw points the raw SKILL.md fetch at an httptest server running h.
func serveRaw(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	oldClient, oldBase := httpClient, rawGitHubBase
	httpClient, rawGitHubBase = srv.Client(), srv.URL
	t.Cleanup(func() { httpClient, rawGitHubBase = oldClient, oldBase })
	return srv
}

func TestRawHint(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"https://github.com/o/code-review/blob/main/SKILL.md", "code-review"},
		{"https://raw.githubusercontent.com/o/code-review/main/SKILL.md", "code-review"},
		{"https://github.com/o/r/blob/main/skills/fresh-skill/SKILL.md", "fresh-skill"},
		{"https://raw.githubusercontent.com/o/r/main/skills/fresh-skill/SKILL.md", "fresh-skill"},
		{"https://example.com/skills/fresh-skill/SKILL.md", "fresh-skill"},
		{"https://example.com/SKILL.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := rawHint(tt.src); got != tt.want {
				t.Errorf("rawHint(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestGithubRaw(t *testing.T) {
	const base = "https://raw.githubusercontent.com"
	tests := []struct {
		src  string
		want string
	}{
		{"https://github.com/o/r/blob/main/skills/x/SKILL.md", base + "/o/r/main/skills/x/SKILL.md"},
		{"https://github.com/o/r/blob/v1.2/SKILL.md", base + "/o/r/v1.2/SKILL.md"},
		{"https://github.com/o/r/tree/main/skills/x/SKILL.md", "https://github.com/o/r/tree/main/skills/x/SKILL.md"},
		{"https://example.com/o/r/blob/main/SKILL.md", "https://example.com/o/r/blob/main/SKILL.md"},
		{"https://example.com/skills/x/SKILL.md", "https://example.com/skills/x/SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := githubRaw(tt.src, base); got != tt.want {
				t.Errorf("githubRaw() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsRawURL(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"https://example.com/skills/x/SKILL.md", true},
		{"https://example.com/skills/x/Notes.MD", true},
		{"https://example.com/skills/x/SKILL.md?plain=1", true},
		{"https://example.com/skill.git", false},
		{"http://example.com/SKILL.md", false},
		{"/home/u/SKILL.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := isRawURL(tt.src); got != tt.want {
				t.Errorf("isRawURL(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestRepoName(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"https://github.com/o/code-review.git", "code-review"},
		{"https://github.com/o/code-review", "code-review"},
		{"https://github.com/o/code-review/", "code-review"},
		{"git@github.com:o/code-review.git", "code-review"},
		{"/home/u/skills.git", "skills"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := repoName(tt.src); got != tt.want {
				t.Errorf("repoName(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		hint string
		want string
	}{
		{"fresh-skill", "fresh-skill"},
		{"My Skill", "my-skill"},
		{"  --Code_Review!!  ", "code-review"},
		{"héllo wörld", "h-llo-w-rld"},
		{"!!!", ""},
		{"", ""},
		{strings.Repeat("a", 70), strings.Repeat("a", 64)},
		{strings.Repeat("a", 63) + " b", strings.Repeat("a", 63)},
	}
	for _, tt := range tests {
		t.Run(tt.hint, func(t *testing.T) {
			if got := NormalizeName(tt.hint); got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.hint, got, tt.want)
			}
		})
	}
}
