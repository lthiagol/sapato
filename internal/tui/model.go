// Package tui is sapato's interactive UI. It calls the ops package through a
// Backend and holds no business logic of its own.
package tui

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/config"
	"sapato/internal/ops"
)

// mode is the screen the model is showing. Keys go to the handler of the mode.
type mode int

const (
	modeMatrix mode = iota
	modeFilter
	modeConfirm
	modeDiff
	modeHelp
)

const nothingSelected = "nothing selected"

// cell is one skill in one agent.
type cell struct {
	skill string
	agent string
}

// action is one mutation of a cell, as the UI runs it through the Backend.
// force is set only once the user has accepted a force prompt.
type action struct {
	op    string
	skill string
	agent config.Agent
	force bool
}

// pending is an action that waits for y or n. A plain confirm shows plan. A
// force prompt has act.force set and shows the reason ops refused without it.
type pending struct {
	act    action
	plan   []ops.Action
	reason string
}

// Model is the state of the UI. Update is a pure transition from a message to
// a new state and at most one command; the commands call the Backend.
type Model struct {
	backend Backend
	agents  []config.Agent
	keys    keyMap
	styles  styles

	cells  map[cell]ops.State // status of each skill in each agent; absent means blank
	names  []string           // every skill name from the last load, sorted
	skills []string           // names that match query: the matrix rows
	query  string
	row    int // cursor, as an index into skills
	col    int // cursor, as an index into agents
	top    int // first row shown, for scrolling

	mode      mode
	filter    textinput.Model
	pending   pending
	diff      viewport.Model
	diffTitle string

	result    string // last action result or error, on the status line
	resultErr bool

	width, height int
}

// New returns the UI for the agents as matrix columns. The caller decides
// which agents are shown; New does not filter them.
func New(b Backend, agents []config.Agent, noColor bool) Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	// A static cursor keeps the model free of blink commands.
	filter.Cursor.SetMode(cursor.CursorStatic)
	return Model{
		backend: b,
		agents:  agents,
		keys:    newKeyMap(),
		styles:  newStyles(noColor),
		cells:   map[cell]ops.State{},
		filter:  filter,
		diff:    viewport.New(0, 0),
	}
}

// loadedMsg carries the result of a status load.
type loadedMsg struct {
	rows []ops.Row
	err  error
}

// planMsg carries the dry-run plan of an action.
type planMsg struct {
	act  action
	plan []ops.Action
	err  error
}

// appliedMsg carries the result of an applied action.
type appliedMsg struct {
	done []ops.Action
	err  error
}

// diffMsg carries the file changes of one installed skill.
type diffMsg struct {
	skill   string
	agent   config.Agent
	changes []ops.FileChange
	err     error
}

func (m Model) Init() tea.Cmd {
	return m.loadCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.diff.Width = msg.Width
		m.diff.Height = max(1, msg.Height-diffChrome)
		m.scroll()
	case loadedMsg:
		if msg.err != nil {
			m.setResult(msg.err.Error(), true)
			return m, nil
		}
		m.setRows(msg.rows)
	case planMsg:
		return m.onPlan(msg)
	case appliedMsg:
		return m.onApplied(msg)
	case diffMsg:
		return m.onDiff(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.forceQuit) {
		return m, tea.Quit
	}
	switch m.mode {
	case modeFilter:
		return m.filterKey(msg)
	case modeConfirm:
		return m.confirmKey(msg)
	case modeDiff:
		return m.diffKey(msg)
	case modeHelp:
		return m.helpKey(msg)
	}
	return m.matrixKey(msg)
}

func (m Model) matrixKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.up):
		m.row = clampIndex(m.row-1, len(m.skills))
		m.scroll()
	case key.Matches(msg, k.down):
		m.row = clampIndex(m.row+1, len(m.skills))
		m.scroll()
	case key.Matches(msg, k.left):
		m.col = clampIndex(m.col-1, len(m.agents))
	case key.Matches(msg, k.right):
		m.col = clampIndex(m.col+1, len(m.agents))
	case key.Matches(msg, k.install):
		return m.startAction(ops.OpInstall)
	case key.Matches(msg, k.uninstall):
		return m.startAction(ops.OpUninstall)
	case key.Matches(msg, k.update):
		return m.startAction(ops.OpUpdate)
	case key.Matches(msg, k.adopt):
		return m.startAction(ops.OpAdopt)
	case key.Matches(msg, k.diff):
		skill, agent, _, ok := m.selected()
		if !ok {
			m.setResult(nothingSelected, true)
			return m, nil
		}
		return m, diffCmd(m.backend, skill, agent)
	case key.Matches(msg, k.refresh):
		return m, m.loadCmd()
	case key.Matches(msg, k.filter):
		m.mode = modeFilter
		m.filter.SetValue(m.query)
		return m, m.filter.Focus()
	case key.Matches(msg, k.esc):
		if m.query != "" {
			m.filter.SetValue("")
			m.setQuery("")
		}
	case key.Matches(msg, k.help):
		m.mode = modeHelp
	case key.Matches(msg, k.quit):
		return m, tea.Quit
	}
	return m, nil
}

// startAction plans op on the selected cell. The dry run never forces, so the
// user sees what ops would do, or what it refuses, before anything changes.
func (m Model) startAction(op string) (Model, tea.Cmd) {
	skill, agent, _, ok := m.selected()
	if !ok {
		m.setResult(nothingSelected, true)
		return m, nil
	}
	return m, planCmd(m.backend, action{op: op, skill: skill, agent: agent})
}

// onPlan decides what a dry run leads to. A refusal that --force overrides
// opens the force prompt. An uninstall opens the plain confirm. Anything else
// applies at once.
func (m Model) onPlan(msg planMsg) (Model, tea.Cmd) {
	if reason, ok := forceReason(msg.plan, msg.err); ok {
		act := msg.act
		act.force = true
		m.pending = pending{act: act, reason: reason}
		m.mode = modeConfirm
		return m, nil
	}
	if msg.err != nil {
		m.setResult(msg.err.Error(), true)
		return m, nil
	}
	if !hasWork(msg.plan) {
		m.setResult(summarize(msg.plan), false)
		return m, nil
	}
	if msg.act.op == ops.OpUninstall {
		m.pending = pending{act: msg.act, plan: msg.plan}
		m.mode = modeConfirm
		return m, nil
	}
	return m, applyCmd(m.backend, msg.act)
}

// forceReason reports whether an unforced dry run was refused for want of
// --force, with the reason. ops refuses with ErrNeedsForce, or an update plans
// to skip a modified copy.
func forceReason(plan []ops.Action, err error) (string, bool) {
	if errors.Is(err, ops.ErrNeedsForce) {
		return err.Error(), true
	}
	for _, a := range plan {
		if a.Op == ops.OpSkip && a.Reason == ops.ReasonModified {
			return describe(a), true
		}
	}
	return "", false
}

// confirmKey applies the pending action on y. Any other key cancels it, so a
// stray key never changes anything.
func (m Model) confirmKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	act := m.pending.act
	m.pending = pending{}
	m.mode = modeMatrix
	if key.Matches(msg, m.keys.yes) {
		return m, applyCmd(m.backend, act)
	}
	m.setResult("cancelled", false)
	return m, nil
}

func (m Model) onApplied(msg appliedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.setResult(msg.err.Error(), true)
	} else {
		m.setResult(summarize(msg.done), false)
	}
	return m, m.loadCmd()
}

func (m Model) onDiff(msg diffMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.setResult(msg.err.Error(), true)
		return m, nil
	}
	m.diffTitle = fmt.Sprintf("diff %s in %s", msg.skill, msg.agent.ID)
	m.diff.SetContent(m.renderChanges(msg.changes))
	m.diff.GotoTop()
	m.mode = modeDiff
	return m, nil
}

func (m Model) diffKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.closeView) {
		m.mode = modeMatrix
		return m, nil
	}
	var cmd tea.Cmd
	m.diff, cmd = m.diff.Update(msg)
	return m, cmd
}

func (m Model) filterKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.enter):
		m.mode = modeMatrix
		m.filter.Blur()
		return m, nil
	case key.Matches(msg, m.keys.esc):
		m.mode = modeMatrix
		m.filter.Blur()
		m.filter.SetValue("")
		m.setQuery("")
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.setQuery(m.filter.Value())
	return m, cmd
}

func (m Model) helpKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.closeView) || key.Matches(msg, m.keys.help) {
		m.mode = modeMatrix
	}
	return m, nil
}

// loadCmd reads the status of every skill in every agent.
func (m Model) loadCmd() tea.Cmd {
	b, agents := m.backend, m.agents
	return func() tea.Msg {
		rows, err := b.Status(agents)
		return loadedMsg{rows: rows, err: err}
	}
}

// planCmd runs act as an unforced dry run.
func planCmd(b Backend, act action) tea.Cmd {
	return func() tea.Msg {
		plan, err := perform(b, act, true)
		return planMsg{act: act, plan: plan, err: err}
	}
}

// applyCmd runs act for real.
func applyCmd(b Backend, act action) tea.Cmd {
	return func() tea.Msg {
		done, err := perform(b, act, false)
		return appliedMsg{done: done, err: err}
	}
}

func diffCmd(b Backend, skill string, agent config.Agent) tea.Cmd {
	return func() tea.Msg {
		changes, err := b.Diff(agent, skill)
		return diffMsg{skill: skill, agent: agent, changes: changes, err: err}
	}
}

// perform calls the Backend for act. A dry run changes nothing.
func perform(b Backend, act action, dryRun bool) ([]ops.Action, error) {
	opts := ops.Options{Force: act.force, DryRun: dryRun}
	agents := []config.Agent{act.agent}
	skills := []string{act.skill}
	switch act.op {
	case ops.OpInstall:
		return b.Install(agents, skills, opts)
	case ops.OpUninstall:
		return b.Uninstall(agents, skills, opts)
	case ops.OpUpdate:
		return b.Update(agents, skills, opts)
	case ops.OpAdopt:
		a, err := b.Adopt(act.agent, act.skill, opts)
		if err != nil {
			return nil, err
		}
		return []ops.Action{a}, nil
	}
	return nil, fmt.Errorf("unknown operation %q", act.op)
}

// hasWork reports whether a plan changes anything, as opposed to only skipping.
func hasWork(plan []ops.Action) bool {
	for _, a := range plan {
		if a.Op != ops.OpSkip {
			return true
		}
	}
	return false
}

// summarize describes what a plan or an applied action did, for the status line.
func summarize(acts []ops.Action) string {
	if len(acts) == 0 {
		return "nothing to do"
	}
	parts := make([]string, 0, len(acts))
	for _, a := range acts {
		if a.Op == ops.OpSkip {
			parts = append(parts, fmt.Sprintf("skipped %s in %s: %s", a.Skill, a.Agent, a.Reason))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s in %s", a.Op, a.Skill, a.Agent))
	}
	return strings.Join(parts, "; ")
}

// selected returns the cell under the cursor. ok is false when there is no
// skill or no agent to point at.
func (m Model) selected() (skill string, agent config.Agent, state ops.State, ok bool) {
	if m.row >= len(m.skills) || m.col >= len(m.agents) {
		return "", config.Agent{}, "", false
	}
	skill = m.skills[m.row]
	agent = m.agents[m.col]
	return skill, agent, m.cells[cell{skill, agent.ID}], true
}

func (m *Model) setResult(text string, isErr bool) {
	m.result = text
	m.resultErr = isErr
}

// setRows replaces the statuses after a load. The cursor stays on its skill
// when that skill is still listed.
func (m *Model) setRows(rows []ops.Row) {
	m.cells = make(map[cell]ops.State, len(rows))
	names := make(map[string]bool)
	for _, r := range rows {
		m.cells[cell{r.Skill, r.Agent}] = r.Status
		names[r.Skill] = true
	}
	m.names = slices.Sorted(maps.Keys(names))
	m.rebuild()
}

func (m *Model) setQuery(q string) {
	m.query = q
	m.rebuild()
}

// rebuild applies the query to the skill names.
func (m *Model) rebuild() {
	prev, _, _, _ := m.selected()
	q := strings.ToLower(m.query)
	var shown []string
	for _, name := range m.names {
		if strings.Contains(strings.ToLower(name), q) {
			shown = append(shown, name)
		}
	}
	m.skills = shown
	if i := slices.Index(m.skills, prev); i >= 0 {
		m.row = i
	}
	m.row = clampIndex(m.row, len(m.skills))
	m.scroll()
}

// visibleRows is how many skill rows fit beside the chrome. Before the first
// size message it is every row.
func (m Model) visibleRows() int {
	if m.height == 0 {
		return max(1, len(m.skills))
	}
	return max(1, m.height-matrixChrome)
}

// scroll moves the window of rows so the cursor row is shown.
func (m *Model) scroll() {
	vis := m.visibleRows()
	if m.row < m.top {
		m.top = m.row
	}
	if m.row >= m.top+vis {
		m.top = m.row - vis + 1
	}
}

// clampIndex keeps i in [0, n-1], or at 0 when n is 0.
func clampIndex(i, n int) int {
	return max(0, min(i, n-1))
}
