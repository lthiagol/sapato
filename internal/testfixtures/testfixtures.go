// Package testfixtures locates the shared fixtures under testdata/ and copies
// them into temporary directories for tests that mutate them.
package testfixtures

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Root returns the absolute path to the testdata directory.
func Root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// CopyTree copies the fixture tree at src into t.TempDir() and returns the new
// path. The copy keeps src's base name. File modes and symlinks are preserved.
func CopyTree(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("copy %s: %w", path, err)
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		switch {
		case d.IsDir():
			if err := os.Mkdir(target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("copy %s: %w", path, err)
			}
			return nil
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read link %s: %w", path, err)
			}
			if err := os.Symlink(link, target); err != nil {
				return fmt.Errorf("copy link %s: %w", path, err)
			}
			return nil
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("copy %s: unsupported file type %s", path, info.Mode().Type())
		}
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	// The umask may have stripped bits at create time; set the mode explicitly.
	if err := os.Chmod(dst, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}
	return nil
}
