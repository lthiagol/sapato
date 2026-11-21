package ops

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/vault"
)

// ChangeKind says how a file differs between an installed copy and the vault.
type ChangeKind string

const (
	ChangeAdded   ChangeKind = "added"   // in the vault only: an update would add it
	ChangeRemoved ChangeKind = "removed" // in the installed copy only: an update would remove it
	ChangeChanged ChangeKind = "changed" // in both, with other content or other mode bits
)

// FileChange is one file of a skill that differs from its installed copy.
type FileChange struct {
	Rel    string     // slash-separated path in the skill folder
	Kind   ChangeKind // how the file differs
	Binary bool       // the content differs and is binary, so Diff is empty
	Mode   bool       // the exec bit differs, as in the content hash
	Diff   string     // unified diff of a text file, from the copy to the vault
}

// Diff lists the files whose content or mode bits differ between the installed
// copy of skill in agent and the vault skill. Files are compared with the same
// rules as the content hash (spec §4). In a text diff the installed copy is the
// "-" side and the vault is the "+" side, so applying it turns the copy into
// the vault skill. The skill must be in the vault and its folder must exist in
// the agent; an invalid vault skill can still be diffed.
func Diff(v *vault.Vault, agent config.Agent, skill string) ([]FileChange, error) {
	skills, err := v.Skills()
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}
	i := slices.IndexFunc(skills, func(s vault.Skill) bool { return s.Name == skill })
	if i < 0 {
		return nil, fmt.Errorf("diff: no skill %q in vault", skill)
	}

	installed := filepath.Join(agent.Path, skill)
	if _, err := os.Stat(installed); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("diff %s in %s: not installed", skill, agent.ID)
		}
		return nil, fmt.Errorf("diff %s in %s: %w", skill, agent.ID, err)
	}
	inst, err := filesByRel(installed)
	if err != nil {
		return nil, fmt.Errorf("diff %s in %s: %w", skill, agent.ID, err)
	}
	want, err := filesByRel(skills[i].Dir)
	if err != nil {
		return nil, fmt.Errorf("diff %s in %s: %w", skill, agent.ID, err)
	}

	rels := make(map[string]bool, len(inst)+len(want))
	for rel := range inst {
		rels[rel] = true
	}
	for rel := range want {
		rels[rel] = true
	}
	var changes []FileChange
	for _, rel := range slices.Sorted(maps.Keys(rels)) {
		c, err := compareFile(rel, inst[rel], want[rel])
		if err != nil {
			return nil, fmt.Errorf("diff %s in %s: %w", skill, agent.ID, err)
		}
		if c != nil {
			changes = append(changes, *c)
		}
	}
	return changes, nil
}

// filesByRel lists the hashed files of dir keyed by their slash-separated path.
func filesByRel(dir string) (map[string]hashdir.File, error) {
	files, err := hashdir.Files(dir)
	if err != nil {
		return nil, err
	}
	byRel := make(map[string]hashdir.File, len(files))
	for _, f := range files {
		byRel[f.Rel] = f
	}
	return byRel, nil
}

// compareFile returns how rel differs between the installed file and the vault
// file, or nil when they are the same. A side is absent when its file is
// missing from that folder; a zero File stands for the absent side.
func compareFile(rel string, inst, want hashdir.File) (*FileChange, error) {
	var oldData, newData []byte
	var err error
	c := FileChange{Rel: rel}
	switch {
	case want.Path == "":
		c.Kind = ChangeRemoved
		if oldData, err = os.ReadFile(inst.Path); err != nil {
			return nil, err
		}
	case inst.Path == "":
		c.Kind = ChangeAdded
		if newData, err = os.ReadFile(want.Path); err != nil {
			return nil, err
		}
	default:
		if oldData, err = os.ReadFile(inst.Path); err != nil {
			return nil, err
		}
		if newData, err = os.ReadFile(want.Path); err != nil {
			return nil, err
		}
		c.Kind = ChangeChanged
		c.Mode = hashdir.Exec(inst.Mode) != hashdir.Exec(want.Mode)
		if bytes.Equal(oldData, newData) && !c.Mode {
			return nil, nil
		}
	}
	if bytes.Equal(oldData, newData) {
		return &c, nil
	}
	if bytes.IndexByte(oldData, 0) >= 0 || bytes.IndexByte(newData, 0) >= 0 {
		c.Binary = true
		return &c, nil
	}
	c.Diff = unifiedDiff(sideName("installed", inst.Path, rel), sideName("vault", want.Path, rel), string(oldData), string(newData))
	return &c, nil
}

// sideName labels one side of a text diff, or /dev/null for an absent side.
func sideName(side, path, rel string) string {
	if path == "" {
		return "/dev/null"
	}
	return side + "/" + rel
}

// contextLines is the number of unchanged lines shown around a change.
const contextLines = 3

// edit is one line of a line diff: ' ' for a line both sides share, '-' for a
// line only the old side has, '+' for a line only the new side has.
type edit struct {
	op   byte
	line string
}

// unifiedDiff returns the unified diff from oldText to newText with three lines
// of context, under the given file labels. It returns "" when they are equal.
func unifiedDiff(oldName, newName, oldText, newText string) string {
	ops := lineEdits(splitLines(oldText), splitLines(newText))
	hunks := groupHunks(ops)
	if len(hunks) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", oldName, newName)
	var oldN, newN, pos int // lines consumed by ops[:pos]
	for _, h := range hunks {
		for ; pos < h.from; pos++ {
			if ops[pos].op != '+' {
				oldN++
			}
			if ops[pos].op != '-' {
				newN++
			}
		}
		var oldCount, newCount int
		for _, e := range ops[h.from:h.to] {
			if e.op != '+' {
				oldCount++
			}
			if e.op != '-' {
				newCount++
			}
		}
		fmt.Fprintf(&b, "@@ -%s +%s @@\n", hunkRange(oldN, oldCount), hunkRange(newN, newCount))
		for _, e := range ops[h.from:h.to] {
			b.WriteByte(e.op)
			b.WriteString(e.line)
			if !strings.HasSuffix(e.line, "\n") {
				b.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	return b.String()
}

// splitLines splits s after each newline. A final line without one is kept.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lineEdits returns an edit script that turns a into b, using a longest common
// subsequence of the lines.
func lineEdits(a, b []string) []edit {
	// lcs[i][j] is the length of the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var ops []edit
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			ops = append(ops, edit{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, edit{'-', a[i]})
			i++
		default:
			ops = append(ops, edit{'+', b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, edit{'-', a[i]})
	}
	for ; j < len(b); j++ {
		ops = append(ops, edit{'+', b[j]})
	}
	return ops
}

// hunk is ops[from:to]: a run of changes with their context.
type hunk struct{ from, to int }

// groupHunks cuts the edit script into hunks. Changes separated by no more than
// twice the context are shown in one hunk.
func groupHunks(ops []edit) []hunk {
	var hunks []hunk
	for i := 0; i < len(ops); {
		if ops[i].op == ' ' {
			i++
			continue
		}
		from := max(0, i-contextLines)
		to := i
		for {
			for to < len(ops) && ops[to].op != ' ' {
				to++
			}
			next := to
			for next < len(ops) && ops[next].op == ' ' {
				next++
			}
			if next < len(ops) && next-to <= 2*contextLines {
				to = next
				continue
			}
			to = min(next, to+contextLines)
			break
		}
		hunks = append(hunks, hunk{from, to})
		i = to
	}
	return hunks
}

// hunkRange formats the line range of a hunk, given the lines consumed before
// it and the lines it covers. An empty range starts at the line before it.
func hunkRange(consumed, count int) string {
	start := consumed + 1
	if count == 0 {
		start = consumed
	}
	if count == 1 {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}
