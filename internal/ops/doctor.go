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
	SeverityInfo  Severity = "info"
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
	FindingNotHere      FindingKind = "not-configured-here"
	FindingIncompatible FindingKind = "incompatible"
	FindingCompatHere   FindingKind = "compat-not-configured-here"
	FindingIncompatRead FindingKind = "incompatible-read"
)

// Finding is one problem Doctor reports.
type Finding struct {
	Severity Severity
	Kind     FindingKind
	Agent    string // agent the finding is about; empty for vault findings
	Skill    string // skill the finding is about; empty when none
	Message  string
}

// Doctor checks the vault and the agents' target dirs and reports what is wrong
// (spec §7). Invalid vault skills and corrupt locks are errors. Modified,
// missing and orphaned entries, managed copies in an agent the skill is not
// compatible with, and skills visible to one agent from two dirs, are warnings.
// Assignments and compatible names that are not configured here, and skills an
// agent reads that it is not compatible with, are info. A corrupt lock is
// reported and its agent is not checked further. Doctor changes nothing.
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
		if kind, msg := entryFinding(r); kind != "" {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Kind:     kind,
				Agent:    r.Agent,
				Skill:    r.Skill,
				Message:  fmt.Sprintf("%s in %s: %s", r.Skill, r.Agent, msg),
			})
		}
		if !r.Compatible && installedState(r.Status) {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Kind:     FindingIncompatible,
				Agent:    r.Agent,
				Skill:    r.Skill,
				Message:  fmt.Sprintf("%s in %s: installed in an agent the skill is not compatible with", r.Skill, r.Agent),
			})
		}
	}

	for _, id := range unconfigured(v.Assignments, agents) {
		findings = append(findings, Finding{
			Severity: SeverityInfo,
			Kind:     FindingNotHere,
			Agent:    id,
			Message:  fmt.Sprintf("assignments for %s: not configured here, skipped by sync", id),
		})
	}

	findings = append(findings, compatFindings(v, skills, agents)...)

	reads, err := incompatReadFindings(v, agents)
	if err != nil {
		return nil, fmt.Errorf("doctor: %w", err)
	}
	findings = append(findings, reads...)

	overlaps, err := overlapFindings(agents)
	if err != nil {
		return nil, fmt.Errorf("doctor: %w", err)
	}
	return append(findings, overlaps...), nil
}

// installedState reports whether a row has a managed copy in its agent.
func installedState(s State) bool {
	return s == Synced || s == Outdated || s == Modified || s == Orphaned
}

// compatFindings reports compatible names that are not configured here (spec §7).
func compatFindings(v *vault.Vault, skills []vault.Skill, agents []config.Agent) []Finding {
	var findings []Finding
	for _, s := range skills {
		for _, id := range v.CompatAgents(s.Name) {
			if slices.ContainsFunc(agents, func(a config.Agent) bool { return a.ID == id }) {
				continue
			}
			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Kind:     FindingCompatHere,
				Agent:    id,
				Skill:    s.Name,
				Message:  fmt.Sprintf("%s is compatible with %s, not configured here", s.Name, id),
			})
		}
	}
	return findings
}

// incompatReadFindings reports each skill in an agent's dir that another agent
// reads through its ReadDirs (spec §2) when the skill is not compatible with
// that reader (spec §7).
func incompatReadFindings(v *vault.Vault, agents []config.Agent) ([]Finding, error) {
	var findings []Finding
	for _, reader := range agents {
		for _, dir := range reader.ReadDirs {
			owner, ok := agentAtDir(agents, dir)
			if !ok || owner.ID == reader.ID {
				continue
			}
			names, err := target.Scan(owner.Path)
			if err != nil {
				return nil, err
			}
			for _, name := range names {
				if v.Compatible(name, reader.ID) {
					continue
				}
				findings = append(findings, Finding{
					Severity: SeverityInfo,
					Kind:     FindingIncompatRead,
					Agent:    reader.ID,
					Skill:    name,
					Message: fmt.Sprintf("%s in %s is read by %s, which the skill is not compatible with",
						name, owner.ID, reader.ID),
				})
			}
		}
	}
	return findings, nil
}

// agentAtDir returns the configured agent whose skills dir is dir, comparing
// resolved paths.
func agentAtDir(agents []config.Agent, dir string) (config.Agent, bool) {
	want := resolveDir(dir)
	for _, a := range agents {
		if resolveDir(a.Path) == want {
			return a, true
		}
	}
	return config.Agent{}, false
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
// own and one of the fixed dirs it also reads (spec §2).
func overlapFindings(agents []config.Agent) ([]Finding, error) {
	var findings []Finding
	for _, reader := range agents {
		if len(reader.ReadDirs) == 0 {
			continue
		}
		own, err := target.Scan(reader.Path)
		if err != nil {
			return nil, err
		}
		seen := []string{resolveDir(reader.Path)}
		for _, dir := range reader.ReadDirs {
			if slices.Contains(seen, resolveDir(dir)) {
				continue
			}
			seen = append(seen, resolveDir(dir))
			other, err := target.Scan(dir)
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
						name, reader.ID, reader.Path, dir),
				})
			}
		}
	}
	return findings, nil
}

// resolveDir returns dir with symlinks resolved. A dir that does not exist is
// cleaned instead.
func resolveDir(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return filepath.Clean(dir)
}
