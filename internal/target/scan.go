package target

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Scan returns the sorted names of the skill folders in dir. Entries that are
// not directories and names starting with "." are skipped. A missing dir has
// no skills.
func Scan(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
