// Package hashdir computes a deterministic content hash of a skill folder.
package hashdir

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type file struct {
	rel  string
	exec bool
	data []byte
}

// Hash returns "sha256:<hex>" over the regular files under dir. Paths with a
// component starting with "." and symlinks are skipped.
func Hash(dir string) (string, error) {
	var files []file
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			if !d.IsDir() {
				return fmt.Errorf("%s is not a directory", dir)
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, file{
			rel:  filepath.ToSlash(rel),
			exec: info.Mode().Perm()&0o111 != 0,
			data: data,
		})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", dir, err)
	}

	slices.SortFunc(files, func(a, b file) int { return strings.Compare(a.rel, b.rel) })

	h := sha256.New()
	for _, f := range files {
		mode := "-"
		if f.exec {
			mode = "x"
		}
		fmt.Fprintf(h, "%s\n%s\n%d\n", f.rel, mode, len(f.data))
		h.Write(f.data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
