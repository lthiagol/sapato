package ops

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"sapato/internal/hashdir"
	"sapato/internal/vault"
)

// Tarball limits and format (spec §11).
const (
	manifestName      = "sapato-export.json"
	manifestFormat    = 1
	maxArchiveBytes   = 50 << 20
	maxArchiveEntries = 10000
)

// manifest is the sapato-export.json entry of a tarball: the skills it holds, their
// content hashes, and the compatible lists of those that have one (spec §11).
type manifest struct {
	Format     int                 `json:"format"`
	Skills     map[string]string   `json:"skills"`
	Compatible map[string][]string `json:"compatible,omitempty"`
}

// isArchive reports whether src names a gzip tarball. A tarball is always a local file.
func isArchive(src string) bool {
	lower := strings.ToLower(src)
	return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

// importArchive copies the skills of the tarball src into the vault (spec §11). The
// tarball is extracted to a temp dir and checked whole before the vault changes. The
// skills then go in as one step, so a failure leaves the vault as it was.
func importArchive(v *vault.Vault, src string, opts ImportOptions) ([]Action, error) {
	if opts.Subdir != "" {
		return nil, errors.New("subdir does not apply to a tarball")
	}
	tmp, err := os.MkdirTemp("", "sapato-import-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	m, err := extractArchive(src, tmp)
	if err != nil {
		return nil, err
	}
	skills, err := archiveSkills(tmp)
	if err != nil {
		return nil, err
	}
	if err := checkManifest(m, skills); err != nil {
		return nil, err
	}
	if len(opts.Pick) > 0 {
		if skills, err = pickArchiveSkills(skills, opts.Pick); err != nil {
			return nil, err
		}
	}
	if err := checkValid(skills); err != nil {
		return nil, err
	}
	if opts.Name != "" {
		if len(skills) != 1 {
			return nil, errors.New("--name only applies to a single-skill archive")
		}
		if opts.Name != skills[0].Name {
			return nil, fmt.Errorf("name %q does not match archive skill %q", opts.Name, skills[0].Name)
		}
	}

	var changes map[string][]string
	if m != nil {
		changes = make(map[string][]string, len(skills))
		for _, s := range skills {
			changes[s.Name] = m.Compatible[s.Name]
		}
	}

	var copies []skillCopy
	var actions []Action
	var present []string
	for _, s := range skills {
		dest := v.SkillDir(s.Name)
		exists, err := pathExists(dest)
		if err != nil {
			return nil, err
		}
		if exists {
			present = append(present, s.Name)
		}
		copies = append(copies, skillCopy{src: s.Dir, dest: dest})
		actions = append(actions, Action{Op: OpImport, Skill: s.Name, Path: dest})
	}
	if len(present) > 0 && !opts.Force {
		return nil, forceRefusal("already in the vault (use --force): "+strings.Join(present, ", "), "already in the vault: "+strings.Join(present, ", "))
	}
	if opts.DryRun {
		return actions, nil
	}
	var commit func() error
	if changes != nil {
		commit = func() error { return v.EditCompat(changes) }
	}
	if err := placeSkills(copies, commit); err != nil {
		return nil, err
	}
	return actions, nil
}

// extractArchive writes the skill folders of the tarball src into dir and returns
// its manifest, or nil when it has none. Every entry is checked against the spec
// §11 rules before it is written. Dot-paths are skipped, as hashing skips them.
func extractArchive(src, dir string) (*manifest, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var m *manifest
	var total int64
	for n := 1; ; n++ {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return m, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if n > maxArchiveEntries {
			return nil, fmt.Errorf("archive has more than %d entries", maxArchiveEntries)
		}
		// git archive starts with a pax global header. It carries no file, so it is
		// skipped once counted.
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeDir {
			return nil, fmt.Errorf("entry %q: type %q is not allowed, only files and folders", hdr.Name, hdr.Typeflag)
		}
		parts, err := entryParts(hdr.Name)
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg {
			total += hdr.Size
			if total > maxArchiveBytes {
				return nil, fmt.Errorf("archive files total more than %d bytes", maxArchiveBytes)
			}
		}
		if slices.ContainsFunc(parts, func(p string) bool { return strings.HasPrefix(p, ".") }) {
			continue
		}

		if len(parts) == 1 && hdr.Typeflag == tar.TypeReg {
			if parts[0] != manifestName {
				return nil, fmt.Errorf("entry %q: top-level files are not allowed", hdr.Name)
			}
			if m != nil {
				return nil, fmt.Errorf("entry %q: duplicate manifest", hdr.Name)
			}
			if m, err = readManifest(tr); err != nil {
				return nil, err
			}
			continue
		}

		dst := filepath.Join(append([]string{dir}, parts...)...)
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return nil, fmt.Errorf("entry %q: %w", hdr.Name, err)
			}
			continue
		}
		exec := hashdir.Exec(fs.FileMode(hdr.Mode))
		if err := writeExtracted(dst, tr, hdr.Size, exec); err != nil {
			return nil, fmt.Errorf("entry %q: %w", hdr.Name, err)
		}
	}
}

// entryParts splits the name of an archive entry into its path parts. The name must
// be relative, use "/" and have no empty, "." or ".." part. A trailing "/" marks a
// folder and is dropped.
func entryParts(name string) ([]string, error) {
	switch {
	case strings.HasPrefix(name, "/"):
		return nil, fmt.Errorf("entry %q: absolute path", name)
	case strings.ContainsRune(name, '\\'):
		return nil, fmt.Errorf("entry %q: backslash in path", name)
	}
	parts := strings.Split(strings.TrimSuffix(name, "/"), "/")
	for _, p := range parts {
		switch p {
		case "..":
			return nil, fmt.Errorf("entry %q: .. component", name)
		case "", ".":
			return nil, fmt.Errorf("entry %q: empty or . component", name)
		}
	}
	return parts, nil
}

// writeExtracted writes the first size bytes of r to a new file at dst, with the
// exec bits that hashing counts. It creates missing parent folders.
func writeExtracted(dst string, r io.Reader, size int64, exec bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(out, r, size); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// The umask may have stripped bits at create time.
	return os.Chmod(dst, mode)
}

// readManifest parses the manifest that r holds.
func readManifest(r io.Reader) (*manifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", manifestName, err)
	}
	return &m, nil
}

// archiveSkills loads each skill folder extracted into dir, sorted by name, invalid
// ones included. It fails when there are none.
func archiveSkills(dir string) ([]vault.Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var skills []vault.Skill
	for _, e := range entries {
		s, err := vault.Load(filepath.Join(dir, e.Name()), e.Name())
		if err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	if len(skills) == 0 {
		return nil, errors.New("archive holds no skills")
	}
	return skills, nil
}

// checkValid fails on the first invalid skill of skills.
func checkValid(skills []vault.Skill) error {
	for _, s := range skills {
		if !s.Valid {
			return fmt.Errorf("invalid skill %s: %s", s.Name, strings.Join(s.Problems, "; "))
		}
	}
	return nil
}

// pickArchiveSkills keeps the skills named in pick, in that order. Each name must be
// a skill of the archive.
func pickArchiveSkills(skills []vault.Skill, pick []string) ([]vault.Skill, error) {
	chosen := make([]vault.Skill, 0, len(pick))
	for _, name := range pick {
		i := slices.IndexFunc(skills, func(s vault.Skill) bool { return s.Name == name })
		if i < 0 {
			return nil, fmt.Errorf("no skill %q in the archive", name)
		}
		chosen = append(chosen, skills[i])
	}
	return chosen, nil
}

// checkManifest checks, when the archive has a manifest, that it lists exactly the
// skills in the archive with their hashes, and that each compatible list belongs to
// one of them and is valid.
func checkManifest(m *manifest, skills []vault.Skill) error {
	if m == nil {
		return nil
	}
	if m.Format != manifestFormat {
		return fmt.Errorf("%s: unsupported format %d", manifestName, m.Format)
	}
	if len(m.Skills) != len(skills) {
		return fmt.Errorf("%s does not list the skills in the archive", manifestName)
	}
	for _, s := range skills {
		if hash, ok := m.Skills[s.Name]; !ok || hash != s.Hash {
			return fmt.Errorf("%s does not match skill %s", manifestName, s.Name)
		}
	}
	for name, list := range m.Compatible {
		if _, ok := m.Skills[name]; !ok {
			return fmt.Errorf("%s lists compatible agents for %s, which is not in the archive", manifestName, name)
		}
		if err := vault.ValidateCompat(list); err != nil {
			return fmt.Errorf("%s: skill %s: %w", manifestName, name, err)
		}
	}
	return nil
}
