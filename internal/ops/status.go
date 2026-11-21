package ops

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/target"
	"sapato/internal/vault"
)

// State is the status of one skill in one agent (spec §6).
type State string

const (
	NotInstalled State = "not-installed"
	Invalid      State = "invalid"
	Synced       State = "synced"
	Outdated     State = "outdated"
	Modified     State = "modified"
	Orphaned     State = "orphaned"
	Unmanaged    State = "unmanaged"
	Missing      State = "missing"
)

// Row is the status of one skill in one agent.
type Row struct {
	Skill  string `json:"skill"`
	Agent  string `json:"agent"`
	Status State  `json:"status"`
}

// Filter narrows Status. Skills are path.Match globs; a skill matches when any
// glob matches it. An empty Skills matches every skill.
type Filter struct {
	Skills []string
}

// presence is what Status knows about one skill in one agent. Hashes are only
// meaningful when the matching in* flag is set.
type presence struct {
	inVault    bool
	valid      bool
	vaultHash  string
	inLock     bool
	lockHash   string
	inTarget   bool
	targetHash string
}

// classify applies the precedence of spec §6. A target that differs from its
// lock entry is reported as modified before any vault comparison.
func classify(p presence) State {
	switch {
	case !p.inLock && p.inTarget:
		return Unmanaged
	case !p.inLock && p.inVault && !p.valid:
		return Invalid
	case !p.inLock:
		return NotInstalled
	case !p.inTarget:
		return Missing
	case p.targetHash != p.lockHash:
		return Modified
	case !p.inVault:
		return Orphaned
	case p.vaultHash != p.lockHash:
		return Outdated
	default:
		return Synced
	}
}

// Status reports the status of each skill for each agent, over the vault and
// the agents' target dirs. Rows are sorted by skill name, then in the order of
// agents. A pair with the skill absent from the vault, the target and the lock
// has no row.
func Status(v *vault.Vault, agents []config.Agent, f Filter) ([]Row, error) {
	for _, p := range f.Skills {
		if _, err := path.Match(p, ""); err != nil {
			return nil, fmt.Errorf("status: bad skill pattern %q: %w", p, err)
		}
	}

	skills, err := v.Skills()
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	vaultHash := make(map[string]string, len(skills))
	valid := make(map[string]bool, len(skills))
	names := make(map[string]bool, len(skills))
	for _, s := range skills {
		vaultHash[s.Name] = s.Hash
		valid[s.Name] = s.Valid
		names[s.Name] = true
	}

	type view struct {
		agent   config.Agent
		lock    target.Lock
		folders map[string]bool
	}
	views := make([]view, 0, len(agents))
	for _, a := range agents {
		lock, err := target.ReadLock(a.Path)
		if err != nil {
			return nil, fmt.Errorf("status: %w", err)
		}
		folders, err := target.Scan(a.Path)
		if err != nil {
			return nil, fmt.Errorf("status: %w", err)
		}
		vw := view{agent: a, lock: lock, folders: make(map[string]bool, len(folders))}
		for _, name := range folders {
			vw.folders[name] = true
			names[name] = true
		}
		for name := range lock.Skills {
			names[name] = true
		}
		views = append(views, vw)
	}

	rows := []Row{}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		if !matchesAny(f.Skills, name) {
			continue
		}
		for _, vw := range views {
			var p presence
			p.vaultHash, p.inVault = vaultHash[name]
			p.valid = valid[name]
			var entry target.LockEntry
			entry, p.inLock = vw.lock.Skills[name]
			p.lockHash = entry.Hash
			p.inTarget = vw.folders[name]
			if !p.inVault && !p.inLock && !p.inTarget {
				continue
			}
			if p.inTarget {
				p.targetHash, err = hashdir.Hash(filepath.Join(vw.agent.Path, name))
				if err != nil {
					return nil, fmt.Errorf("status: %w", err)
				}
			}
			rows = append(rows, Row{Skill: name, Agent: vw.agent.ID, Status: classify(p)})
		}
	}
	return rows, nil
}

// matchesAny reports whether name matches one of the globs, or whether there
// are no globs. The globs were checked by Status.
func matchesAny(globs []string, name string) bool {
	if len(globs) == 0 {
		return true
	}
	for _, g := range globs {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
	}
	return false
}
