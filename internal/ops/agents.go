// Package ops implements sapato's operations. The CLI and TUI both call into it.
package ops

import (
	"fmt"
	"slices"

	"sapato/internal/config"
)

// SelectAgents returns the agents named by ids, in config order. An id is
// selected even when its agent is disabled. With no ids it returns the enabled
// agents. An unknown id is an error.
func SelectAgents(agents []config.Agent, ids []string) ([]config.Agent, error) {
	var selected []config.Agent
	if len(ids) == 0 {
		for _, a := range agents {
			if a.Enabled {
				selected = append(selected, a)
			}
		}
		return selected, nil
	}

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
