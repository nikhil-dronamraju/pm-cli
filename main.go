package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
}

type goal struct {
	ID           int    `json:"id"`
	MilestoneID  int    `json:"milestone_id"`
	ParentGoalID int    `json:"parent_goal_id"`
	Name         string `json:"name"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
}

type todo struct {
	ID        int    `json:"id"`
	GoalID    int    `json:"goal_id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
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
	paneContent
)

type itemKind int

const (
	itemGoal itemKind = iota
	itemTodo
)

type focusItem struct {
	kind itemKind
	id   int
}

type formMode int

const (
	formNone formMode = iota
	formAddMilestone
	formAddGoal
	formAddSubgoal
	formAddTodo
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

type model struct {
	data         plannerData
	dataPath     string
	width        int
	height       int
	activePane   pane
	milestoneIdx int
	contentIdx   int
	activeGoalID int
	form         formState
	timer        pomodoroState
	status       string
	showHelp     bool
	quitting     bool
}

var (
	bodyColor      = lipgloss.AdaptiveColor{Light: "0", Dark: "252"}
	mutedColor     = lipgloss.AdaptiveColor{Light: "8", Dark: "245"}
	borderColor    = lipgloss.AdaptiveColor{Light: "246", Dark: "240"}
	accentBg       = lipgloss.AdaptiveColor{Light: "62", Dark: "69"}
	accentFg       = lipgloss.AdaptiveColor{Light: "255", Dark: "255"}
	successColor   = lipgloss.AdaptiveColor{Light: "28", Dark: "78"}
	appStyle       = lipgloss.NewStyle().Padding(1, 2).Foreground(bodyColor)
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(bodyColor)
	mutedStyle     = lipgloss.NewStyle().Foreground(mutedColor)
	highlightStyle = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true).Padding(0, 1)
	panelStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Foreground(bodyColor).Padding(1)
	activeRowStyle = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true).Padding(0, 1)
	formStyle      = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentBg).Foreground(bodyColor).Padding(1)
	successStyle   = lipgloss.NewStyle().Foreground(successColor).Bold(true)
)

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
		data:         data,
		dataPath:     dataPath,
		activePane:   paneSidebar,
		milestoneIdx: 0,
		contentIdx:   0,
		timer: pomodoroState{
			phase:     phaseWork,
			remaining: workDuration,
		},
		status: "hjkl move • enter/l open • h/esc back • a add in context • t add todo • s add subgoal • p start/pause timer • n next phase • r reset timer • ? help",
	}
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
	case "n":
		m.advanceTimer()
		m.status = successStyle.Render("Pomodoro phase advanced.")
		if m.timer.running {
			return m, timerTick()
		}
		return m, nil
	case "tab":
		if m.activePane == paneSidebar {
			m.activePane = paneContent
		} else {
			m.activePane = paneSidebar
		}
		return m, nil
	case "g", "home":
		if m.activePane == paneSidebar {
			m.milestoneIdx = 0
			m.activeGoalID = 0
			m.contentIdx = 0
		} else {
			m.contentIdx = 0
		}
		return m, nil
	case "G", "end":
		if m.activePane == paneSidebar {
			if len(m.data.Milestones) > 0 {
				m.milestoneIdx = len(m.data.Milestones) - 1
				m.activeGoalID = 0
				m.contentIdx = 0
			}
		} else {
			items := m.currentItems()
			if len(items) > 0 {
				m.contentIdx = len(items) - 1
			}
		}
		return m, nil
	case "up", "k":
		if m.activePane == paneSidebar {
			if m.milestoneIdx > 0 {
				m.milestoneIdx--
				m.activeGoalID = 0
				m.contentIdx = 0
			}
		} else if m.contentIdx > 0 {
			m.contentIdx--
		}
		return m, nil
	case "down", "j":
		if m.activePane == paneSidebar {
			if m.milestoneIdx < len(m.data.Milestones)-1 {
				m.milestoneIdx++
				m.activeGoalID = 0
				m.contentIdx = 0
			}
		} else if m.contentIdx < len(m.currentItems())-1 {
			m.contentIdx++
		}
		return m, nil
	case "enter", "right", "l":
		if m.activePane == paneSidebar {
			m.activePane = paneContent
			return m, nil
		}

		items := m.currentItems()
		if len(items) == 0 || m.contentIdx >= len(items) {
			return m, nil
		}
		item := items[m.contentIdx]
		if item.kind == itemGoal {
			m.activeGoalID = item.id
			m.contentIdx = 0
		}
		return m, nil
	case "esc", "left", "h", "backspace":
		if m.activeGoalID != 0 {
			parent := m.parentGoalID(m.activeGoalID)
			m.activeGoalID = parent
			m.contentIdx = 0
		} else {
			m.activePane = paneSidebar
		}
		return m, nil
	case "m":
		m.startForm(formAddMilestone, 0)
		return m, textinput.Blink
	case "a":
		if m.activeMilestone() == nil {
			m.startForm(formAddMilestone, 0)
		} else if m.activeGoal() == nil {
			m.startForm(formAddGoal, 0)
		} else {
			m.startForm(formAddTodo, 0)
		}
		return m, textinput.Blink
	case "s":
		if m.activeGoal() != nil {
			m.startForm(formAddSubgoal, 0)
			return m, textinput.Blink
		}
	case "t":
		if m.activeGoal() != nil {
			m.startForm(formAddTodo, 0)
			return m, textinput.Blink
		}
	case "e":
		targetMode, targetID := m.editTarget()
		if targetMode != formNone {
			m.startForm(targetMode, targetID)
			return m, textinput.Blink
		}
	case "d", "x":
		if err := m.deleteTarget(); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
		}
		m.normalize()
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
		if m.form.index < len(m.form.inputs)-1 && m.requiresDetailInputs() {
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

func (m model) View() string {
	if m.quitting {
		return ""
	}

	header := m.renderHeader()
	sidebar := m.renderSidebar()
	focus := m.renderFocus()
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, focus)

	parts := []string{header, body, mutedStyle.Render(m.status)}
	if m.showHelp {
		parts = append(parts, m.renderHelp())
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
		mutedStyle.Render("Drill into one branch at a time. Add in context. Edit only when needed."),
		stats,
		mutedStyle.Render(fmt.Sprintf("Pomodoro: %s", m.timerSummary())),
		mutedStyle.Render("Todo flow: drill into a goal, then press t for a todo or a for the default action."),
	}

	return panelStyle.Width(max(40, m.width-6)).Render(strings.Join(lines, "\n"))
}

func (m model) renderSidebar() string {
	lines := []string{headerStyle.Render("Milestones")}
	if len(m.data.Milestones) == 0 {
		lines = append(lines, mutedStyle.Render("No milestones yet. Press m to add one."))
		return panelStyle.Width(max(24, min(30, m.width/4))).Render(strings.Join(lines, "\n"))
	}

	for i, milestone := range m.data.Milestones {
		line := fmt.Sprintf("%s\n%s", milestone.Name, mutedStyle.Render(dateRange(milestone.StartDate, milestone.EndDate)))
		if i == m.milestoneIdx {
			line = activeRowStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return panelStyle.Width(max(24, min(30, m.width/4))).Render(strings.Join(lines, "\n\n"))
}

func (m model) renderFocus() string {
	width := max(50, m.width-42)
	activeMilestone := m.activeMilestone()
	if activeMilestone == nil {
		return panelStyle.Width(width).Render(headerStyle.Render("Nothing selected"))
	}

	var lines []string
	lines = append(lines, headerStyle.Render(m.focusTitle()))
	lines = append(lines, mutedStyle.Render(strings.Join(m.breadcrumbs(), " / ")))
	lines = append(lines, mutedStyle.Render(strings.Join(m.focusMeta(), " • ")))

	items := m.currentItems()
	if len(items) == 0 {
		lines = append(lines, "")
		lines = append(lines, mutedStyle.Render("Nothing in this branch yet. Press a to add in context."))
	} else {
		lines = append(lines, "")
		lines = append(lines, m.renderItems(items))
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render(m.contextHint()))

	return panelStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderItems(items []focusItem) string {
	var lines []string
	if m.activeGoal() != nil {
		lines = append(lines, mutedStyle.Render("Subgoals and todos"))
	}

	for i, item := range items {
		line := m.renderItem(item)
		if m.activePane == paneContent && i == m.contentIdx {
			line = activeRowStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m model) renderItem(item focusItem) string {
	switch item.kind {
	case itemGoal:
		goal := m.mustGoal(item.id)
		return fmt.Sprintf("%s %s\n%s", highlightStyle.Render("G"), goal.Name, mutedStyle.Render(dateRange(goal.StartDate, goal.EndDate)))
	case itemTodo:
		todo := m.mustTodo(item.id)
		return fmt.Sprintf("%s %s\n%s", highlightStyle.Render("T"), todo.Name, mutedStyle.Render(dateRange(todo.StartDate, todo.EndDate)))
	default:
		return ""
	}
}

func (m model) renderHelp() string {
	help := []string{
		"q quit",
		"tab switch pane",
		"j/k or up/down move",
		"g/G jump to first or last item",
		"enter or l open selected goal",
		"h, left, esc, or backspace go back",
		"m add milestone",
		"a add in current context",
		"t add todo in goal view",
		"s add subgoal in goal view",
		"e edit current item",
		"d delete current item",
		"p or space start/pause pomodoro",
		"r reset pomodoro to current phase default",
		"n skip to next pomodoro phase",
	}
	return panelStyle.Render(strings.Join(help, "\n"))
}

func (m model) renderForm() string {
	title := map[formMode]string{
		formAddMilestone:  "Add Milestone",
		formAddGoal:       "Add Goal",
		formAddSubgoal:    "Add Subgoal",
		formAddTodo:       "Add Todo",
		formEditMilestone: "Edit Milestone",
		formEditGoal:      "Edit Goal",
		formEditTodo:      "Edit Todo",
	}[m.form.mode]

	lines := []string{headerStyle.Render(title)}
	for i, input := range m.form.inputs {
		label := "Name"
		if i == 1 {
			label = "Start date"
		} else if i == 2 {
			label = "End date"
		}
		lines = append(lines, fmt.Sprintf("%s\n%s", mutedStyle.Render(label), input.View()))
	}
	lines = append(lines, mutedStyle.Render("enter submit • tab move • esc cancel"))
	return formStyle.Width(max(42, m.width/2)).Render(strings.Join(lines, "\n\n"))
}

func (m *model) startForm(mode formMode, target int) {
	count := 1
	if mode == formEditMilestone || mode == formEditGoal || mode == formEditTodo {
		count = 3
	}

	inputs := make([]textinput.Model, count)
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].Width = 36
	}

	switch mode {
	case formAddMilestone:
		inputs[0].Placeholder = "Milestone name"
	case formAddGoal, formAddSubgoal:
		inputs[0].Placeholder = "Goal name"
	case formAddTodo:
		inputs[0].Placeholder = "Todo name"
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

func (m model) requiresDetailInputs() bool {
	return m.form.mode == formEditMilestone || m.form.mode == formEditGoal || m.form.mode == formEditTodo
}

func (m *model) submitForm() error {
	name := strings.TrimSpace(m.form.inputs[0].Value())
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	switch m.form.mode {
	case formAddMilestone:
		m.data.Milestones = append(m.data.Milestones, milestone{ID: m.nextID(), Name: name})
		slices.SortFunc(m.data.Milestones, func(a, b milestone) int { return strings.Compare(a.Name, b.Name) })
		m.status = successStyle.Render("Milestone added.")
	case formAddGoal:
		activeMilestone := m.activeMilestone()
		if activeMilestone == nil {
			return fmt.Errorf("no milestone selected")
		}
		m.data.Goals = append(m.data.Goals, goal{ID: m.nextID(), MilestoneID: activeMilestone.ID, Name: name})
		m.status = successStyle.Render("Goal added.")
	case formAddSubgoal:
		activeGoal := m.activeGoal()
		if activeGoal == nil {
			return fmt.Errorf("no goal selected")
		}
		m.data.Goals = append(m.data.Goals, goal{
			ID:           m.nextID(),
			MilestoneID:  activeGoal.MilestoneID,
			ParentGoalID: activeGoal.ID,
			Name:         name,
		})
		m.status = successStyle.Render("Subgoal added.")
	case formAddTodo:
		activeGoal := m.activeGoal()
		if activeGoal == nil {
			return fmt.Errorf("no goal selected")
		}
		m.data.Todos = append(m.data.Todos, todo{ID: m.nextID(), GoalID: activeGoal.ID, Name: name})
		m.status = successStyle.Render("Todo added.")
	case formEditMilestone:
		item := m.mustMilestonePtr(m.form.target)
		item.Name = name
		item.StartDate = strings.TrimSpace(m.form.inputs[1].Value())
		item.EndDate = strings.TrimSpace(m.form.inputs[2].Value())
		m.status = successStyle.Render("Milestone updated.")
	case formEditGoal:
		item := m.mustGoalPtr(m.form.target)
		item.Name = name
		item.StartDate = strings.TrimSpace(m.form.inputs[1].Value())
		item.EndDate = strings.TrimSpace(m.form.inputs[2].Value())
		m.status = successStyle.Render("Goal updated.")
	case formEditTodo:
		item := m.mustTodoPtr(m.form.target)
		item.Name = name
		item.StartDate = strings.TrimSpace(m.form.inputs[1].Value())
		item.EndDate = strings.TrimSpace(m.form.inputs[2].Value())
		m.status = successStyle.Render("Todo updated.")
	}

	return nil
}

func (m model) editTarget() (formMode, int) {
	items := m.currentItems()
	if m.activePane == paneContent && len(items) > 0 && m.contentIdx < len(items) {
		item := items[m.contentIdx]
		if item.kind == itemGoal {
			return formEditGoal, item.id
		}
		return formEditTodo, item.id
	}

	if activeGoal := m.activeGoal(); activeGoal != nil {
		return formEditGoal, activeGoal.ID
	}
	if activeMilestone := m.activeMilestone(); activeMilestone != nil {
		return formEditMilestone, activeMilestone.ID
	}

	return formNone, 0
}

func (m *model) deleteTarget() error {
	items := m.currentItems()
	if m.activePane == paneContent && len(items) > 0 && m.contentIdx < len(items) {
		item := items[m.contentIdx]
		switch item.kind {
		case itemGoal:
			m.deleteGoal(item.id)
			m.status = successStyle.Render("Goal deleted.")
		case itemTodo:
			m.data.Todos = deleteByID(m.data.Todos, item.id, func(t todo) int { return t.ID })
			m.status = successStyle.Render("Todo deleted.")
		}
		return nil
	}

	if activeGoal := m.activeGoal(); activeGoal != nil {
		parent := activeGoal.ParentGoalID
		m.deleteGoal(activeGoal.ID)
		m.activeGoalID = parent
		m.status = successStyle.Render("Goal deleted.")
		return nil
	}

	if activeMilestone := m.activeMilestone(); activeMilestone != nil {
		m.deleteMilestone(activeMilestone.ID)
		m.activeGoalID = 0
		m.status = successStyle.Render("Milestone deleted.")
		return nil
	}

	return fmt.Errorf("nothing to delete")
}

func (m *model) deleteMilestone(id int) {
	goalIDs := make(map[int]struct{})
	for _, g := range m.data.Goals {
		if g.MilestoneID == id {
			goalIDs[g.ID] = struct{}{}
		}
	}

	m.data.Milestones = deleteByID(m.data.Milestones, id, func(item milestone) int { return item.ID })
	m.data.Goals = slices.DeleteFunc(m.data.Goals, func(item goal) bool {
		return item.MilestoneID == id
	})
	m.data.Todos = slices.DeleteFunc(m.data.Todos, func(item todo) bool {
		_, ok := goalIDs[item.GoalID]
		return ok
	})
}

func (m *model) deleteGoal(id int) {
	children := []int{id}
	for {
		size := len(children)
		for _, g := range m.data.Goals {
			if g.ParentGoalID != 0 && slices.Contains(children, g.ParentGoalID) && !slices.Contains(children, g.ID) {
				children = append(children, g.ID)
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

func (m *model) normalize() {
	if len(m.data.Milestones) == 0 {
		m.milestoneIdx = 0
		m.activeGoalID = 0
		m.contentIdx = 0
		return
	}

	if m.milestoneIdx >= len(m.data.Milestones) {
		m.milestoneIdx = len(m.data.Milestones) - 1
	}
	if m.milestoneIdx < 0 {
		m.milestoneIdx = 0
	}

	activeMilestone := m.activeMilestone()
	if activeMilestone == nil {
		m.activeGoalID = 0
		m.contentIdx = 0
		return
	}

	if m.activeGoalID != 0 {
		goal := m.findGoal(m.activeGoalID)
		if goal == nil || goal.MilestoneID != activeMilestone.ID {
			m.activeGoalID = 0
		}
	}

	items := m.currentItems()
	if len(items) == 0 {
		m.contentIdx = 0
		return
	}

	if m.contentIdx >= len(items) {
		m.contentIdx = len(items) - 1
	}
	if m.contentIdx < 0 {
		m.contentIdx = 0
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

func (m *model) nextID() int {
	id := m.data.NextID
	m.data.NextID++
	return id
}

func (m model) activeMilestone() *milestone {
	if len(m.data.Milestones) == 0 || m.milestoneIdx >= len(m.data.Milestones) {
		return nil
	}
	return &m.data.Milestones[m.milestoneIdx]
}

func (m model) activeGoal() *goal {
	if m.activeGoalID == 0 {
		return nil
	}
	return m.findGoal(m.activeGoalID)
}

func (m model) currentItems() []focusItem {
	activeMilestone := m.activeMilestone()
	if activeMilestone == nil {
		return nil
	}

	items := []focusItem{}
	if activeGoal := m.activeGoal(); activeGoal != nil {
		for _, g := range m.data.Goals {
			if g.ParentGoalID == activeGoal.ID {
				items = append(items, focusItem{kind: itemGoal, id: g.ID})
			}
		}
		for _, t := range m.data.Todos {
			if t.GoalID == activeGoal.ID {
				items = append(items, focusItem{kind: itemTodo, id: t.ID})
			}
		}
		return items
	}

	for _, g := range m.data.Goals {
		if g.MilestoneID == activeMilestone.ID && g.ParentGoalID == 0 {
			items = append(items, focusItem{kind: itemGoal, id: g.ID})
		}
	}
	return items
}

func (m model) focusTitle() string {
	if activeGoal := m.activeGoal(); activeGoal != nil {
		return activeGoal.Name
	}
	if activeMilestone := m.activeMilestone(); activeMilestone != nil {
		return activeMilestone.Name
	}
	return "Planner"
}

func (m model) breadcrumbs() []string {
	parts := []string{"Milestones"}
	if activeMilestone := m.activeMilestone(); activeMilestone != nil {
		parts = append(parts, activeMilestone.Name)
	}
	if activeGoal := m.activeGoal(); activeGoal != nil {
		chain := []string{activeGoal.Name}
		parent := activeGoal.ParentGoalID
		for parent != 0 {
			next := m.findGoal(parent)
			if next == nil {
				break
			}
			chain = append([]string{next.Name}, chain...)
			parent = next.ParentGoalID
		}
		parts = append(parts[:2], chain...)
	}
	return parts
}

func (m model) focusMeta() []string {
	if activeGoal := m.activeGoal(); activeGoal != nil {
		return []string{
			dateRange(activeGoal.StartDate, activeGoal.EndDate),
			fmt.Sprintf("%d subgoals", m.countChildGoals(activeGoal.ID)),
			fmt.Sprintf("%d todos", m.countTodos(activeGoal.ID)),
		}
	}

	if activeMilestone := m.activeMilestone(); activeMilestone != nil {
		return []string{
			dateRange(activeMilestone.StartDate, activeMilestone.EndDate),
			fmt.Sprintf("%d goals", m.countMilestoneGoals(activeMilestone.ID)),
			fmt.Sprintf("%d todos", m.countMilestoneTodos(activeMilestone.ID)),
		}
	}

	return nil
}

func (m model) contextHint() string {
	if m.activeGoal() != nil {
		return "t add todo • s add subgoal • a default add • e edit focused item • d delete • h back • p timer"
	}
	if m.activeMilestone() != nil {
		return "a add goal • e edit milestone or selected goal • d delete • l open selected goal • p timer"
	}
	return "m add milestone • p timer"
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
	goalIDs := map[int]struct{}{}
	for _, goal := range m.data.Goals {
		if goal.MilestoneID == id {
			goalIDs[goal.ID] = struct{}{}
		}
	}
	count := 0
	for _, todo := range m.data.Todos {
		if _, ok := goalIDs[todo.GoalID]; ok {
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

func (m model) countTodos(goalID int) int {
	count := 0
	for _, todo := range m.data.Todos {
		if todo.GoalID == goalID {
			count++
		}
	}
	return count
}

func (m model) parentGoalID(goalID int) int {
	goal := m.findGoal(goalID)
	if goal == nil {
		return 0
	}
	return goal.ParentGoalID
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
