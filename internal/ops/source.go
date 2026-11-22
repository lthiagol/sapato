package ops

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"sapato/internal/vault"
)

// Limits and bounds of an import source (spec §7.1).
const (
	skillFileName = "SKILL.md"
	maxSkillDepth = 4
	rawMaxBytes   = 1 << 20
	rawTimeout    = 15 * time.Second
)

var (
	// httpClient fetches raw SKILL.md URLs. Tests replace it to reach an httptest server.
	httpClient = &http.Client{Timeout: rawTimeout}
	// rawGitHubBase is the host that GitHub blob URLs are rewritten to. Tests replace it.
	rawGitHubBase = "https://raw.githubusercontent.com"
)

// Candidate is a skill found in an import source (spec §7.1).
type Candidate struct {
	// Rel is the folder of the skill relative to the source root, slash-separated.
	// It is "." for the root.
	Rel string
	// Hint is the folder name the skill is expected to carry, and the name hint for
	// --fix. For the root it is the repository name, the URL's folder name or the
	// folder of the file. Empty when there is none.
	Hint string
	// Name is the name in the frontmatter, "" when there is none.
	Name     string
	Valid    bool
	Problems []string
	// Fixable reports whether --fix would repair the problems.
	Fixable bool

	// dir is the folder that holds the skill, in the fetched copy of the source.
	dir string
}

// Discover lists the skills in the import source src without changing the vault
// (spec §7.1). subdir, when set, names the one folder to read. A remote source or a
// tarball is fetched into a temp dir, which cleanup removes. Call cleanup even when
// err is set. The candidates are sorted by Rel. Discover does not decide which
// candidates to import.
func Discover(src, subdir string) (cands []Candidate, cleanup func(), err error) {
	if isArchive(src) {
		return discoverArchive(src, subdir)
	}
	cleanup = func() {}
	var root, rootHint string
	single := false
	switch {
	case isRawURL(src):
		dir, cl, err := fetchRaw(src)
		if err != nil {
			return nil, cleanup, err
		}
		root, cleanup, rootHint, single = dir, cl, rawHint(src), true
	case isGitURL(src):
		dir, cl, err := cloneRepo(src)
		if err != nil {
			return nil, cleanup, err
		}
		root, cleanup, rootHint = dir, cl, repoName(src)
	default:
		resolved, err := filepath.EvalSymlinks(src)
		if err != nil {
			return nil, cleanup, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, cleanup, err
		}
		switch {
		case info.IsDir():
			root, rootHint = resolved, folderHint(filepath.Base(resolved))
		case info.Mode().IsRegular():
			if !strings.EqualFold(filepath.Ext(src), ".md") {
				return nil, cleanup, fmt.Errorf("%s is not a .md file or a tarball", src)
			}
			dir, cl, err := stageFile(resolved)
			if err != nil {
				return nil, cleanup, err
			}
			root, cleanup, rootHint, single = dir, cl, folderHint(filepath.Base(filepath.Dir(resolved))), true
		default:
			return nil, cleanup, fmt.Errorf("%s is neither a file nor a folder", src)
		}
	}
	if single && subdir != "" {
		return nil, cleanup, errors.New("subdir does not apply to a single file")
	}
	cands, err = candidates(root, rootHint, subdir)
	if err != nil {
		return nil, cleanup, err
	}
	return cands, cleanup, nil
}

// discoverArchive lists the top-level skill folders of the tarball src. It does not
// check the manifest, and it never offers a fix for an archive.
func discoverArchive(src, subdir string) ([]Candidate, func(), error) {
	cleanup := func() {}
	if subdir != "" {
		return nil, cleanup, errors.New("subdir does not apply to a tarball")
	}
	tmp, err := os.MkdirTemp("", "sapato-import-")
	if err != nil {
		return nil, cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	if _, err := extractArchive(src, tmp); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	var cands []Candidate
	for _, e := range entries {
		c, err := inspectCandidate(tmp, filepath.Join(tmp, e.Name()), "", true)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		c.Fixable = false
		cands = append(cands, c)
	}
	return cands, cleanup, nil
}

// candidates lists the skills in root: the folder subdir alone when set, else root
// when it holds a SKILL.md, else every skill folder below it.
func candidates(root, rootHint, subdir string) ([]Candidate, error) {
	if subdir != "" {
		dir, err := skillDir(root, subdir)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("subdir %q is not a folder", subdir)
		}
		c, err := inspectCandidate(root, dir, rootHint, false)
		return []Candidate{c}, err
	}
	dirs := []string{root}
	if !holdsSkill(root) {
		var err error
		if dirs, err = findSkillDirs(root); err != nil {
			return nil, err
		}
	}
	cands := make([]Candidate, 0, len(dirs))
	for _, dir := range dirs {
		c, err := inspectCandidate(root, dir, rootHint, false)
		if err != nil {
			return nil, err
		}
		cands = append(cands, c)
	}
	slices.SortFunc(cands, func(a, b Candidate) int { return strings.Compare(a.Rel, b.Rel) })
	return cands, nil
}

// findSkillDirs returns the folders below root that hold a SKILL.md, without
// descending into them. It skips dot-folders and symlinks, and looks no deeper than
// maxSkillDepth.
func findSkillDirs(root string) ([]string, error) {
	var found []string
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if depth == maxSkillDepth {
			return nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			sub := filepath.Join(dir, e.Name())
			if holdsSkill(sub) {
				found = append(found, sub)
				continue
			}
			if err := walk(sub, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	err := walk(root, 0)
	return found, err
}

// holdsSkill reports whether dir has a SKILL.md entry of any kind.
func holdsSkill(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, skillFileName))
	return err == nil
}

// inspectCandidate reads the skill in dir, which is root or a folder inside it.
// rootHint is the name hint of root and applies only when dir is root. A skill of a
// tarball must sit in a folder with its name (spec §3 rule 4); any other skill takes
// its name from the frontmatter and the vault folder (spec §7.1).
func inspectCandidate(root, dir, rootHint string, tarball bool) (Candidate, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return Candidate{}, err
	}
	hint := filepath.Base(dir)
	if dir == root {
		hint = rootHint
	}
	c := Candidate{Rel: filepath.ToSlash(rel), Hint: hint, dir: dir}

	file := filepath.Join(dir, skillFileName)
	info, err := os.Lstat(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		c.Problems = []string{skillFileName + " is missing"}
		return c, nil
	case err != nil:
		return Candidate{}, err
	case !info.Mode().IsRegular():
		c.Problems = []string{skillFileName + " is not a regular file"}
		return c, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return Candidate{}, err
	}
	s := vault.ParseContent(data)
	if tarball {
		s = vault.ParseFolder(data, hint)
	}
	c.Name, c.Valid, c.Problems = s.Name, s.Valid, s.Problems
	c.Fixable = !s.Valid && fixable(data)
	return c, nil
}

// fixable reports whether --fix turns the skill data into a valid skill. The name and
// the description it would take are placeholders here: a name comes from --name, the
// frontmatter or the name hint, and a description from --description.
func fixable(data []byte) bool {
	repaired, err := vault.Repair(data, "skill", "fixed")
	if err != nil {
		return false
	}
	return vault.ParseContent(repaired).Valid
}

// NormalizeName turns a name hint into a skill name (spec §7.1): lowercase, runs of
// characters outside [a-z0-9] become "-", leading and trailing "-" are trimmed, and
// the result is cut to 64 characters. It returns "" when nothing is left.
func NormalizeName(hint string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(hint) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimSuffix(b.String(), "-")
	if len(s) > 64 {
		s = strings.TrimSuffix(s[:64], "-")
	}
	return s
}

// isRawURL reports whether src is an https URL whose path ends in .md (spec §7.1).
func isRawURL(src string) bool {
	if !strings.HasPrefix(src, "https://") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(urlPath(src)), ".md")
}

// urlPath returns the path of src, or "" when src does not parse.
func urlPath(src string) string {
	u, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return u.Path
}

// folderHint returns name as a name hint, or "" for a root or a dot.
func folderHint(name string) string {
	if name == "/" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}

// githubRaw maps a GitHub blob URL to the raw file URL on base. Other URLs are
// returned unchanged.
func githubRaw(src, base string) string {
	u, err := url.Parse(src)
	if err != nil || u.Host != "github.com" {
		return src
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 5 || parts[2] != "blob" {
		return src
	}
	return base + "/" + strings.Join(append([]string{parts[0], parts[1]}, parts[3:]...), "/")
}

// rawHint returns the name hint of the raw file at src (spec §7.1): the name of its
// parent folder, or for a GitHub blob or raw URL whose file sits at the repository
// root, the repository name.
func rawHint(src string) string {
	u, err := url.Parse(src)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	switch {
	case u.Host == "github.com" && len(parts) == 5 && parts[2] == "blob":
		return parts[1]
	case u.Host == "raw.githubusercontent.com" && len(parts) == 4:
		return parts[1]
	}
	return folderHint(path.Base(path.Dir(u.Path)))
}

// fetchRaw downloads the raw SKILL.md at src into a new temp dir, which cleanup
// removes. A GitHub blob URL is fetched from its raw file.
func fetchRaw(src string) (dir string, cleanup func(), err error) {
	body, err := getRaw(githubRaw(src, rawGitHubBase))
	if err != nil {
		return "", func() {}, err
	}
	tmp, err := os.MkdirTemp("", "sapato-import-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	if err := os.WriteFile(filepath.Join(tmp, skillFileName), body, 0o644); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmp, cleanup, nil
}

// getRaw returns the body of u. The status must be 200, and the body at most rawMaxBytes.
func getRaw(u string) ([]byte, error) {
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %s", u, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, rawMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u, err)
	}
	if len(body) > rawMaxBytes {
		return nil, fmt.Errorf("fetch %s: body is larger than %d bytes", u, rawMaxBytes)
	}
	return body, nil
}

// stageFile copies the local file at path into a new temp dir as its SKILL.md. The
// returned cleanup removes the temp dir.
func stageFile(path string) (dir string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "sapato-import-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	if err := copyRegular(path, filepath.Join(tmp, skillFileName), 0o644); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmp, cleanup, nil
}

// isGitURL reports whether src names a git repository rather than a local folder.
func isGitURL(src string) bool {
	return strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "git@") || strings.HasSuffix(src, ".git")
}

// repoName returns the name of the repository at the git URL src, without .git.
func repoName(src string) string {
	s := strings.TrimSuffix(strings.TrimRight(src, "/"), ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// cloneRepo makes a shallow clone of the git URL src in a new temp dir, which cleanup
// removes. The clone never prompts for credentials and never runs an ext:: helper.
func cloneRepo(src string) (root string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "sapato-import-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	root = filepath.Join(tmp, "repo")
	// -c wins over the user's git config, so ext:: stays refused even where
	// the config allows it.
	cmd := exec.Command("git", "-c", "protocol.ext.allow=never", "clone", "--depth", "1", "--", src, root)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git clone: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return root, cleanup, nil
}

// skillDir returns the folder of the skill inside root. subdir must stay inside root.
func skillDir(root, subdir string) (string, error) {
	if !filepath.IsLocal(subdir) {
		return "", fmt.Errorf("subdir %q must be a path inside the source", subdir)
	}
	return filepath.Join(root, subdir), nil
}

// pathExists reports whether anything, even a broken symlink, is at path.
func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}
