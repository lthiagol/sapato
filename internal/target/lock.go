// Package target reads and writes the per-agent state Sapato keeps in an agent's
// skills directory: the lock file and the list of installed skill folders.
package target

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	lockName    = ".sapato-lock.json"
	lockVersion = 1
)

// Lock is the lock file in a target directory (spec §5).
type Lock struct {
	Version int                  `json:"version"`
	Skills  map[string]LockEntry `json:"skills"`
}

// LockEntry records a skill Sapato installed into a target.
type LockEntry struct {
	Hash        string    `json:"hash"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
}

// ReadLock reads the lock file in dir. A missing file yields an empty lock.
func ReadLock(dir string) (Lock, error) {
	path := filepath.Join(dir, lockName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Lock{Version: lockVersion, Skills: map[string]LockEntry{}}, nil
	}
	if err != nil {
		return Lock{}, fmt.Errorf("read lock %s: %w", path, err)
	}
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return Lock{}, fmt.Errorf("parse lock %s: %w", path, err)
	}
	if l.Version != lockVersion {
		return Lock{}, fmt.Errorf("parse lock %s: unsupported version %d", path, l.Version)
	}
	if l.Skills == nil {
		l.Skills = map[string]LockEntry{}
	}
	return l, nil
}

// WriteLock writes l to the lock file in dir. It writes a temp file in dir and
// renames it into place, and leaves the file untouched when the contents are
// already identical.
func WriteLock(dir string, l Lock) error {
	path := filepath.Join(dir, lockName)
	l.Version = lockVersion
	if l.Skills == nil {
		l.Skills = map[string]LockEntry{}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lock %s: %w", path, err)
	}
	data = append(data, '\n')

	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, data):
		return nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("read lock %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, lockName+".tmp-*")
	if err != nil {
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	return nil
}
