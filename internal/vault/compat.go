package vault

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"

	"sapato/internal/config"
)

// Compatible reports whether skill works with agent (spec §3). A skill without a
// compatible list works with every agent.
func (v *Vault) Compatible(skill, agent string) bool {
	list, ok := v.compat[skill]
	return !ok || slices.Contains(list, agent)
}

// CompatAgents returns the agents skill is compatible with, or nil when it is
// compatible with every agent.
func (v *Vault) CompatAgents(skill string) []string {
	return slices.Clone(v.compat[skill])
}

// ValidateCompat checks a compatible list. It must name at least one agent, and
// each name must have the form of an agent name (spec §2, §3).
func ValidateCompat(agents []string) error {
	if len(agents) == 0 {
		return errors.New("compatible list is empty")
	}
	for _, a := range agents {
		if !config.ValidAgentName(a) {
			return fmt.Errorf("agent name %q is invalid: lowercase letters, digits and dashes, starting with a letter, at most 32 characters", a)
		}
	}
	return nil
}

// parseCompat returns the compatible list of each [skills.<name>] table. Only
// the compatible key is allowed in a table, and its list must be valid.
func parseCompat(path string, skills map[string]map[string]any) (map[string][]string, error) {
	compat := make(map[string][]string)
	for _, name := range slices.Sorted(maps.Keys(skills)) {
		table := skills[name]
		for _, key := range slices.Sorted(maps.Keys(table)) {
			if key != "compatible" {
				return nil, fmt.Errorf("vault %s: [skills.%s]: unknown key %q", path, name, key)
			}
		}
		raw, ok := table["compatible"]
		if !ok {
			continue
		}
		list, ok := toStrings(raw)
		if !ok {
			return nil, fmt.Errorf("vault %s: [skills.%s]: compatible must be a list of agent names", path, name)
		}
		if err := ValidateCompat(list); err != nil {
			return nil, fmt.Errorf("vault %s: [skills.%s]: %w", path, name, err)
		}
		compat[name] = list
	}
	return compat, nil
}

// toStrings returns the elements of a TOML array of strings.
func toStrings(raw any) ([]string, bool) {
	items, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	list := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		list = append(list, s)
	}
	return list, true
}

// CompatEdit is a checked change to the compatible tables of sapato.toml. Write
// applies it.
type CompatEdit struct {
	v       *Vault
	file    string
	edited  []byte
	compat  map[string][]string
	changed bool
}

// EditCompat sets the compatible lists in changes and writes sapato.toml in one
// atomic write. See PlanCompat for the rules.
func (v *Vault) EditCompat(changes map[string][]string) error {
	edit, err := v.PlanCompat(changes)
	if err != nil {
		return err
	}
	return edit.Write()
}

// PlanCompat checks the change EditCompat would make, without writing anything. A
// nil list removes the skill's [skills.<name>] table; removing a table that is not
// there changes nothing. Other bytes of the file are kept, and new tables go at the
// end (spec §3). The result is parsed and must give the lists asked for.
func (v *Vault) PlanCompat(changes map[string][]string) (*CompatEdit, error) {
	for name, list := range changes {
		if list == nil {
			continue
		}
		if err := ValidateCompat(list); err != nil {
			return nil, fmt.Errorf("compat %s: %w", name, err)
		}
	}

	file := filepath.Join(v.Path, manifestName)
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("edit compat in %s: %w", v.Path, err)
	}
	assignments, compat, err := parseManifest(v.Path, data)
	if err != nil {
		return nil, err
	}
	want := maps.Clone(compat)
	for name, list := range changes {
		if list == nil {
			delete(want, name)
		} else {
			want[name] = slices.Clone(list)
		}
	}

	edited, err := editCompatTables(data, changes)
	if err != nil {
		return nil, fmt.Errorf("edit compat in %s: %w", v.Path, err)
	}
	editedAssignments, got, err := parseManifest(v.Path, edited)
	if err != nil {
		return nil, fmt.Errorf("edit compat in %s: %w", v.Path, err)
	}
	if !maps.EqualFunc(got, want, slices.Equal) || !maps.EqualFunc(editedAssignments, assignments, slices.Equal) {
		return nil, fmt.Errorf("edit compat in %s: result does not match the change", v.Path)
	}
	return &CompatEdit{v: v, file: file, edited: edited, compat: got, changed: !bytes.Equal(edited, data)}, nil
}

// Write replaces sapato.toml with the planned bytes, in one atomic write, and
// updates the vault's compatible lists. A change that changes nothing writes nothing.
func (e *CompatEdit) Write() error {
	if e.changed {
		if err := writeFileAtomic(e.file, e.edited); err != nil {
			return fmt.Errorf("edit compat in %s: %w", e.v.Path, err)
		}
	}
	e.v.compat = e.compat
	return nil
}

// editCompatTables returns data with the [skills.<name>] table of each name in
// changes removed. A removed table is cut from its header line through its last
// key/value line; blank and comment lines after that may belong to the next table,
// so they are kept. A non-nil list adds a table at the end.
func editCompatTables(data []byte, changes map[string][]string) ([]byte, error) {
	type header struct {
		start int // offset of the start of the header line
		end   int // offset just past the last key/value line of the table
		keys  []string
	}
	var headers []header
	var p unstable.Parser
	p.Reset(data)
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			keys, offset := headerKey(e)
			headers = append(headers, header{start: lineStart(data, offset), end: lineEnd(data, offset), keys: keys})
		case unstable.KeyValue:
			if len(headers) > 0 {
				h := &headers[len(headers)-1]
				h.end = lineEnd(data, int(e.Raw.Offset+e.Raw.Length))
			}
		}
	}
	if err := p.Error(); err != nil {
		return nil, err
	}

	rootEnd := len(data)
	if len(headers) > 0 {
		rootEnd = headers[0].start
	}
	out := slices.Clone(data[:rootEnd])
	for i, h := range headers {
		next := len(data)
		if i+1 < len(headers) {
			next = headers[i+1].start
		}
		if len(h.keys) == 2 && h.keys[0] == "skills" {
			if _, ok := changes[h.keys[1]]; ok {
				out = append(out, data[h.end:next]...)
				continue
			}
		}
		out = append(out, data[h.start:next]...)
	}

	for _, name := range slices.Sorted(maps.Keys(changes)) {
		list := changes[name]
		if list == nil {
			continue
		}
		if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n\n")) {
			out = append(out, '\n')
		}
		quoted := make([]string, len(list))
		for i, a := range list {
			quoted[i] = strconv.Quote(a)
		}
		out = fmt.Appendf(out, "[skills.%s]\ncompatible = [%s]\n", tomlKey(name), strings.Join(quoted, ", "))
	}
	return out, nil
}

// lineStart returns the offset of the start of the line that holds offset.
func lineStart(data []byte, offset int) int {
	return bytes.LastIndexByte(data[:offset], '\n') + 1
}

// lineEnd returns the offset just past the line that holds offset, newline included.
func lineEnd(data []byte, offset int) int {
	if i := bytes.IndexByte(data[offset:], '\n'); i >= 0 {
		return offset + i + 1
	}
	return len(data)
}

// headerKey returns the parts of the key of a table header, and the offset of its
// first part in the document. A table node has no range of its own.
func headerKey(n *unstable.Node) ([]string, int) {
	var parts []string
	offset := -1
	for it := n.Key(); it.Next(); {
		if offset < 0 {
			offset = int(it.Node().Raw.Offset)
		}
		parts = append(parts, string(it.Node().Data))
	}
	return parts, offset
}

// tomlKey returns name as a TOML key: bare when it only has bare-key characters,
// quoted otherwise.
func tomlKey(name string) string {
	bare := name != ""
	for _, r := range name {
		bare = bare && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}
	if bare {
		return name
	}
	return strconv.Quote(name)
}

// writeFileAtomic replaces file with data through a temp file in the same
// folder, keeping the file's mode.
func writeFileAtomic(file string, data []byte) error {
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+"-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := writeTemp(tmp, data, info.Mode().Perm()); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, file); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// writeTemp writes data to f, sets its mode and closes it.
func writeTemp(f *os.File, data []byte, mode os.FileMode) error {
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
