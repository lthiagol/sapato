package ops

import (
	"fmt"

	"sapato/internal/hashdir"
	"sapato/internal/target"
)

// File is a file of a skill folder, as the skill screen lists it.
type File struct {
	Path string // slash-separated path relative to the skill folder
	Size int64  // size in bytes
	Exec bool   // an exec bit is set, as in the content hash
}

// SkillFiles lists the files of the skill folder dir that count toward its hash
// (spec §4), in the order hashdir.Files gives them. dir must exist.
func SkillFiles(dir string) ([]File, error) {
	files, err := hashdir.Files(dir)
	if err != nil {
		return nil, fmt.Errorf("files %s: %w", dir, err)
	}
	out := make([]File, len(files))
	for i, f := range files {
		out[i] = File{Path: f.Rel, Size: f.Size, Exec: hashdir.Exec(f.Mode)}
	}
	return out, nil
}

// LockEntry returns the lock entry of skill in the target at agentDir. ok is false
// when the lock has no entry for it, which is also the case for an unmanaged copy.
func LockEntry(agentDir, skill string) (entry target.LockEntry, ok bool, err error) {
	lock, err := target.ReadLock(agentDir)
	if err != nil {
		return target.LockEntry{}, false, fmt.Errorf("lock entry %s: %w", skill, err)
	}
	entry, ok = lock.Skills[skill]
	return entry, ok, nil
}
