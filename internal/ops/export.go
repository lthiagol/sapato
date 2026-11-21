package ops

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"sapato/internal/hashdir"
	"sapato/internal/vault"
)

// archiveTime is the mtime of every archive entry, so an export is byte-identical
// across runs.
var archiveTime = time.Unix(0, 0)

// Export writes the vault skills selected by skills to w as a gzip tarball (spec
// §11). Selection works as for install. An invalid skill that a literal name selects
// is refused, and nothing is written. An invalid skill that only a glob selects is
// left out and returned as an OpSkip action.
func Export(v *vault.Vault, skills []string, w io.Writer) ([]Action, error) {
	picked, skips, err := pickExport(v, skills)
	if err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := writeArchive(tw, picked); err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	return skips, nil
}

// ExportName returns the default file name of an export of skills at now (spec §7):
// <name>-<version or 8-char hash>.skill.tar.gz for one skill, and
// skills-<date>.skill.tar.gz for several. It applies the same checks as Export.
func ExportName(v *vault.Vault, skills []string, now time.Time) (string, error) {
	picked, _, err := pickExport(v, skills)
	if err != nil {
		return "", fmt.Errorf("export: %w", err)
	}
	if len(picked) > 1 {
		return "skills-" + now.UTC().Format("2006-01-02") + ".skill.tar.gz", nil
	}
	s := picked[0]
	id := s.Version
	if id == "" || strings.ContainsAny(id, `/\`) {
		_, hex, _ := strings.Cut(s.Hash, ":")
		id = hex[:8]
	}
	return s.Name + "-" + id + ".skill.tar.gz", nil
}

// pickExport returns the valid vault skills that skills select, sorted by name, and
// the skips for invalid skills that only a glob selects.
func pickExport(v *vault.Vault, skills []string) ([]vault.Skill, []Action, error) {
	if len(skills) == 0 {
		return nil, nil, errors.New("no skills given")
	}
	all, err := v.Skills()
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(all))
	for _, s := range all {
		names = append(names, s.Name)
	}
	if err := checkMatched(skills, names); err != nil {
		return nil, nil, err
	}

	literal := literalNames(skills)
	var picked []vault.Skill
	var skips []Action
	for _, s := range all {
		if !matchesAny(skills, s.Name) {
			continue
		}
		switch {
		case s.Valid:
			picked = append(picked, s)
		case literal[s.Name]:
			return nil, nil, fmt.Errorf("invalid skill %s: %s", s.Name, strings.Join(s.Problems, "; "))
		default:
			skips = append(skips, Action{Op: OpSkip, Skill: s.Name, Reason: "invalid"})
		}
	}
	if len(picked) == 0 {
		return nil, nil, errors.New("no valid skill to export")
	}
	return picked, skips, nil
}

// writeArchive writes the manifest, then each skill folder in the order given.
func writeArchive(tw *tar.Writer, skills []vault.Skill) error {
	m := manifest{Format: manifestFormat, Skills: make(map[string]string, len(skills))}
	for _, s := range skills {
		m.Skills[s.Name] = s.Hash
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := writeEntry(tw, &tar.Header{Name: manifestName, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(data))}, data); err != nil {
		return err
	}
	for _, s := range skills {
		if err := writeSkill(tw, s); err != nil {
			return fmt.Errorf("%s: %w", s.Name, err)
		}
	}
	return nil
}

// writeSkill writes the folder of s: its directories and the files that count
// toward its hash, in path order. Each folder is written before its contents.
func writeSkill(tw *tar.Writer, s vault.Skill) error {
	files, err := hashdir.Files(s.Dir)
	if err != nil {
		return err
	}
	if err := writeEntry(tw, &tar.Header{Name: s.Name + "/", Typeflag: tar.TypeDir, Mode: 0o755}, nil); err != nil {
		return err
	}
	written := map[string]bool{s.Name: true}
	for _, f := range files {
		parts := strings.Split(f.Rel, "/")
		for i := 1; i < len(parts); i++ {
			dir := path.Join(s.Name, path.Join(parts[:i]...))
			if written[dir] {
				continue
			}
			written[dir] = true
			if err := writeEntry(tw, &tar.Header{Name: dir + "/", Typeflag: tar.TypeDir, Mode: 0o755}, nil); err != nil {
				return err
			}
		}

		data, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		mode := int64(0o644)
		if hashdir.Exec(f.Mode) {
			mode = 0o755
		}
		hdr := &tar.Header{Name: path.Join(s.Name, f.Rel), Typeflag: tar.TypeReg, Mode: mode, Size: int64(len(data))}
		if err := writeEntry(tw, hdr, data); err != nil {
			return err
		}
	}
	return nil
}

// writeEntry writes one entry with the fixed mtime that keeps exports identical.
func writeEntry(tw *tar.Writer, hdr *tar.Header, data []byte) error {
	hdr.ModTime = archiveTime
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
