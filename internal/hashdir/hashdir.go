// Package hashdir lists the files of a skill folder and computes their
// deterministic content hash.
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

// File is a regular file of a skill folder that counts toward its hash.
type File struct {
	Rel  string      // slash-separated path relative to the folder
	Path string      // path on disk
	Mode fs.FileMode // permission bits
	Size int64       // size in bytes
}

// Files lists the regular files under dir, sorted by Rel. Paths with a
// component starting with "." and symlinks are skipped.
func Files(dir string) ([]File, error) {
	var files []File
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
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, File{
			Rel:  filepath.ToSlash(rel),
			Path: path,
			Mode: info.Mode().Perm(),
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Rel, b.Rel) })
	return files, nil
}

// Exec reports whether a file with mode counts as executable in the hash. Only
// the exec bits matter, so diff and status agree on mode changes.
func Exec(mode fs.FileMode) bool {
	return mode&0o111 != 0
}

// Hash returns "sha256:<hex>" over the files that Files lists in dir.
func Hash(dir string) (string, error) {
	files, err := Files(dir)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", dir, err)
	}

	h := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return "", fmt.Errorf("hash %s: %w", dir, err)
		}
		mode := "-"
		if Exec(f.Mode) {
			mode = "x"
		}
		fmt.Fprintf(h, "%s\n%s\n%d\n", f.Rel, mode, len(data))
		h.Write(data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
