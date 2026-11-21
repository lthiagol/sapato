package ops

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"sapato/internal/config"
	"sapato/internal/target"
	"sapato/internal/vault"
)

// Severity ranks a Finding. Doctor exits 1 when any finding is an error.
type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
)

// FindingKind names what a Finding is about.
type FindingKind string

const (
	FindingInvalidSkill FindingKind = "invalid-skill"
	FindingCorruptLock  FindingKind = "corrupt-lock"
	FindingOverlap      FindingKind = "overlap"
	FindingModified     FindingKind = "modified"
	FindingMissing      FindingKind = "missing"
	FindingOrphaned     FindingKind = "orphaned"
)

// Finding is one problem Doctor reports.
type Finding struct {
	Severity Severity
	Kind     FindingKind
	Agent    string // agent the finding is about; empty for vault findings
	Skill    string // skill the finding is about; empty when none
	Message  string
}

// readOverlaps lists the agents whose skills dirs each agent also reads (spec §2).
// Each entry is resolved against the configured paths of the agents it names.
var readOverlaps = map[string][]string{
	"cursor":   {"claude"},
	"opencode": {"claude", "agents"},
	"pi":       {"agents"},
}

// Doctor checks the vault and the agents' target dirs and reports what is wrong
// (spec §7). Invalid vault skills and corrupt locks are errors. Modified,
// missing and orphaned entries, and skills visible to one agent from two dirs,
// are warnings. A corrupt lock is reported and its agent is not checked further.
// Doctor changes nothing.
func Doctor(v *vault.Vault, agents []config.Agent) ([]Finding, error) {
	skills, err := v.Skills()
	if err != nil {
		return nil, fmt.Errorf("doctor: %w", err)
	}
	var findings []Finding
	for _, s := range skills {
		if !s.Valid {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Kind:     FindingInvalidSkill,
				Skill:    s.Name,
				Message:  fmt.Sprintf("vault skill %s is invalid: %s", s.Name, strings.Join(s.Problems, "; ")),
			})
		}
	}

	var readable []config.Agent
	for _, a := range agents {
		if _, err := target.ReadLock(a.Path); err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Kind:     FindingCorruptLock,
				Agent:    a.ID,
				Message:  fmt.Sprintf("%s: %v", a.ID, err),
			})
			continue
		}
		readable = append(readable, a)
	}

	rows, err := Status(v, readable, Filter{})
	if err != nil {
		return nil, fmt.Errorf("doctor: %w", err)
	}
	for _, r := range rows {
		kind, msg := entryFinding(r)
		if kind == "" {
			continue
		}
		findings = append(findings, Finding{
			Severity: SeverityWarn,
			Kind:     kind,
			Agent:    r.Agent,
			Skill:    r.Skill,
			Message:  fmt.Sprintf("%s in %s: %s", r.Skill, r.Agent, msg),
		})
	}

	overlaps, err := overlapFindings(agents)
	if err != nil {
		return nil, fmt.Errorf("doctor: %w", err)
	}
	return append(findings, overlaps...), nil
}

// entryFinding returns the finding kind and text for a row that needs attention,
// or an empty kind for a row that is fine.
func entryFinding(r Row) (FindingKind, string) {
	switch r.Status {
	case Modified:
		return FindingModified, "installed copy has local edits"
	case Missing:
		return FindingMissing, "in the lock but its folder is gone"
	case Orphaned:
		return FindingOrphaned, "installed but no longer in the vault"
	}
	return "", ""
}

// overlapFindings reports each skill that an agent can see from two dirs: its
// own and the dir of an agent it also reads.
func overlapFindings(agents []config.Agent) ([]Finding, error) {
	var findings []Finding
	for _, reader := range agents {
		for _, id := range readOverlaps[reader.ID] {
			i := slices.IndexFunc(agents, func(a config.Agent) bool { return a.ID == id })
			if i < 0 || filepath.Clean(agents[i].Path) == filepath.Clean(reader.Path) {
				continue
			}
			own, err := target.Scan(reader.Path)
			if err != nil {
				return nil, err
			}
			other, err := target.Scan(agents[i].Path)
			if err != nil {
				return nil, err
			}
			for _, name := range own {
				if !slices.Contains(other, name) {
					continue
				}
				findings = append(findings, Finding{
					Severity: SeverityWarn,
					Kind:     FindingOverlap,
					Agent:    reader.ID,
					Skill:    name,
					Message: fmt.Sprintf("%s is visible to %s from both %s and %s",
						name, reader.ID, reader.Path, agents[i].Path),
				})
			}
		}
	}
	return findings, nil
}
