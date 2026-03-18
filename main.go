package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const dataFileName = "planner.json"

const (
	workDuration       = 25 * time.Minute
	shortBreakDuration = 5 * time.Minute
	longBreakDuration  = 30 * time.Minute
	longBreakEveryWork = 2 * time.Hour
)

type milestone struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Order     int    `json:"order"`
}

type goal struct {
	ID           int    `json:"id"`
	MilestoneID  int    `json:"milestone_id"`
	ParentGoalID int    `json:"parent_goal_id"`
	Name         string `json:"name"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Order        int    `json:"order"`
	Important    bool   `json:"important"`
	Urgent       bool   `json:"urgent"`
}

type todo struct {
	ID          int    `json:"id"`
	GoalID      int    `json:"goal_id"`
	Name        string `json:"name"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Order       int    `json:"order"`
	GlobalOrder int    `json:"global_order"`
	Important   bool   `json:"important"`
	Urgent      bool   `json:"urgent"`
}

type plannerData struct {
	NextID     int         `json:"next_id"`
	Milestones []milestone `json:"milestones"`
	Goals      []goal      `json:"goals"`
	Todos      []todo      `json:"todos"`
}

type pane int

const (
	paneSidebar pane = iota
	paneList
)

type screenKind int

const (
	screenInbox screenKind = iota
	screenAll
	screenMilestone
	screenGoal
)

type screenState struct {
	kind        screenKind
	milestoneID int
	goalID      int
}

type itemKind int

const (
	itemGoal itemKind = iota
	itemTodo
)

type focusItem struct {
	kind  itemKind
	id    int
	order int
}

type formMode int

const (
	formNone formMode = iota
	formQuickAdd
	formAddMilestone
	formAddGoal
	formEditMilestone
	formEditGoal
	formEditTodo
)

type formState struct {
	mode   formMode
	target int
	inputs []textinput.Model
	index  int
}

type searchMode int

const (
	searchNone searchMode = iota
	searchJump
	searchMove
)

type searchState struct {
	active bool
	mode   searchMode
	input  textinput.Model
	index  int
	item   focusItem
}

type searchResult struct {
	kind        string
	id          int
	milestoneID int
	goalID      int
	label       string
	query       string
}

type sidebarEntry struct {
	label       string
	meta        string
	screen      screenState
	milestoneID int
}

type pomodoroPhase int

const (
	phaseWork pomodoroPhase = iota
	phaseShortBreak
	phaseLongBreak
)

type tickMsg time.Time

type pomodoroState struct {
	phase           pomodoroPhase
	running         bool
	remaining       time.Duration
	workAccumulated time.Duration
}

type grabState struct {
	active bool
	item   focusItem
}

type model struct {
	data       plannerData
	dataPath   string
	width      int
	height     int
	activePane pane

	screen     screenState
	screenBack []screenState

	sidebarIdx int
	listIdx    int

	form   formState
	search searchState
	grab   grabState
	timer  pomodoroState

	status   string
	showHelp bool
	quitting bool
}

var (
	bodyColor      = lipgloss.AdaptiveColor{Light: "0", Dark: "255"}
	mutedColor     = lipgloss.AdaptiveColor{Light: "8", Dark: "250"}
	borderColor    = lipgloss.AdaptiveColor{Light: "246", Dark: "244"}
	accentColor    = lipgloss.AdaptiveColor{Light: "25", Dark: "45"}
	accentBg       = lipgloss.AdaptiveColor{Light: "25", Dark: "31"}
	accentFg       = lipgloss.AdaptiveColor{Light: "255", Dark: "255"}
	successColor   = lipgloss.AdaptiveColor{Light: "28", Dark: "78"}
	warnColor      = lipgloss.AdaptiveColor{Light: "166", Dark: "214"}
	appStyle       = lipgloss.NewStyle().Padding(1, 2).Foreground(bodyColor)
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(bodyColor)
	mutedStyle     = lipgloss.NewStyle().Foreground(mutedColor)
	highlightStyle = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	activeBadgeStyle = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true).Padding(0, 1)
	panelStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Foreground(bodyColor).Padding(1)
	activeRowStyle = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true)
	inactiveRowStyle = lipgloss.NewStyle().Foreground(bodyColor).BorderLeft(true).BorderForeground(accentColor).PaddingLeft(1)
	formStyle      = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentBg).Foreground(bodyColor).Padding(1)
	successStyle   = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	warnStyle      = lipgloss.NewStyle().Foreground(warnColor).Bold(true)
)

func panelFrame(active bool) lipgloss.Style {
	style := panelStyle
	if active {
		return style.BorderForeground(accentColor)
	}
	return style.BorderForeground(borderColor)
}

func panelHeading(label string, active bool) string {
	if active {
		return fmt.Sprintf("%s %s", headerStyle.Render(label), activeBadgeStyle.Render("ACTIVE"))
	}
	return headerStyle.Render(label)
}

func main() {
	dataPath, err := filepath.Abs(dataFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data file: %v\n", err)
		os.Exit(1)
	}

	data, err := loadData(dataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load planner data: %v\n", err)
		os.Exit(1)
	}

	m := model{
		data:       data,
		dataPath:   dataPath,
		activePane: paneList,
		screen:     screenState{kind: screenInbox},
		timer: pomodoroState{
			phase:     phaseWork,
			remaining: workDuration,
		},
		status: "n quick add • / jump/create • m move • v grab • enter open/drop • i inbox • tab switch panes • ? help",
	}
	m.search.input = textinput.New()
	m.search.input.Width = 42
	m.search.input.Placeholder = "Search tasks, goals, milestones"
	m.normalize()

	program := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run planner: %v\n", err)
		os.Exit(1)
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		return m.updateTimerTick()
	case tea.KeyMsg:
		if m.search.active {
			return m.updateSearch(msg)
		}
		if m.form.mode != formNone {
			return m.updateForm(msg)
		}
		return m.updateBrowse(msg)
	}
	return m, nil
}

func (m model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "tab", "ctrl+w":
		if m.activePane == paneSidebar {
			m.activePane = paneList
		} else {
			m.activePane = paneSidebar
		}
		return m, nil
	case "i":
		m.setScreen(screenState{kind: screenInbox}, false)
		m.status = successStyle.Render("Inbox ready.")
		return m, nil
	case "A":
		m.setScreen(screenState{kind: screenAll}, false)
		m.status = successStyle.Render("All tasks ready.")
		return m, nil
	case "/":
		m.startSearch(searchJump, focusItem{})
		return m, textinput.Blink
	case "m":
		item, ok := m.selectedItem()
		if !ok {
			m.status = "Select a goal or todo to move."
			return m, nil
		}
		m.startSearch(searchMove, item)
		return m, textinput.Blink
	case "n", "a":
		m.startForm(formQuickAdd, m.quickAddGoalID())
		return m, textinput.Blink
	case "M":
		m.startForm(formAddMilestone, 0)
		return m, textinput.Blink
	case "g":
		if m.activePane == paneSidebar {
			m.sidebarIdx = 0
			m.applySidebarSelection()
		} else {
			m.listIdx = 0
		}
		return m, nil
	case "G":
		if m.activePane == paneSidebar {
			entries := m.sidebarEntries()
			if len(entries) > 0 {
				m.sidebarIdx = len(entries) - 1
				m.applySidebarSelection()
			}
		} else {
			items := m.visibleItems()
			if len(items) > 0 {
				m.listIdx = len(items) - 1
			}
		}
		return m, nil
	case "up", "k":
		if m.activePane == paneSidebar {
			if m.sidebarIdx > 0 {
				m.sidebarIdx--
				m.applySidebarSelection()
			}
		} else if m.listIdx > 0 {
			m.listIdx--
		}
		return m, nil
	case "down", "j":
		if m.activePane == paneSidebar {
			entries := m.sidebarEntries()
			if m.sidebarIdx < len(entries)-1 {
				m.sidebarIdx++
				m.applySidebarSelection()
			}
		} else if m.listIdx < len(m.visibleItems())-1 {
			m.listIdx++
		}
		return m, nil
	case "enter", "right", "l":
		if m.activePane == paneSidebar {
			m.activePane = paneList
			return m, nil
		}
		if m.grab.active {
			if err := m.dropGrabbedItem(); err != nil {
				m.status = err.Error()
				return m, nil
			}
			if err := m.save(); err != nil {
				m.status = fmt.Sprintf("save failed: %v", err)
				return m, nil
			}
			m.normalize()
			m.status = successStyle.Render("Dropped.")
			return m, nil
		}
		item, ok := m.selectedItem()
		if !ok {
			return m, nil
		}
		if item.kind == itemGoal {
			m.screenBack = append(m.screenBack, m.screen)
			m.setScreen(screenState{kind: screenGoal, goalID: item.id}, true)
			m.status = successStyle.Render("Opened goal.")
		}
		return m, nil
	case "esc", "left", "h", "backspace":
		if m.grab.active {
			m.grab = grabState{}
			m.status = "Grab canceled."
			return m, nil
		}
		if len(m.screenBack) > 0 {
			last := m.screenBack[len(m.screenBack)-1]
			m.screenBack = m.screenBack[:len(m.screenBack)-1]
			m.setScreen(last, true)
			return m, nil
		}
		m.activePane = paneSidebar
		return m, nil
	case "v":
		if m.grab.active {
			m.grab = grabState{}
			m.status = "Grab canceled."
			return m, nil
		}
		item, ok := m.selectedItem()
		if !ok {
			m.status = "Nothing to grab."
			return m, nil
		}
		m.grab = grabState{active: true, item: item}
		m.status = warnStyle.Render("Grab active. Move to target row and press enter to drop.")
		return m, nil
	case "J":
		if err := m.bumpSelection(1); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
			return m, nil
		}
		m.normalize()
		m.status = successStyle.Render("Moved down.")
		return m, nil
	case "K":
		if err := m.bumpSelection(-1); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
			return m, nil
		}
		m.normalize()
		m.status = successStyle.Render("Moved up.")
		return m, nil
	case "S":
		if err := m.autoSortVisible(); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
			return m, nil
		}
		m.normalize()
		m.status = successStyle.Render("Sorted by urgent/important.")
		return m, nil
	case "e":
		mode, id := m.editTarget()
		if mode != formNone {
			m.startForm(mode, id)
			return m, textinput.Blink
		}
	case "d", "x":
		if err := m.deleteTarget(); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
			return m, nil
		}
		m.normalize()
		return m, nil
	case "s":
		if m.screen.kind == screenMilestone || m.screen.kind == screenGoal {
			m.startForm(formAddGoal, 0)
			return m, textinput.Blink
		}
	case "p", " ":
		m.timer.running = !m.timer.running
		if m.timer.running {
			m.status = successStyle.Render("Pomodoro running.")
			return m, timerTick()
		}
		m.status = "Pomodoro paused."
		return m, nil
	case "r":
		m.resetTimer()
		m.status = "Pomodoro reset."
		return m, nil
	case "N":
		m.advanceTimer()
		m.status = successStyle.Render("Pomodoro phase advanced.")
		if m.timer.running {
			return m, timerTick()
		}
		return m, nil
	case "u":
		if err := m.togglePriority(false); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
		}
		return m, nil
	case "I":
		if err := m.togglePriority(true); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
		}
		return m, nil
	}
	return m, nil
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.form = formState{}
		m.status = "Canceled."
		return m, nil
	case "tab", "shift+tab", "up", "down":
		step := 1
		if msg.String() == "shift+tab" || msg.String() == "up" {
			step = -1
		}
		m.form.inputs[m.form.index].Blur()
		m.form.index = (m.form.index + step + len(m.form.inputs)) % len(m.form.inputs)
		m.form.inputs[m.form.index].Focus()
		return m, nil
	case "enter":
		if m.form.index < len(m.form.inputs)-1 {
			m.form.inputs[m.form.index].Blur()
			m.form.index++
			m.form.inputs[m.form.index].Focus()
			return m, nil
		}
		if err := m.submitForm(); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
			return m, nil
		}
		m.form = formState{}
		m.normalize()
		return m, nil
	}

	var cmd tea.Cmd
	m.form.inputs[m.form.index], cmd = m.form.inputs[m.form.index].Update(msg)
	return m, cmd
}

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.search = searchState{}
		m.status = "Search canceled."
		return m, nil
	case "up", "k":
		if m.search.index > 0 {
			m.search.index--
		}
		return m, nil
	case "down", "j":
		results := m.searchResults()
		if m.search.index < len(results)-1 {
			m.search.index++
		}
		return m, nil
	case "enter":
		results := m.searchResults()
		if len(results) == 0 || m.search.index >= len(results) {
			return m, nil
		}
		result := results[m.search.index]
		switch m.search.mode {
		case searchJump:
			if err := m.applyJumpResult(result); err != nil {
				m.status = err.Error()
				return m, nil
			}
		case searchMove:
			if err := m.applyMoveResult(m.search.item, result); err != nil {
				m.status = err.Error()
				return m, nil
			}
			if err := m.save(); err != nil {
				m.status = fmt.Sprintf("save failed: %v", err)
				return m, nil
			}
			m.normalize()
		}
		m.search = searchState{}
		return m, nil
	}

	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	if m.search.index >= len(m.searchResults()) {
		m.search.index = max(0, len(m.searchResults())-1)
	}
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	header := m.renderHeader()
	sidebar := m.renderSidebar()
	list := m.renderList()
	detail := m.renderDetail()
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, list, detail)

	parts := []string{header, body, mutedStyle.Render(m.status)}
	if m.showHelp {
		parts = append(parts, m.renderHelp())
	}
	if m.search.active {
		parts = append(parts, m.renderSearch())
	}
	if m.form.mode != formNone {
		parts = append(parts, m.renderForm())
	}
	return appStyle.Render(strings.Join(parts, "\n\n"))
}

func (m model) renderHeader() string {
	stats := fmt.Sprintf(
		"%s  %s  %s  %s",
		highlightStyle.Render(fmt.Sprintf("%d milestones", len(m.data.Milestones))),
		highlightStyle.Render(fmt.Sprintf("%d goals", len(m.data.Goals))),
		highlightStyle.Render(fmt.Sprintf("%d todos", len(m.data.Todos))),
		highlightStyle.Render(m.timerBadge()),
	)

	lines := []string{
		headerStyle.Render("Planner"),
		mutedStyle.Render("Use tab to switch panes."),
		stats,
		mutedStyle.Render(fmt.Sprintf("Pomodoro: %s", m.timerSummary())),
	}
	if m.grab.active {
		lines = append(lines, warnStyle.Render("Grab mode active. Press enter to drop or h to cancel."))
	}
	return panelStyle.Width(max(48, m.width-6)).Render(strings.Join(lines, "\n"))
}

func (m model) renderSidebar() string {
	active := m.activePane == paneSidebar
	lines := []string{panelHeading("Spaces", active)}
	entries := m.sidebarEntries()
	for i, entry := range entries {
		line := entry.label
		if entry.meta != "" {
			line = fmt.Sprintf("%s\n%s", line, mutedStyle.Render(entry.meta))
		}
		if i == m.sidebarIdx && m.activePane == paneSidebar {
			line = activeRowStyle.Render(line)
		} else if i == m.sidebarIdx {
			line = inactiveRowStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return panelFrame(active).Width(max(24, min(30, m.width/5))).Render(strings.Join(lines, "\n\n"))
}

func (m model) renderList() string {
	active := m.activePane == paneList
	width := max(42, m.width/2-10)
	lines := []string{panelHeading(m.screenTitle(), active)}
	if subtitle := m.screenSubtitle(); subtitle != "" {
		lines = append(lines, mutedStyle.Render(subtitle))
	}

	items := m.visibleItems()
	if len(items) == 0 {
		lines = append(lines, "", mutedStyle.Render("Nothing here yet. Press n to add an item."))
	} else {
		lines = append(lines, "")
		for i, item := range items {
			if i == m.listIdx && m.activePane == paneList {
				line := m.renderActiveItem(item)
				if m.grab.active && m.grab.item.kind == item.kind && m.grab.item.id == item.id {
					line = activeRowStyle.Render("GRAB " + line)
				} else {
					line = activeRowStyle.Render(line)
				}
				lines = append(lines, line)
			} else if i == m.listIdx {
				line := m.renderItem(item)
				if m.grab.active && m.grab.item.kind == item.kind && m.grab.item.id == item.id {
					line = warnStyle.Render("GRAB ") + line
				}
				lines = append(lines, inactiveRowStyle.Render(line))
			} else {
				line := m.renderItem(item)
				if m.grab.active && m.grab.item.kind == item.kind && m.grab.item.id == item.id {
					line = warnStyle.Render("GRAB ") + line
				}
				lines = append(lines, line)
			}
		}
	}

	lines = append(lines, "", mutedStyle.Render(m.contextHint()))
	return panelFrame(active).Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderDetail() string {
	width := max(34, m.width-lipgloss.Width(m.renderSidebar())-lipgloss.Width(m.renderList())-12)
	lines := []string{headerStyle.Render("Details")}

	if item, ok := m.selectedItem(); ok {
		lines = append(lines, m.renderSelectionDetail(item)...)
	} else {
		lines = append(lines, mutedStyle.Render("Select a task or goal to inspect and edit it."))
	}

	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("Shortcuts"))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("n quick add to %s", m.quickAddBrowseDestinationLabel())))
	lines = append(lines, mutedStyle.Render("/ search or create a todo"))
	lines = append(lines, mutedStyle.Render("m move selection to Inbox, goal, or milestone"))
	lines = append(lines, mutedStyle.Render("v grab, then move and press enter to drop"))
	return panelStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderSelectionDetail(item focusItem) []string {
	switch item.kind {
	case itemGoal:
		goal := m.mustGoal(item.id)
		lines := []string{
			headerStyle.Render(goal.Name),
			mutedStyle.Render(fmt.Sprintf("Goal • %s", dateRange(goal.StartDate, goal.EndDate))),
		}
		lines = append(lines, mutedStyle.Render(strings.Join(m.goalPath(goal), " / ")))
		meta := append(m.priorityMeta(goal.Important, goal.Urgent), fmt.Sprintf("%d subgoals", m.countChildGoals(goal.ID)), fmt.Sprintf("%d todos", m.countGoalTodos(goal.ID)))
		lines = append(lines, strings.Join(meta, " • "))
		return lines
	case itemTodo:
		todo := m.mustTodo(item.id)
		lines := []string{
			headerStyle.Render(todo.Name),
			mutedStyle.Render(fmt.Sprintf("Todo • %s", dateRange(todo.StartDate, todo.EndDate))),
			mutedStyle.Render(m.todoContext(todo)),
		}
		meta := m.priorityMeta(todo.Important, todo.Urgent)
		if len(meta) == 0 {
			meta = []string{"normal priority"}
		}
		lines = append(lines, strings.Join(meta, " • "))
		return lines
	default:
		return []string{mutedStyle.Render("Unknown selection")}
	}
}

func (m model) renderItem(item focusItem) string {
	switch item.kind {
	case itemGoal:
		goal := m.mustGoal(item.id)
		meta := fmt.Sprintf("%d subgoals • %d todos", m.countChildGoals(goal.ID), m.countGoalTodos(goal.ID))
		return fmt.Sprintf("%s %s%s\n%s", highlightStyle.Render("G"), goal.Name, m.prioritySuffix(goal.Important, goal.Urgent), mutedStyle.Render(meta))
	case itemTodo:
		todo := m.mustTodo(item.id)
		return fmt.Sprintf("%s %s%s\n%s", highlightStyle.Render("T"), todo.Name, m.prioritySuffix(todo.Important, todo.Urgent), mutedStyle.Render(fmt.Sprintf("%s • %s", m.todoContext(todo), dateRange(todo.StartDate, todo.EndDate))))
	default:
		return ""
	}
}

func (m model) renderActiveItem(item focusItem) string {
	switch item.kind {
	case itemGoal:
		goal := m.mustGoal(item.id)
		meta := fmt.Sprintf("%d subgoals • %d todos", m.countChildGoals(goal.ID), m.countGoalTodos(goal.ID))
		suffix := strings.Join(m.priorityMeta(goal.Important, goal.Urgent), " ")
		if suffix != "" {
			suffix = " " + suffix
		}
		return fmt.Sprintf("G %s%s\n%s", goal.Name, suffix, meta)
	case itemTodo:
		todo := m.mustTodo(item.id)
		suffix := strings.Join(m.priorityMeta(todo.Important, todo.Urgent), " ")
		if suffix != "" {
			suffix = " " + suffix
		}
		return fmt.Sprintf("T %s%s\n%s • %s", todo.Name, suffix, m.todoContext(todo), dateRange(todo.StartDate, todo.EndDate))
	default:
		return ""
	}
}

func (m model) renderHelp() string {
	help := []string{
		"n or a quick add todo to Inbox",
		"/ search to jump, or create a todo from the query",
		"m move selected goal/todo without navigating first",
		"v start grab mode, move cursor, enter to drop",
		"i open Inbox",
		"A open All Tasks",
		"s add goal in milestone or goal views",
		"M add milestone",
		"e edit selected item",
			"x or d delete selected item",
			"I toggle important",
			"u toggle urgent",
			"S auto-sort current list by urgent/important",
			"tab switch sidebar/list",
			"enter open selected goal",
			"h go back or cancel grab",
		"p or space start/pause pomodoro",
		"r reset pomodoro",
		"N advance pomodoro phase",
	}
	return panelStyle.Render(strings.Join(help, "\n"))
}

func (m model) renderSearch() string {
	title := "Jump"
	if m.search.mode == searchMove {
		title = "Move To"
	}

	lines := []string{
		headerStyle.Render(title),
		m.search.input.View(),
	}
	results := m.searchResults()
		if len(results) == 0 {
			lines = append(lines, mutedStyle.Render("No matches"))
		} else {
			for i, result := range results {
				line := result.label
				if i == m.search.index {
					line = activeBadgeStyle.Render(line)
				}
				lines = append(lines, line)
			}
	}
	lines = append(lines, mutedStyle.Render("enter confirm • esc cancel"))
	return formStyle.Width(max(56, m.width/2)).Render(strings.Join(lines, "\n\n"))
}

func (m model) renderForm() string {
	titles := map[formMode]string{
		formQuickAdd:      "Quick Add",
		formAddMilestone:  "Add Milestone",
		formAddGoal:       "Add Goal",
		formEditMilestone: "Edit Milestone",
		formEditGoal:      "Edit Goal",
		formEditTodo:      "Edit Todo",
	}
	lines := []string{headerStyle.Render(titles[m.form.mode])}
	for i, input := range m.form.inputs {
		label := "Name"
		if m.form.mode == formQuickAdd {
			label = "Todo"
		} else if i == 1 {
			label = "Start date"
		} else if i == 2 {
			label = "End date"
		}
		lines = append(lines, fmt.Sprintf("%s\n%s", mutedStyle.Render(label), input.View()))
	}
	if m.form.mode == formQuickAdd {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("Creates a todo in %s.", m.quickAddDestinationLabel())))
	}
	lines = append(lines, mutedStyle.Render("enter submit • tab move • esc cancel"))
	return formStyle.Width(max(46, m.width/2)).Render(strings.Join(lines, "\n\n"))
}

func (m *model) startForm(mode formMode, target int) {
	count := 1
	if mode == formAddMilestone || mode == formAddGoal || mode == formEditMilestone || mode == formEditGoal || mode == formEditTodo {
		count = 3
	}
	inputs := make([]textinput.Model, count)
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].Width = 40
	}
	if count == 3 {
		inputs[1].Placeholder = "YYYY-MM-DD (optional)"
		inputs[2].Placeholder = "YYYY-MM-DD (optional)"
	}

	switch mode {
	case formQuickAdd:
		inputs[0].Placeholder = "Task name"
	case formAddMilestone:
		inputs[0].Placeholder = "Milestone name"
	case formAddGoal:
		inputs[0].Placeholder = "Goal name"
	case formEditMilestone:
		item := m.mustMilestone(target)
		inputs[0].SetValue(item.Name)
		inputs[1].SetValue(item.StartDate)
		inputs[2].SetValue(item.EndDate)
	case formEditGoal:
		item := m.mustGoal(target)
		inputs[0].SetValue(item.Name)
		inputs[1].SetValue(item.StartDate)
		inputs[2].SetValue(item.EndDate)
	case formEditTodo:
		item := m.mustTodo(target)
		inputs[0].SetValue(item.Name)
		inputs[1].SetValue(item.StartDate)
		inputs[2].SetValue(item.EndDate)
	}

	inputs[0].Focus()
	m.form = formState{mode: mode, target: target, inputs: inputs, index: 0}
}

func (m *model) startSearch(mode searchMode, item focusItem) {
	m.search = searchState{
		active: true,
		mode:   mode,
		index:  0,
		item:   item,
		input:  textinput.New(),
	}
	m.search.input.Width = 42
	m.search.input.Focus()
	if mode == searchJump {
		m.search.input.Placeholder = "Search or create a todo"
	} else {
		m.search.input.Placeholder = "Move to Inbox, milestone, or goal"
	}
}

func (m *model) submitForm() error {
	name := strings.TrimSpace(m.form.inputs[0].Value())
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	startDate, endDate, err := formDateValues(m.form)
	if err != nil {
		return err
	}

	switch m.form.mode {
	case formQuickAdd:
		goalID := m.form.target
		newTodo := todo{
			ID:          m.nextID(),
			GoalID:      goalID,
			Name:        name,
			Order:       m.nextTodoOrder(goalID),
			GlobalOrder: m.nextTodoGlobalOrder(),
		}
		m.data.Todos = append(m.data.Todos, newTodo)
		if goalID == 0 {
			m.setScreen(screenState{kind: screenInbox}, false)
			m.status = successStyle.Render("Todo captured in Inbox.")
		} else {
			m.setScreen(screenState{kind: screenGoal, goalID: goalID}, true)
			m.status = successStyle.Render("Todo added to goal.")
		}
		m.selectTodo(newTodo.ID)
	case formAddMilestone:
		m.data.Milestones = append(m.data.Milestones, milestone{
			ID:        m.nextID(),
			Name:      name,
			StartDate: startDate,
			EndDate:   endDate,
			Order:     len(m.data.Milestones) + 1,
		})
		m.status = successStyle.Render("Milestone added.")
	case formAddGoal:
		parentGoal := 0
		milestoneID := m.screen.milestoneID
		if m.screen.kind == screenGoal {
			parent := m.mustGoal(m.screen.goalID)
			parentGoal = parent.ID
			milestoneID = parent.MilestoneID
		}
		if milestoneID == 0 {
			return fmt.Errorf("open a milestone or goal first")
		}
		newGoal := goal{
			ID:           m.nextID(),
			MilestoneID:  milestoneID,
			ParentGoalID: parentGoal,
			Name:         name,
			StartDate:    startDate,
			EndDate:      endDate,
			Order:        m.nextGoalOrder(milestoneID, parentGoal),
		}
		m.data.Goals = append(m.data.Goals, newGoal)
		m.selectGoal(newGoal.ID)
		m.status = successStyle.Render("Goal added.")
	case formEditMilestone:
		item := m.mustMilestonePtr(m.form.target)
		item.Name = name
		item.StartDate = startDate
		item.EndDate = endDate
		m.status = successStyle.Render("Milestone updated.")
	case formEditGoal:
		item := m.mustGoalPtr(m.form.target)
		item.Name = name
		item.StartDate = startDate
		item.EndDate = endDate
		m.status = successStyle.Render("Goal updated.")
	case formEditTodo:
		item := m.mustTodoPtr(m.form.target)
		item.Name = name
		item.StartDate = startDate
		item.EndDate = endDate
		m.status = successStyle.Render("Todo updated.")
	}
	return nil
}

func (m *model) applyJumpResult(result searchResult) error {
	switch result.kind {
	case "create":
		newTodo := todo{
			ID:          m.nextID(),
			GoalID:      0,
			Name:        result.query,
			Order:       m.nextTodoOrder(0),
			GlobalOrder: m.nextTodoGlobalOrder(),
		}
		m.data.Todos = append(m.data.Todos, newTodo)
		if err := m.save(); err != nil {
			return err
		}
		m.setScreen(screenState{kind: screenInbox}, false)
		m.selectTodo(newTodo.ID)
		m.status = successStyle.Render("Todo created from search.")
	case "milestone":
		m.setScreen(screenState{kind: screenMilestone, milestoneID: result.id}, false)
		m.status = successStyle.Render("Jumped to milestone.")
	case "goal":
		m.screenBack = nil
		m.setScreen(screenState{kind: screenGoal, goalID: result.id}, true)
		m.status = successStyle.Render("Jumped to goal.")
	case "todo":
		if result.goalID == 0 {
			m.setScreen(screenState{kind: screenInbox}, false)
		} else {
			m.screenBack = nil
			m.setScreen(screenState{kind: screenGoal, goalID: result.goalID}, true)
		}
		m.selectTodo(result.id)
		m.status = successStyle.Render("Jumped to todo.")
	}
	return nil
}

func (m *model) applyMoveResult(item focusItem, result searchResult) error {
	switch item.kind {
	case itemTodo:
		target := m.mustTodoPtr(item.id)
		if target == nil {
			return fmt.Errorf("todo not found")
		}
		switch result.kind {
		case "inbox":
			target.GoalID = 0
			target.Order = m.nextTodoOrder(0)
			m.setScreen(screenState{kind: screenInbox}, false)
			m.selectTodo(target.ID)
		case "goal":
			target.GoalID = result.id
			target.Order = m.nextTodoOrder(result.id)
			m.setScreen(screenState{kind: screenGoal, goalID: result.id}, true)
			m.selectTodo(target.ID)
		default:
			return fmt.Errorf("todo can only move to Inbox or a goal")
		}
		m.status = successStyle.Render("Todo moved.")
	case itemGoal:
		target := m.mustGoalPtr(item.id)
		if target == nil {
			return fmt.Errorf("goal not found")
		}
		switch result.kind {
		case "milestone":
			target.MilestoneID = result.id
			target.ParentGoalID = 0
			target.Order = m.nextGoalOrder(result.id, 0)
			m.setScreen(screenState{kind: screenMilestone, milestoneID: result.id}, false)
			m.selectGoal(target.ID)
		case "goal":
			if result.id == target.ID || m.goalIsDescendant(result.id, target.ID) {
				return fmt.Errorf("cannot move a goal into itself or its descendants")
			}
			parent := m.mustGoal(result.id)
			target.MilestoneID = parent.MilestoneID
			target.ParentGoalID = parent.ID
			target.Order = m.nextGoalOrder(parent.MilestoneID, parent.ID)
			m.setScreen(screenState{kind: screenGoal, goalID: parent.ID}, true)
			m.selectGoal(target.ID)
		default:
			return fmt.Errorf("goal can only move to a milestone or goal")
		}
		m.status = successStyle.Render("Goal moved.")
	default:
		return fmt.Errorf("nothing to move")
	}
	return nil
}

func (m *model) deleteTarget() error {
	item, ok := m.selectedItem()
	if !ok {
		if m.screen.kind == screenMilestone {
			m.deleteMilestone(m.screen.milestoneID)
			m.setScreen(screenState{kind: screenInbox}, false)
			m.status = successStyle.Render("Milestone deleted.")
			return nil
		}
		return fmt.Errorf("nothing selected")
	}

	switch item.kind {
	case itemTodo:
		m.data.Todos = deleteByID(m.data.Todos, item.id, func(t todo) int { return t.ID })
		m.status = successStyle.Render("Todo deleted.")
	case itemGoal:
		m.deleteGoal(item.id)
		m.status = successStyle.Render("Goal deleted.")
	default:
		return fmt.Errorf("nothing selected")
	}
	return nil
}

func (m *model) editTarget() (formMode, int) {
	item, ok := m.selectedItem()
	if ok {
		if item.kind == itemGoal {
			return formEditGoal, item.id
		}
		return formEditTodo, item.id
	}
	if m.screen.kind == screenMilestone && m.screen.milestoneID != 0 {
		return formEditMilestone, m.screen.milestoneID
	}
	return formNone, 0
}

func (m *model) togglePriority(toggleImportant bool) error {
	item, ok := m.selectedItem()
	if !ok {
		return fmt.Errorf("select a goal or todo first")
	}

	switch item.kind {
	case itemGoal:
		target := m.mustGoalPtr(item.id)
		if toggleImportant {
			target.Important = !target.Important
			m.status = successStyle.Render(fmt.Sprintf("Goal marked %simportant.", boolPrefix(target.Important)))
		} else {
			target.Urgent = !target.Urgent
			m.status = successStyle.Render(fmt.Sprintf("Goal marked %surgent.", boolPrefix(target.Urgent)))
		}
	case itemTodo:
		target := m.mustTodoPtr(item.id)
		if toggleImportant {
			target.Important = !target.Important
			m.status = successStyle.Render(fmt.Sprintf("Todo marked %simportant.", boolPrefix(target.Important)))
		} else {
			target.Urgent = !target.Urgent
			m.status = successStyle.Render(fmt.Sprintf("Todo marked %surgent.", boolPrefix(target.Urgent)))
		}
	default:
		return fmt.Errorf("select a goal or todo first")
	}
	return nil
}

func (m *model) bumpSelection(direction int) error {
	if direction != -1 && direction != 1 {
		return fmt.Errorf("invalid move direction")
	}
	if m.activePane == paneSidebar {
		entries := m.sidebarEntries()
		if m.sidebarIdx < 2 || m.sidebarIdx >= len(entries) {
			return fmt.Errorf("reorder milestones from the milestone list only")
		}
		next := m.sidebarIdx + direction
		if next < 2 || next >= len(entries) {
			return fmt.Errorf("cannot move further")
		}
		currentID := entries[m.sidebarIdx].milestoneID
		nextID := entries[next].milestoneID
		cur := m.mustMilestonePtr(currentID)
		other := m.mustMilestonePtr(nextID)
		cur.Order, other.Order = other.Order, cur.Order
		m.sidebarIdx = next
		m.applySidebarSelection()
		return nil
	}

	items := m.visibleItems()
	if len(items) == 0 || m.listIdx >= len(items) {
		return fmt.Errorf("nothing selected")
	}
	next := m.listIdx + direction
	if next < 0 || next >= len(items) {
		return fmt.Errorf("cannot move further")
	}
	current := items[m.listIdx]
	other := items[next]
	if !m.canReorder(current, other) {
		return fmt.Errorf("reorder within the same parent only")
	}
	m.swapItemOrder(current, other)
	m.listIdx = next
	return nil
}

func (m *model) dropGrabbedItem() error {
	if !m.grab.active {
		return fmt.Errorf("nothing grabbed")
	}
	items := m.visibleItems()
	if len(items) == 0 || m.listIdx >= len(items) {
		return fmt.Errorf("no drop target")
	}
	target := items[m.listIdx]
	if !m.canReorder(m.grab.item, target) {
		return fmt.Errorf("drop only works within the same list")
	}

	m.reorderVisibleItems(m.grab.item, target)
	m.grab = grabState{}
	return nil
}

func (m *model) autoSortVisible() error {
	items := m.visibleItems()
	if len(items) < 2 {
		return fmt.Errorf("nothing to sort")
	}
	selected, hasSelection := m.selectedItem()
	ordered := append([]focusItem(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return m.focusItemPriorityScore(ordered[i]) > m.focusItemPriorityScore(ordered[j])
	})
	for i, item := range ordered {
		if m.screen.kind == screenAll && item.kind == itemTodo {
			m.mustTodoPtr(item.id).GlobalOrder = i + 1
			continue
		}
		switch item.kind {
		case itemGoal:
			m.mustGoalPtr(item.id).Order = i + 1
		case itemTodo:
			m.mustTodoPtr(item.id).Order = i + 1
		}
	}
	if hasSelection {
		m.selectItem(selected)
	}
	return nil
}

func (m *model) reorderVisibleItems(source, target focusItem) {
	items := m.visibleItems()
	sourceIndex := -1
	targetIndex := -1
	for i, item := range items {
		if item.kind == source.kind && item.id == source.id {
			sourceIndex = i
		}
		if item.kind == target.kind && item.id == target.id {
			targetIndex = i
		}
	}
	if sourceIndex == -1 || targetIndex == -1 {
		return
	}

	ordered := append([]focusItem(nil), items...)
	moving := ordered[sourceIndex]
	ordered = append(ordered[:sourceIndex], ordered[sourceIndex+1:]...)
	if sourceIndex < targetIndex {
		targetIndex--
	}
	ordered = slices.Insert(ordered, targetIndex, moving)
	for i, item := range ordered {
		if m.screen.kind == screenAll && item.kind == itemTodo {
			m.mustTodoPtr(item.id).GlobalOrder = i + 1
			continue
		}
		switch item.kind {
		case itemGoal:
			m.mustGoalPtr(item.id).Order = i + 1
		case itemTodo:
			m.mustTodoPtr(item.id).Order = i + 1
		}
	}
	m.selectItem(moving)
}

func (m model) focusItemPriorityScore(item focusItem) int {
	switch item.kind {
	case itemGoal:
		goal := m.mustGoal(item.id)
		return priorityScore(goal.Important, goal.Urgent)
	case itemTodo:
		todo := m.mustTodo(item.id)
		return priorityScore(todo.Important, todo.Urgent)
	default:
		return 0
	}
}

func (m model) canReorder(a, b focusItem) bool {
	if a.kind == itemTodo && b.kind == itemTodo {
		if m.screen.kind == screenAll {
			return true
		}
		return m.mustTodo(a.id).GoalID == m.mustTodo(b.id).GoalID
	}
	if a.kind == itemGoal && b.kind == itemGoal {
		goalA := m.mustGoal(a.id)
		goalB := m.mustGoal(b.id)
		return goalA.MilestoneID == goalB.MilestoneID && goalA.ParentGoalID == goalB.ParentGoalID
	}
	return false
}

func (m *model) swapItemOrder(a, b focusItem) {
	if m.screen.kind == screenAll && a.kind == itemTodo && b.kind == itemTodo {
		m.mustTodoPtr(a.id).GlobalOrder = b.order
		m.mustTodoPtr(b.id).GlobalOrder = a.order
		return
	}
	switch a.kind {
	case itemGoal:
		m.mustGoalPtr(a.id).Order = b.order
	case itemTodo:
		m.mustTodoPtr(a.id).Order = b.order
	}
	switch b.kind {
	case itemGoal:
		m.mustGoalPtr(b.id).Order = a.order
	case itemTodo:
		m.mustTodoPtr(b.id).Order = a.order
	}
}

func (m *model) applySidebarSelection() {
	entries := m.sidebarEntries()
	if len(entries) == 0 {
		return
	}
	if m.sidebarIdx >= len(entries) {
		m.sidebarIdx = len(entries) - 1
	}
	m.setScreen(entries[m.sidebarIdx].screen, false)
}

func (m *model) setScreen(screen screenState, preserveSelection bool) {
	m.screen = screen
	if !preserveSelection {
		m.listIdx = 0
	}

	switch screen.kind {
	case screenMilestone:
		for i, entry := range m.sidebarEntries() {
			if entry.screen.kind == screenMilestone && entry.screen.milestoneID == screen.milestoneID {
				m.sidebarIdx = i
				break
			}
		}
	case screenInbox:
		m.sidebarIdx = 0
	case screenAll:
		m.sidebarIdx = 1
	}
}

func (m *model) normalize() {
	m.ensureOrdering()
	entries := m.sidebarEntries()
	if len(entries) == 0 {
		m.sidebarIdx = 0
	} else {
		if m.sidebarIdx < 0 {
			m.sidebarIdx = 0
		}
		if m.sidebarIdx >= len(entries) {
			m.sidebarIdx = len(entries) - 1
		}
	}
	if !m.screenExists(m.screen) {
		m.setScreen(screenState{kind: screenInbox}, false)
	}
	items := m.visibleItems()
	if len(items) == 0 {
		m.listIdx = 0
	} else {
		if m.listIdx < 0 {
			m.listIdx = 0
		}
		if m.listIdx >= len(items) {
			m.listIdx = len(items) - 1
		}
	}
	if m.grab.active {
		found := false
		for _, item := range items {
			if item.kind == m.grab.item.kind && item.id == m.grab.item.id {
				found = true
				break
			}
		}
		if !found {
			m.grab = grabState{}
		}
	}
}

func (m *model) save() error {
	payload, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.dataPath, payload, 0o644)
}

func loadData(path string) (plannerData, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return plannerData{NextID: 1}, nil
		}
		return plannerData{}, err
	}
	var data plannerData
	if err := json.Unmarshal(contents, &data); err != nil {
		return plannerData{}, err
	}
	if data.NextID == 0 {
		data.NextID = 1
	}
	return data, nil
}

func (m *model) ensureOrdering() {
	for i := range m.data.Milestones {
		if m.data.Milestones[i].Order == 0 {
			m.data.Milestones[i].Order = i + 1
		}
	}
	slices.SortFunc(m.data.Milestones, func(a, b milestone) int {
		return compareOrder(a.Order, b.Order, a.ID, b.ID)
	})
	for i := range m.data.Milestones {
		m.data.Milestones[i].Order = i + 1
	}

	goalBuckets := map[string][]int{}
	for i := range m.data.Goals {
		key := fmt.Sprintf("%d:%d", m.data.Goals[i].MilestoneID, m.data.Goals[i].ParentGoalID)
		goalBuckets[key] = append(goalBuckets[key], i)
	}
	for _, indexes := range goalBuckets {
		slices.SortFunc(indexes, func(a, b int) int {
			return compareOrder(m.data.Goals[a].Order, m.data.Goals[b].Order, m.data.Goals[a].ID, m.data.Goals[b].ID)
		})
		for pos, idx := range indexes {
			m.data.Goals[idx].Order = pos + 1
		}
	}

	todoBuckets := map[int][]int{}
	for i := range m.data.Todos {
		todoBuckets[m.data.Todos[i].GoalID] = append(todoBuckets[m.data.Todos[i].GoalID], i)
	}
	for _, indexes := range todoBuckets {
		slices.SortFunc(indexes, func(a, b int) int {
			return compareOrder(m.data.Todos[a].Order, m.data.Todos[b].Order, m.data.Todos[a].ID, m.data.Todos[b].ID)
		})
		for pos, idx := range indexes {
			m.data.Todos[idx].Order = pos + 1
		}
	}

	allTodos := make([]int, len(m.data.Todos))
	for i := range m.data.Todos {
		if m.data.Todos[i].GlobalOrder == 0 {
			m.data.Todos[i].GlobalOrder = i + 1
		}
		allTodos[i] = i
	}
	slices.SortFunc(allTodos, func(a, b int) int {
		return compareOrder(m.data.Todos[a].GlobalOrder, m.data.Todos[b].GlobalOrder, m.data.Todos[a].ID, m.data.Todos[b].ID)
	})
	for pos, idx := range allTodos {
		m.data.Todos[idx].GlobalOrder = pos + 1
	}
}

func (m *model) nextID() int {
	id := m.data.NextID
	m.data.NextID++
	return id
}

func (m model) sidebarEntries() []sidebarEntry {
	entries := []sidebarEntry{
		{label: "Inbox", meta: fmt.Sprintf("%d todos", len(m.inboxTodos())), screen: screenState{kind: screenInbox}},
		{label: "All Tasks", meta: fmt.Sprintf("%d todos", len(m.data.Todos)), screen: screenState{kind: screenAll}},
	}
	for _, milestone := range m.data.Milestones {
		entries = append(entries, sidebarEntry{
			label:       milestone.Name,
			meta:        fmt.Sprintf("%d goals • %d todos", m.countMilestoneGoals(milestone.ID), m.countMilestoneTodos(milestone.ID)),
			screen:      screenState{kind: screenMilestone, milestoneID: milestone.ID},
			milestoneID: milestone.ID,
		})
	}
	return entries
}

func (m model) visibleItems() []focusItem {
	switch m.screen.kind {
	case screenInbox:
		items := make([]focusItem, 0, len(m.data.Todos))
		for _, item := range m.inboxTodos() {
			items = append(items, focusItem{kind: itemTodo, id: item.ID, order: item.Order})
		}
		return items
	case screenAll:
		items := make([]focusItem, 0, len(m.data.Todos))
		for _, item := range m.allTodos() {
			items = append(items, focusItem{kind: itemTodo, id: item.ID, order: item.GlobalOrder})
		}
		return items
	case screenMilestone:
		items := []focusItem{}
		for _, goal := range m.data.Goals {
			if goal.MilestoneID == m.screen.milestoneID && goal.ParentGoalID == 0 {
				items = append(items, focusItem{kind: itemGoal, id: goal.ID, order: goal.Order})
			}
		}
		slices.SortFunc(items, func(a, b focusItem) int {
			return compareOrder(a.order, b.order, a.id, b.id)
		})
		return items
	case screenGoal:
		items := []focusItem{}
		for _, goal := range m.data.Goals {
			if goal.ParentGoalID == m.screen.goalID {
				items = append(items, focusItem{kind: itemGoal, id: goal.ID, order: goal.Order})
			}
		}
		for _, todo := range m.data.Todos {
			if todo.GoalID == m.screen.goalID {
				items = append(items, focusItem{kind: itemTodo, id: todo.ID, order: todo.Order})
			}
		}
		slices.SortFunc(items, func(a, b focusItem) int {
			return compareOrder(a.order, b.order, a.id, b.id)
		})
		return items
	default:
		return nil
	}
}

func (m model) selectedItem() (focusItem, bool) {
	items := m.visibleItems()
	if len(items) == 0 || m.listIdx >= len(items) {
		return focusItem{}, false
	}
	return items[m.listIdx], true
}

func (m *model) selectItem(target focusItem) {
	items := m.visibleItems()
	for i, item := range items {
		if item.kind == target.kind && item.id == target.id {
			m.listIdx = i
			return
		}
	}
	m.listIdx = 0
}

func (m *model) selectTodo(id int) {
	m.selectItem(focusItem{kind: itemTodo, id: id})
}

func (m *model) selectGoal(id int) {
	m.selectItem(focusItem{kind: itemGoal, id: id})
}

func (m model) screenTitle() string {
	switch m.screen.kind {
	case screenInbox:
		return "Inbox"
	case screenAll:
		return "All Tasks"
	case screenMilestone:
		return m.mustMilestone(m.screen.milestoneID).Name
	case screenGoal:
		return m.mustGoal(m.screen.goalID).Name
	default:
		return "Planner"
	}
}

func (m model) screenSubtitle() string {
	switch m.screen.kind {
	case screenInbox:
		return "Inbox"
	case screenAll:
		return "All todos"
	case screenMilestone:
		milestone := m.mustMilestone(m.screen.milestoneID)
		return fmt.Sprintf("%s • %d top-level goals", dateRange(milestone.StartDate, milestone.EndDate), len(m.visibleItems()))
	case screenGoal:
		goal := m.mustGoal(m.screen.goalID)
		return fmt.Sprintf("%s • %s • %d child items", strings.Join(m.goalPath(goal), " / "), dateRange(goal.StartDate, goal.EndDate), len(m.visibleItems()))
	default:
		return ""
	}
}

func (m model) contextHint() string {
	switch m.screen.kind {
	case screenInbox, screenAll:
		return "n quick add • / jump or create • m move • v grab • e edit • x delete • I/u priority • S sort"
	case screenMilestone:
		return "s add goal • n quick add • enter open goal • m move • v grab • e edit • x delete • I/u priority • S sort"
	case screenGoal:
		return "n add todo • s add subgoal • m move • v grab • e edit • x delete • I/u priority • S sort • h back"
	default:
		return ""
	}
}

func (m model) quickAddGoalID() int {
	if m.screen.kind == screenGoal {
		return m.screen.goalID
	}
	return 0
}

func (m model) quickAddDestinationLabel() string {
	if m.form.target == 0 {
		return "Inbox"
	}
	return fmt.Sprintf("goal %q", m.mustGoal(m.form.target).Name)
}

func (m model) quickAddBrowseDestinationLabel() string {
	if m.screen.kind == screenGoal {
		return fmt.Sprintf("goal %q", m.mustGoal(m.screen.goalID).Name)
	}
	return "Inbox"
}

func (m model) searchResults() []searchResult {
	query := strings.TrimSpace(strings.ToLower(m.search.input.Value()))
	raw := strings.TrimSpace(m.search.input.Value())
	switch m.search.mode {
	case searchJump:
		if query == "" {
			return nil
		}
		results := []searchResult{{
			kind:  "create",
			label: fmt.Sprintf(`+ Create todo "%s" in Inbox`, raw),
			query: raw,
		}}
		for _, milestone := range m.data.Milestones {
			if strings.Contains(strings.ToLower(milestone.Name), query) {
				results = append(results, searchResult{
					kind:  "milestone",
					id:    milestone.ID,
					label: fmt.Sprintf("M  %s", milestone.Name),
				})
			}
		}
		for _, goal := range m.data.Goals {
			if strings.Contains(strings.ToLower(goal.Name), query) {
				results = append(results, searchResult{
					kind:        "goal",
					id:          goal.ID,
					milestoneID: goal.MilestoneID,
					label:       fmt.Sprintf("G  %s", strings.Join(m.goalPath(goal), " / ")),
				})
			}
		}
		for _, todo := range m.data.Todos {
			if strings.Contains(strings.ToLower(todo.Name), query) {
				results = append(results, searchResult{
					kind:   "todo",
					id:     todo.ID,
					goalID: todo.GoalID,
					label:  fmt.Sprintf("T  %s • %s", todo.Name, m.todoContext(todo)),
				})
			}
		}
		return results
	case searchMove:
		if query == "" {
			return m.moveTargets("")
		}
		return m.moveTargets(query)
	default:
		return nil
	}
}

func (m model) moveTargets(query string) []searchResult {
	results := []searchResult{}
	item := m.search.item
	if item.kind == itemTodo {
		if query == "" || strings.Contains("inbox", query) {
			results = append(results, searchResult{kind: "inbox", label: "Inbox"})
		}
		for _, goal := range m.data.Goals {
			label := strings.Join(m.goalPath(goal), " / ")
			if query == "" || strings.Contains(strings.ToLower(label), query) {
				results = append(results, searchResult{
					kind:        "goal",
					id:          goal.ID,
					milestoneID: goal.MilestoneID,
					label:       "Goal: " + label,
				})
			}
		}
		return results
	}

	for _, milestone := range m.data.Milestones {
		if query == "" || strings.Contains(strings.ToLower(milestone.Name), query) {
			results = append(results, searchResult{
				kind:  "milestone",
				id:    milestone.ID,
				label: "Milestone: " + milestone.Name,
			})
		}
	}
	for _, goal := range m.data.Goals {
		if goal.ID == item.id || m.goalIsDescendant(goal.ID, item.id) {
			continue
		}
		label := strings.Join(m.goalPath(goal), " / ")
		if query == "" || strings.Contains(strings.ToLower(label), query) {
			results = append(results, searchResult{
				kind:        "goal",
				id:          goal.ID,
				milestoneID: goal.MilestoneID,
				label:       "Goal: " + label,
			})
		}
	}
	return results
}

func (m model) inboxTodos() []todo {
	items := []todo{}
	for _, item := range m.data.Todos {
		if item.GoalID == 0 {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b todo) int {
		return compareOrder(a.Order, b.Order, a.ID, b.ID)
	})
	return items
}

func (m model) allTodos() []todo {
	items := append([]todo(nil), m.data.Todos...)
	slices.SortFunc(items, func(a, b todo) int {
		return compareOrder(a.GlobalOrder, b.GlobalOrder, a.ID, b.ID)
	})
	return items
}

func (m model) goalPath(goal goal) []string {
	path := []string{}
	milestone := m.mustMilestone(goal.MilestoneID)
	if milestone.ID != 0 {
		path = append(path, milestone.Name)
	}
	chain := []string{goal.Name}
	parent := goal.ParentGoalID
	for parent != 0 {
		next := m.findGoal(parent)
		if next == nil {
			break
		}
		chain = append([]string{next.Name}, chain...)
		parent = next.ParentGoalID
	}
	return append(path, chain...)
}

func (m model) todoContext(item todo) string {
	if item.GoalID == 0 {
		return "Inbox"
	}
	goal := m.findGoal(item.GoalID)
	if goal == nil {
		return "Unknown goal"
	}
	return strings.Join(m.goalPath(*goal), " / ")
}

func (m model) countMilestoneGoals(id int) int {
	count := 0
	for _, goal := range m.data.Goals {
		if goal.MilestoneID == id {
			count++
		}
	}
	return count
}

func (m model) countMilestoneTodos(id int) int {
	count := 0
	for _, todo := range m.data.Todos {
		if todo.GoalID == 0 {
			continue
		}
		goal := m.findGoal(todo.GoalID)
		if goal != nil && goal.MilestoneID == id {
			count++
		}
	}
	return count
}

func (m model) countChildGoals(id int) int {
	count := 0
	for _, goal := range m.data.Goals {
		if goal.ParentGoalID == id {
			count++
		}
	}
	return count
}

func (m model) countGoalTodos(id int) int {
	count := 0
	for _, todo := range m.data.Todos {
		if todo.GoalID == id {
			count++
		}
	}
	return count
}

func (m model) screenExists(screen screenState) bool {
	switch screen.kind {
	case screenInbox, screenAll:
		return true
	case screenMilestone:
		return m.mustMilestone(screen.milestoneID).ID != 0
	case screenGoal:
		return m.mustGoal(screen.goalID).ID != 0
	default:
		return false
	}
}

func (m model) goalIsDescendant(candidateID, rootID int) bool {
	current := m.findGoal(candidateID)
	for current != nil && current.ParentGoalID != 0 {
		if current.ParentGoalID == rootID {
			return true
		}
		current = m.findGoal(current.ParentGoalID)
	}
	return false
}

func (m model) updateTimerTick() (tea.Model, tea.Cmd) {
	if !m.timer.running {
		return m, nil
	}
	m.timer.remaining -= time.Second
	if m.timer.phase == phaseWork {
		m.timer.workAccumulated += time.Second
	}
	if m.timer.remaining <= 0 {
		m.advanceTimer()
		m.status = successStyle.Render(fmt.Sprintf("Pomodoro switched to %s.", m.phaseLabel()))
		notifyPomodoroPhase(m.phaseLabel(), m.timer.remaining)
	}
	if m.timer.running {
		return m, timerTick()
	}
	return m, nil
}

func (m *model) advanceTimer() {
	switch m.timer.phase {
	case phaseWork:
		if m.timer.workAccumulated > 0 && m.timer.workAccumulated%longBreakEveryWork == 0 {
			m.timer.phase = phaseLongBreak
			m.timer.remaining = longBreakDuration
		} else {
			m.timer.phase = phaseShortBreak
			m.timer.remaining = shortBreakDuration
		}
	case phaseShortBreak, phaseLongBreak:
		m.timer.phase = phaseWork
		m.timer.remaining = workDuration
	}
}

func (m *model) resetTimer() {
	m.timer.running = false
	switch m.timer.phase {
	case phaseWork:
		m.timer.remaining = workDuration
	case phaseShortBreak:
		m.timer.remaining = shortBreakDuration
	case phaseLongBreak:
		m.timer.remaining = longBreakDuration
	}
}

func (m model) timerBadge() string {
	label := "paused"
	if m.timer.running {
		label = "running"
	}
	return fmt.Sprintf("%s %s", m.phaseLabel(), label)
}

func (m model) timerSummary() string {
	return fmt.Sprintf("%s • %s remaining • %s work banked", m.phaseLabel(), formatDuration(m.timer.remaining), formatDuration(m.timer.workAccumulated))
}

func (m model) phaseLabel() string {
	switch m.timer.phase {
	case phaseWork:
		return "work"
	case phaseShortBreak:
		return "short break"
	case phaseLongBreak:
		return "long break"
	default:
		return "timer"
	}
}

func timerTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func notifyPomodoroPhase(phase string, duration time.Duration) {
	message := fmt.Sprintf("Switched to %s for %s.", phase, formatDuration(duration))
	script := fmt.Sprintf(`display dialog %q with title %q buttons {"OK"} default button "OK"`, message, "PM Go")
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Start(); err != nil {
		fmt.Printf("\a")
	}
}

func (m *model) deleteMilestone(id int) {
	goalIDs := map[int]struct{}{}
	for _, goal := range m.data.Goals {
		if goal.MilestoneID == id {
			goalIDs[goal.ID] = struct{}{}
		}
	}
	m.data.Milestones = deleteByID(m.data.Milestones, id, func(item milestone) int { return item.ID })
	m.data.Goals = slices.DeleteFunc(m.data.Goals, func(item goal) bool {
		return item.MilestoneID == id
	})
	m.data.Todos = slices.DeleteFunc(m.data.Todos, func(item todo) bool {
		if item.GoalID == 0 {
			return false
		}
		_, ok := goalIDs[item.GoalID]
		return ok
	})
}

func (m *model) deleteGoal(id int) {
	children := []int{id}
	for {
		size := len(children)
		for _, goal := range m.data.Goals {
			if goal.ParentGoalID != 0 && slices.Contains(children, goal.ParentGoalID) && !slices.Contains(children, goal.ID) {
				children = append(children, goal.ID)
			}
		}
		if len(children) == size {
			break
		}
	}
	m.data.Goals = slices.DeleteFunc(m.data.Goals, func(item goal) bool {
		return slices.Contains(children, item.ID)
	})
	m.data.Todos = slices.DeleteFunc(m.data.Todos, func(item todo) bool {
		return slices.Contains(children, item.GoalID)
	})
}

func (m model) findGoal(id int) *goal {
	for i := range m.data.Goals {
		if m.data.Goals[i].ID == id {
			return &m.data.Goals[i]
		}
	}
	return nil
}

func (m model) mustGoal(id int) goal {
	item := m.findGoal(id)
	if item == nil {
		return goal{}
	}
	return *item
}

func (m *model) mustGoalPtr(id int) *goal {
	return m.findGoal(id)
}

func (m model) mustMilestone(id int) milestone {
	for _, item := range m.data.Milestones {
		if item.ID == id {
			return item
		}
	}
	return milestone{}
}

func (m *model) mustMilestonePtr(id int) *milestone {
	for i := range m.data.Milestones {
		if m.data.Milestones[i].ID == id {
			return &m.data.Milestones[i]
		}
	}
	return nil
}

func (m model) mustTodo(id int) todo {
	for _, item := range m.data.Todos {
		if item.ID == id {
			return item
		}
	}
	return todo{}
}

func (m *model) mustTodoPtr(id int) *todo {
	for i := range m.data.Todos {
		if m.data.Todos[i].ID == id {
			return &m.data.Todos[i]
		}
	}
	return nil
}

func deleteByID[T any](items []T, id int, getID func(T) int) []T {
	return slices.DeleteFunc(items, func(item T) bool {
		return getID(item) == id
	})
}

func compareOrder(aOrder, bOrder, aID, bID int) int {
	if aOrder == 0 && bOrder == 0 {
		return aID - bID
	}
	if aOrder == 0 {
		return 1
	}
	if bOrder == 0 {
		return -1
	}
	if aOrder != bOrder {
		return aOrder - bOrder
	}
	return aID - bID
}

func dateRange(start, end string) string {
	if start == "" && end == "" {
		return "No dates"
	}
	switch {
	case start != "" && end != "":
		return start + " -> " + end
	case start != "":
		return "From " + start
	default:
		return "Until " + end
	}
}

func formDateValues(form formState) (string, string, error) {
	if len(form.inputs) < 3 {
		return "", "", nil
	}
	start := strings.TrimSpace(form.inputs[1].Value())
	end := strings.TrimSpace(form.inputs[2].Value())
	if start != "" {
		if _, err := time.Parse(time.DateOnly, start); err != nil {
			return "", "", fmt.Errorf("start date must use YYYY-MM-DD")
		}
	}
	if end != "" {
		if _, err := time.Parse(time.DateOnly, end); err != nil {
			return "", "", fmt.Errorf("end date must use YYYY-MM-DD")
		}
	}
	if start != "" && end != "" && start > end {
		return "", "", fmt.Errorf("start date must be on or before end date")
	}
	return start, end, nil
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	totalSeconds := int(value / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func boolPrefix(value bool) string {
	if value {
		return ""
	}
	return "not "
}

func priorityScore(important, urgent bool) int {
	score := 0
	if important {
		score += 2
	}
	if urgent {
		score += 3
	}
	return score
}

func (m model) prioritySuffix(important, urgent bool) string {
	parts := m.priorityMeta(important, urgent)
	if len(parts) == 0 {
		return ""
	}
	return " " + highlightStyle.Render(strings.Join(parts, " "))
}

func (m model) priorityMeta(important, urgent bool) []string {
	parts := []string{}
	if important {
		parts = append(parts, "important")
	}
	if urgent {
		parts = append(parts, "urgent")
	}
	return parts
}

func (m model) nextGoalOrder(milestoneID, parentGoalID int) int {
	maxOrder := 0
	for _, goal := range m.data.Goals {
		if goal.MilestoneID == milestoneID && goal.ParentGoalID == parentGoalID && goal.Order > maxOrder {
			maxOrder = goal.Order
		}
	}
	return maxOrder + 1
}

func (m model) nextTodoOrder(goalID int) int {
	maxOrder := 0
	for _, todo := range m.data.Todos {
		if todo.GoalID == goalID && todo.Order > maxOrder {
			maxOrder = todo.Order
		}
	}
	return maxOrder + 1
}

func (m model) nextTodoGlobalOrder() int {
	maxOrder := 0
	for _, todo := range m.data.Todos {
		if todo.GlobalOrder > maxOrder {
			maxOrder = todo.GlobalOrder
		}
	}
	return maxOrder + 1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
