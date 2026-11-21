// Package ops implements sapato's operations. The CLI and TUI both call into it.
package ops

import (
	"fmt"
	"slices"

	"sapato/internal/config"
)

// SelectAgents returns the agents named by ids, in config order. With no ids it
// returns every configured agent. An unknown id is an error.
func SelectAgents(agents []config.Agent, ids []string) ([]config.Agent, error) {
	if len(ids) == 0 {
		return slices.Clone(agents), nil
	}

	var selected []config.Agent
	for _, id := range ids {
		if !slices.ContainsFunc(agents, func(a config.Agent) bool { return a.ID == id }) {
			return nil, fmt.Errorf("unknown agent %q", id)
		}
	}
	for _, a := range agents {
		if slices.Contains(ids, a.ID) {
			selected = append(selected, a)
		}
	}
	return selected, nil
}
