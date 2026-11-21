// Package tui is sapato's interactive UI. It calls the ops package through a
// Backend and holds no business logic of its own.
package tui

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/target"
	"sapato/internal/tui/icons"
	"sapato/internal/tui/theme"
	"sapato/internal/vault"
)

// screen is the page the model shows. An overlay mode may cover it.
type screen int

const (
	screenMain screen = iota
	screenSkill
)

// mode is an overlay on the screen. Keys go to the handler of the mode, or to
// the screen when the mode is modeNormal.
type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeConfirm
	modeDiff
	modeHelp
	modeAdopt
	modeNew
	modeImport
	modeExport
	modeTheme
	modeLog
)

const nothingSelected = "nothing selected"

// cell is one skill in one agent.
type cell struct {
	skill string
	agent string
}

// action is one mutation, as the UI runs it through the Backend. The agents are
// the targets of a skill action. force is set only once the user has accepted a
// force prompt. back is the mode a confirm of the action returns to. content,
// src and path are the inputs of a create, an import and an export.
type action struct {
	op      string
	skill   string
	agents  []config.Agent
	force   bool
	back    mode
	content []byte
	src     string
	path    string
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
	icons   icons.Set

	cells       map[cell]ops.State     // status of each skill in each agent; absent means blank
	names       []string               // every skill name from the last load, sorted
	vaultSkills map[string]vault.Skill // vault skills of the last load, by name
	skills      []string               // names that match query: the grid rows
	query       string
	row         int // cursor, as an index into skills
	top         int // first row shown, for scrolling

	screen    screen
	mode      mode
	open      string          // the skill the skill screen shows
	cur       int             // cursor over the agents of the skill screen
	agentTop  int             // first sidebar row shown on the skill screen, for scrolling
	detailTop int             // first line of the details pane shown, for scrolling
	marks     map[string]bool // agent IDs marked on the skill screen
	detail    detail          // what the skill screen reads for the cursor's agent

	filter    textinput.Model
	pending   pending
	noVault   string // the vault the vault prompt offers to create; "" once a vault is open
	diff      viewport.Model
	diffTitle string

	th        theme.Theme     // the palette the styles paint with
	themePrev theme.Theme     // the palette in use when THEME opened, restored on esc
	themeCur  int             // cursor over theme.All while THEME is open
	themeTop  int             // first theme row shown
	pick      int             // cursor over the ADOPT rows
	pickTop   int             // first ADOPT row shown
	area      textarea.Model  // the NEW editor
	path      textinput.Model // the IMPORT and EXPORT path

	notes   []notification // every notification of the session, oldest first, at most maxNotes
	pushed  int            // notifications pushed so far, LOG drops the oldest past maxNotes
	logBack mode           // the mode LOG closes back to
	logTop  int            // first LOG entry shown

	// now is the clock of the notifications. tick schedules the repaint when one
	// runs out. Tests replace both.
	now  func() time.Time
	tick func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd

	width, height int
}

// New returns the UI for the agents as grid columns, painted with th and drawn
// with the glyphs of set. The caller decides which agents are shown; New does
// not filter them.
func New(b Backend, agents []config.Agent, th theme.Theme, set icons.Set, noColor bool) Model {
	filter := textinput.New()
	filter.Prompt = "/ "
	// A static cursor keeps the model free of blink commands.
	filter.Cursor.SetMode(cursor.CursorStatic)
	return Model{
		backend:     b,
		agents:      agents,
		keys:        newKeyMap(),
		styles:      newStyles(th, noColor),
		icons:       set,
		cells:       map[cell]ops.State{},
		vaultSkills: map[string]vault.Skill{},
		marks:       map[string]bool{},
		filter:      filter,
		diff:        viewport.New(0, 0),
		th:          th,
		area:        newEditor(),
		path:        newPathInput(),
		now:         time.Now,
		tick:        tea.Tick,
	}
}

// loadedMsg carries the result of a status load.
type loadedMsg struct {
	rows    []ops.Row
	skills  []vault.Skill
	refresh bool // the load was asked for by r, so it notifies what it loaded
	err     error
}

// planMsg carries the dry-run plan of an action.
type planMsg struct {
	act  action
	plan []ops.Action
	err  error
}

// appliedMsg carries the result of an applied action.
type appliedMsg struct {
	act  action
	done []ops.Action
	err  error
}

// detail is what the skill screen reads for the agent under its cursor: the files of
// the vault copy, or of the installed copy when the vault lacks the skill, and the
// agent's lock entry. It is loaded for one skill and agent at a time.
type detail struct {
	skill     string // the skill it was loaded for; "" until one is loaded
	agent     string // the agent's ID, "" when none is configured
	files     []ops.File
	installed string // the agent whose installed copy the files are, "" for the vault copy
	entry     target.LockEntry
	managed   bool // the lock has an entry for the agent
}

// detailMsg carries a loaded detail of the skill screen.
type detailMsg struct {
	detail detail
	err    error
}

// diffMsg carries the file changes of one installed skill.
type diffMsg struct {
	skill   string
	agent   config.Agent
	changes []ops.FileChange
	err     error
}

func (m Model) Init() tea.Cmd {
	if m.backend == nil {
		return nil
	}
	return m.loadCmd(false)
}

// Update is the transition for msg. A notification that msg pushes gets a repaint
// when its time runs out.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.pushed
	next, cmd := m.handle(msg)
	return next, tea.Batch(cmd, next.expiries(before))
}

// handle is the transition for msg, without the expiry repaints.
func (m Model) handle(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.diff.Width = msg.Width
		m.diff.Height = max(1, m.bodyHeight()-1)
		m.sizeInputs()
		m.scroll()
		m.scrollAgents()
	case exportNameMsg:
		return m.onExportName(msg)
	case loadedMsg:
		if msg.err != nil {
			m.notify(noteError, "error", msg.err.Error())
			return m, nil
		}
		m.load(msg.rows, msg.skills)
		if msg.refresh {
			m.notify(noteInfo, "refreshed", fmt.Sprintf("%d skills", len(m.names)))
		}
		return m, m.detailCmd()
	case detailMsg:
		return m.onDetail(msg), nil
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
	if key.Matches(msg, m.keys.log) && logOpensFrom(m.mode) {
		return m.openLog(), nil
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
	case modeAdopt:
		return m.adoptKey(msg)
	case modeNew:
		return m.newKey(msg)
	case modeImport, modeExport:
		return m.pathKey(msg)
	case modeTheme:
		return m.themeKey(msg)
	case modeLog:
		return m.logKey(msg)
	}
	if m.screen == screenSkill {
		return m.skillKey(msg)
	}
	return m.mainKey(msg)
}

func (m Model) mainKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.up):
		m.row = clampIndex(m.row-1, len(m.skills))
		m.scroll()
	case key.Matches(msg, k.down):
		m.row = clampIndex(m.row+1, len(m.skills))
		m.scroll()
	case key.Matches(msg, k.top):
		m.row = 0
		m.scroll()
	case key.Matches(msg, k.bottom):
		m.row = clampIndex(len(m.skills)-1, len(m.skills))
		m.scroll()
	case key.Matches(msg, k.pageUp):
		m.row = clampIndex(m.row-m.visibleRows(), len(m.skills))
		m.scroll()
	case key.Matches(msg, k.pageDown):
		m.row = clampIndex(m.row+m.visibleRows(), len(m.skills))
		m.scroll()
	case key.Matches(msg, k.enter):
		return m.openSkill()
	case key.Matches(msg, k.filter):
		m.mode = modeFilter
		m.filter.SetValue(m.query)
		return m, m.filter.Focus()
	case key.Matches(msg, k.esc):
		if m.query != "" {
			m.filter.SetValue("")
			m.setQuery("")
		}
	case key.Matches(msg, k.refresh):
		return m, m.loadCmd(true)
	case key.Matches(msg, k.adopt):
		return m.openAdopt(), nil
	case key.Matches(msg, k.newSkill):
		return m.openNew()
	case key.Matches(msg, k.importTar):
		return m.openImport()
	case key.Matches(msg, k.theme):
		return m.openTheme(), nil
	case key.Matches(msg, k.help):
		m.mode = modeHelp
	case key.Matches(msg, k.quit):
		return m, tea.Quit
	}
	return m, nil
}

// openSkill shows the skill under the grid cursor on the skill screen, and loads
// its detail for the first agent.
func (m Model) openSkill() (Model, tea.Cmd) {
	if m.row >= len(m.skills) {
		m.notify(noteError, "error", nothingSelected)
		return m, nil
	}
	m.screen = screenSkill
	m.open = m.skills[m.row]
	m.cur = 0
	m.agentTop = 0
	m.detailTop = 0
	m.marks = map[string]bool{}
	m.detail = detail{}
	return m, m.detailCmd()
}

func (m Model) skillKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.up):
		m.cur = clampIndex(m.cur-1, len(m.agents))
		m.scrollAgents()
		return m, m.detailCmd()
	case key.Matches(msg, k.down):
		m.cur = clampIndex(m.cur+1, len(m.agents))
		m.scrollAgents()
		return m, m.detailCmd()
	case key.Matches(msg, k.pageUp):
		m.scrollDetails(-m.detailRows())
	case key.Matches(msg, k.pageDown):
		m.scrollDetails(m.detailRows())
	case key.Matches(msg, k.mark):
		m.toggleMark()
	case key.Matches(msg, k.install):
		return m.startAction(ops.OpInstall, m.targets())
	case key.Matches(msg, k.uninstall):
		return m.startAction(ops.OpUninstall, m.targets())
	case key.Matches(msg, k.update):
		return m.startAction(ops.OpUpdate, m.targets())
	case key.Matches(msg, k.installAll):
		return m.startAction(ops.OpInstall, m.agents)
	case key.Matches(msg, k.adopt):
		return m.adopt()
	case key.Matches(msg, k.diff):
		return m.openDiff()
	case key.Matches(msg, k.export):
		return m.openExport()
	case key.Matches(msg, k.remove):
		return m.startRemove()
	case key.Matches(msg, k.esc):
		m.screen = screenMain
		m.marks = map[string]bool{}
	case key.Matches(msg, k.help):
		m.mode = modeHelp
	case key.Matches(msg, k.quit):
		return m, tea.Quit
	}
	return m, nil
}

// cursorID is the ID of the agent under the skill screen's cursor, or "" when none
// is configured.
func (m Model) cursorID() string {
	if m.cur < len(m.agents) {
		return m.agents[m.cur].ID
	}
	return ""
}

// detailCmd loads the detail of the open skill for the agent under the cursor. The
// files are those of the vault copy, or of the installed copy when the vault lacks
// the skill and the agent holds one. It is nil off the skill screen.
func (m Model) detailCmd() tea.Cmd {
	if m.screen != screenSkill {
		return nil
	}
	d := detail{skill: m.open, agent: m.cursorID()}
	var agentPath string
	if m.cur < len(m.agents) {
		agentPath = m.agents[m.cur].Path
	}
	var folder string
	if s, ok := m.vaultSkills[m.open]; ok {
		folder = s.Dir
	} else if d.agent != "" && hasFolder(m.cells[cell{m.open, d.agent}]) {
		folder, d.installed = filepath.Join(agentPath, m.open), d.agent
	}
	b, skill := m.backend, m.open
	return func() tea.Msg {
		// Both reads run even when one fails, so the other still shows. A failed read
		// leaves its part empty.
		out := d
		var filesErr, lockErr error
		if folder != "" {
			out.files, filesErr = b.Files(folder)
		}
		if out.agent != "" {
			out.entry, out.managed, lockErr = b.LockEntry(agentPath, skill)
		}
		return detailMsg{detail: out, err: cmp.Or(filesErr, lockErr)}
	}
}

// onDetail keeps a loaded detail when it is for the skill and agent the screen shows
// now, even when a read of it failed, and notifies the failure. A late one, for
// another skill or agent, is dropped.
func (m Model) onDetail(msg detailMsg) Model {
	if msg.err != nil {
		m.notify(noteError, "error", msg.err.Error())
	}
	if m.screen == screenSkill && msg.detail.skill == m.open && msg.detail.agent == m.cursorID() {
		m.detail = msg.detail
	}
	return m
}

// toggleMark marks or unmarks the agent under the cursor.
func (m *Model) toggleMark() {
	if m.cur >= len(m.agents) {
		return
	}
	id := m.agents[m.cur].ID
	if m.marks[id] {
		delete(m.marks, id)
		return
	}
	m.marks[id] = true
}

// targets are the agents an action on the skill screen applies to: the marked
// agents in agent order, or the cursor's agent when none is marked.
func (m Model) targets() []config.Agent {
	var out []config.Agent
	for _, a := range m.agents {
		if m.marks[a.ID] {
			out = append(out, a)
		}
	}
	if len(out) == 0 && m.cur < len(m.agents) {
		out = append(out, m.agents[m.cur])
	}
	return out
}

// startAction plans op on the open skill in agents. The dry run never forces, so
// the user sees what ops would do, or what it refuses, before anything changes.
// Install and update copy from the vault, so they wait on a skill the vault lacks.
func (m Model) startAction(op string, agents []config.Agent) (Model, tea.Cmd) {
	if len(agents) == 0 {
		m.notify(noteError, "error", "no agents configured")
		return m, nil
	}
	if op != ops.OpUninstall && !m.inVault(m.open) {
		m.notify(noteError, "error", m.open+" is not in the vault")
		return m, nil
	}
	return m, planCmd(m.backend, action{op: op, skill: m.open, agents: agents})
}

// adopt takes the first target whose copy is unmanaged. Adopt copies one folder
// into the vault, so the others wait for their own adopt.
func (m Model) adopt() (Model, tea.Cmd) {
	for _, a := range m.targets() {
		if m.cells[cell{m.open, a.ID}] == ops.Unmanaged {
			return m, planCmd(m.backend, action{op: ops.OpAdopt, skill: m.open, agents: []config.Agent{a}})
		}
	}
	m.notify(noteError, "error", "adopt needs an unmanaged copy")
	return m, nil
}

// diff opens the pager on the first target. The diff compares against the vault,
// so it waits on a skill the vault lacks.
func (m Model) openDiff() (Model, tea.Cmd) {
	targets := m.targets()
	if len(targets) == 0 {
		m.notify(noteError, "error", "no agents configured")
		return m, nil
	}
	if !m.inVault(m.open) {
		m.notify(noteError, "error", m.open+" is not in the vault")
		return m, nil
	}
	return m, diffCmd(m.backend, m.open, targets[0])
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
		m.notify(noteError, "error", msg.err.Error())
		return m, nil
	}
	if !hasWork(msg.plan) {
		m.notify(noteInfo, "no change", summarize(msg.plan))
		return m, nil
	}
	if msg.act.op == ops.OpUninstall || msg.act.op == ops.OpRemove {
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
// stray key never changes anything. Either way the prompt returns to the mode that
// asked it.
func (m Model) confirmKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	act := m.pending.act
	if act.op == opVaultInit {
		return m.vaultKey(msg)
	}
	m.pending = pending{}
	m.mode = act.back
	if act.op == opConfigInit {
		return m.configKey(msg, act), nil
	}
	if !key.Matches(msg, m.keys.yes) {
		m.notify(noteInfo, "cancelled", subject(act))
		return m, nil
	}
	if act.op == opDiscard {
		m.mode = modeNormal
		m.notify(noteInfo, "discarded", "new skill discarded")
		return m, nil
	}
	return m, applyCmd(m.backend, act)
}

// onApplied shows the result of an applied action. A dialog closes once its
// action is done. The adopt picker stays open, so the copies left can be adopted.
func (m Model) onApplied(msg appliedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.notify(noteError, "error", msg.err.Error())
		return m, m.loadCmd(false)
	}
	m.notify(noteSuccess, doneTitle(msg.act.op), summarize(msg.done))
	if msg.act.op != ops.OpAdopt {
		m.mode = modeNormal
	}
	return m, m.loadCmd(false)
}

func (m Model) onDiff(msg diffMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.notify(noteError, "error", msg.err.Error())
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
		m.mode = modeNormal
		return m, nil
	}
	var cmd tea.Cmd
	m.diff, cmd = m.diff.Update(msg)
	return m, cmd
}

func (m Model) filterKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.enter):
		m.mode = modeNormal
		m.filter.Blur()
		return m, nil
	case key.Matches(msg, m.keys.esc):
		m.mode = modeNormal
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
		m.mode = modeNormal
	}
	return m, nil
}

// loadCmd reads the status of every skill in every agent, and the vault skills.
// refresh marks a load that r asked for.
func (m Model) loadCmd(refresh bool) tea.Cmd {
	b, agents := m.backend, m.agents
	return func() tea.Msg {
		rows, err := b.Status(agents)
		if err != nil {
			return loadedMsg{err: err, refresh: refresh}
		}
		skills, err := b.Skills()
		return loadedMsg{rows: rows, skills: skills, refresh: refresh, err: err}
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
		return appliedMsg{act: act, done: done, err: err}
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
	skills := []string{act.skill}
	switch act.op {
	case ops.OpInstall:
		return b.Install(act.agents, skills, opts)
	case ops.OpUninstall:
		return b.Uninstall(act.agents, skills, opts)
	case ops.OpUpdate:
		return b.Update(act.agents, skills, opts)
	case ops.OpAdopt:
		return one(b.Adopt(act.agents[0], act.skill, opts))
	case ops.OpRemove:
		return one(b.Remove(act.skill, opts))
	case ops.OpCreate:
		return one(b.Create(act.content, opts))
	case ops.OpImport:
		return b.Import(act.src, opts)
	case opExport:
		skips, err := b.Export(act.skill, act.path)
		if err != nil {
			return nil, err
		}
		return append([]ops.Action{{Op: opExport, Skill: act.skill, Path: act.path}}, skips...), nil
	}
	return nil, fmt.Errorf("unknown operation %q", act.op)
}

// one is the plan of a call that acts on a single skill or folder.
func one(a ops.Action, err error) ([]ops.Action, error) {
	if err != nil {
		return nil, err
	}
	return []ops.Action{a}, nil
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

// summarize describes what a plan or an applied action did, for the bar.
func summarize(acts []ops.Action) string {
	if len(acts) == 0 {
		return "nothing to do"
	}
	parts := make([]string, 0, len(acts))
	for _, a := range acts {
		switch {
		case a.Op == ops.OpSkip:
			parts = append(parts, fmt.Sprintf("skipped %s in %s: %s", a.Skill, a.Agent, a.Reason))
		case a.Op == opExport:
			parts = append(parts, "wrote "+a.Path)
		case a.Agent == "":
			parts = append(parts, fmt.Sprintf("%s %s", a.Op, a.Skill))
		default:
			parts = append(parts, fmt.Sprintf("%s %s in %s", a.Op, a.Skill, a.Agent))
		}
	}
	return strings.Join(parts, "; ")
}

// load replaces the statuses and vault skills after a load. The grid cursor
// stays on its skill when that skill is still listed. The skill screen goes back
// to the grid when its skill is gone.
func (m *Model) load(rows []ops.Row, skills []vault.Skill) {
	m.cells = make(map[cell]ops.State, len(rows))
	names := make(map[string]bool)
	for _, r := range rows {
		m.cells[cell{r.Skill, r.Agent}] = r.Status
		names[r.Skill] = true
	}
	m.vaultSkills = make(map[string]vault.Skill, len(skills))
	for _, s := range skills {
		m.vaultSkills[s.Name] = s
	}
	m.names = slices.Sorted(maps.Keys(names))
	m.rebuild()
	m.pick = clampIndex(m.pick, len(m.unmanaged()))
	m.scrollPick()
	if m.screen == screenSkill && !slices.Contains(m.names, m.open) {
		m.screen = screenMain
	}
	m.cur = clampIndex(m.cur, len(m.agents))
	m.scrollAgents()
}

// inVault reports whether the vault holds a skill called name, valid or not.
func (m Model) inVault(name string) bool {
	_, ok := m.vaultSkills[name]
	return ok
}

func (m *Model) setQuery(q string) {
	m.query = q
	m.rebuild()
}

// rebuild applies the query to the skill names.
func (m *Model) rebuild() {
	prev := ""
	if m.row < len(m.skills) {
		prev = m.skills[m.row]
	}
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

// visibleRows is how many grid rows fit in the body beside the column header and
// its rule. Before the first size message it is every row.
func (m Model) visibleRows() int {
	if m.height == 0 {
		return max(1, len(m.skills))
	}
	return max(1, m.bodyHeight()-2)
}

// agentRowsShown is how many sidebar rows fit, at least one.
func (m Model) agentRowsShown() int {
	return max(1, m.skillLayout().side.h-2)
}

// detailRows is how many lines of the details pane show, at least one.
func (m Model) detailRows() int {
	return max(1, m.skillLayout().detail.h-2)
}

// scrollDetails moves the details pane by delta lines, within its content.
func (m *Model) scrollDetails(delta int) {
	lay := m.skillLayout()
	lines := m.detailLines(lay.detail.w - 4)
	m.detailTop = max(0, min(m.detailTop+delta, len(lines)-max(0, lay.detail.h-2)))
}

// scroll moves the window of rows so the cursor row is shown.
func (m *Model) scroll() {
	scrollWindow(&m.top, m.row, m.visibleRows())
}

// scrollAgents moves the window of agent rows so the cursor agent is shown.
func (m *Model) scrollAgents() {
	scrollWindow(&m.agentTop, m.cur, m.agentRowsShown())
}

// scrollWindow moves the first row of a window of vis rows, so that cursor is in it.
func scrollWindow(top *int, cursor, vis int) {
	if cursor < *top {
		*top = cursor
	}
	if cursor >= *top+vis {
		*top = cursor - vis + 1
	}
}

// clampIndex keeps i in [0, n-1], or at 0 when n is 0.
func clampIndex(i, n int) int {
	return max(0, min(i, n-1))
}
