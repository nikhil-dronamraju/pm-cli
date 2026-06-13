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
	ID          int    `json:"id"`
	Name        string `json:"name"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Order       int    `json:"order"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at"`
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
	Completed    bool   `json:"completed"`
	CompletedAt  string `json:"completed_at"`
}

type todo struct {
	ID               int    `json:"id"`
	MilestoneID      int    `json:"milestone_id"`
	GoalID           int    `json:"goal_id"`
	Name             string `json:"name"`
	StartDate        string `json:"start_date"`
	EndDate          string `json:"end_date"`
	Order            int    `json:"order"`
	GlobalOrder      int    `json:"global_order"`
	Important        bool   `json:"important"`
	Urgent           bool   `json:"urgent"`
	Status           string `json:"status,omitempty"`
	Completed        bool   `json:"completed"`
	CompletedAt      string `json:"completed_at"`
	ArchiveMilestone string `json:"archive_milestone,omitempty"`
	ArchiveGoalPath  string `json:"archive_goal_path,omitempty"`
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
	paneDetail
)

type screenKind int

const (
	screenAll screenKind = iota
	screenCompleted
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
	mode              formMode
	target            int
	targetGoalID      int
	targetMilestoneID int
	inputs            []textinput.Model
	index             int
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

type tickMsg struct {
	at      time.Time
	version int
}

type pomodoroState struct {
	phase           pomodoroPhase
	running         bool
	remaining       time.Duration
	workAccumulated time.Duration
	version         int
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

	sidebarIdx   int
	listIdx      int
	listScroll   int
	detailScroll int

	form   formState
	search searchState
	grab   grabState
	timer  pomodoroState

	status   string
	undo     []undoState
	showHelp bool
	quitting bool
}

type undoState struct {
	data         plannerData
	screen       screenState
	screenBack   []screenState
	sidebarIdx   int
	listIdx      int
	listScroll   int
	detailScroll int
	activePane   pane
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
	bodyColor           lipgloss.TerminalColor = lipgloss.NoColor{}
	mutedColor                                 = lipgloss.AdaptiveColor{Light: "#7C6F64", Dark: "#A89984"}
	borderColor                                = lipgloss.AdaptiveColor{Light: "#928374", Dark: "#665C54"}
	accentColor                                = lipgloss.AdaptiveColor{Light: "#076678", Dark: "#FABD2F"}
	accentBg                                   = lipgloss.AdaptiveColor{Light: "#076678", Dark: "#D79921"}
	accentFg                                   = lipgloss.AdaptiveColor{Light: "#FBF1C7", Dark: "#1D2021"}
	successColor                               = lipgloss.AdaptiveColor{Light: "#427B58", Dark: "#B8BB26"}
	warnColor                                  = lipgloss.AdaptiveColor{Light: "#AF3A03", Dark: "#FE8019"}
	appStyle                                   = lipgloss.NewStyle().Padding(1, 2).Foreground(bodyColor)
	headerStyle                                = lipgloss.NewStyle().Bold(true).Foreground(bodyColor)
	mutedStyle                                 = lipgloss.NewStyle().Foreground(mutedColor)
	highlightStyle                             = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	activeBadgeStyle                           = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true).Padding(0, 1)
	panelStyle                                 = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Foreground(bodyColor).Padding(1)
	activeRowStyle                             = lipgloss.NewStyle().Foreground(accentFg).Background(accentBg).Bold(true)
	inactiveRowStyle                           = lipgloss.NewStyle().Foreground(bodyColor).BorderLeft(true).BorderForeground(accentColor).PaddingLeft(1)
	formStyle                                  = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accentBg).Foreground(bodyColor).Padding(1)
	successStyle                               = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	warnStyle                                  = lipgloss.NewStyle().Foreground(warnColor).Bold(true)
	inProgressTodoStyle                        = lipgloss.NewStyle().Foreground(warnColor)
	completedTodoStyle                         = lipgloss.NewStyle().Foreground(mutedColor)
	titleStyle                                 = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	sectionStyle                               = lipgloss.NewStyle().Bold(true).Foreground(bodyColor)
	keyStyle                                   = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
)

const (
	todoStatusOpen       = "open"
	todoStatusInProgress = "in_progress"
	todoStatusCompleted  = "completed"
)
