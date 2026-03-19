package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

const (
	dataFileName        = "planner.json"
	fixedSidebarEntries = 3
)

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
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at"`
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
	screenAnalytics
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

type analyticsSeries struct {
	Label  string
	Counts map[string]int
	Total  int
}

type analyticsGroup struct {
	Label  string
	Counts map[string]int
	Total  int
	Goals  []analyticsSeries
}

var (
	bodyColor          = lipgloss.AdaptiveColor{Light: "0", Dark: "255"}
	mutedColor         = lipgloss.AdaptiveColor{Light: "8", Dark: "250"}
	borderColor        = lipgloss.AdaptiveColor{Light: "246", Dark: "244"}
	accentColor        = lipgloss.AdaptiveColor{Light: "25", Dark: "45"}
	accentBg           = lipgloss.AdaptiveColor{Light: "25", Dark: "31"}
	accentFg           = lipgloss.AdaptiveColor{Light: "255", Dark: "255"}
	successColor       = lipgloss.AdaptiveColor{Light: "28", Dark: "78"}
	warnColor          = lipgloss.AdaptiveColor{Light: "166", Dark: "214"}
	appStyle           = lipgloss.NewStyle().Padding(1, 2).Foreground(bodyColor)
	headerStyle        = lipgloss.NewStyle().Bold(true).Foreground(bodyColor)
	mutedStyle         = lipgloss.NewStyle().Foreground(mutedColor)
	highlightStyle     = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	activeBadgeStyle   = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true).Padding(0, 1)
	panelStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Foreground(bodyColor).Padding(1)
	activeRowStyle     = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true)
	inactiveRowStyle   = lipgloss.NewStyle().Foreground(bodyColor).BorderLeft(true).BorderForeground(accentColor).PaddingLeft(1)
	formStyle          = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentBg).Foreground(bodyColor).Padding(1)
	successStyle       = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	warnStyle          = lipgloss.NewStyle().Foreground(warnColor).Bold(true)
	completedTodoStyle = lipgloss.NewStyle().Foreground(mutedColor)
)
