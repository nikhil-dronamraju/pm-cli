package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newModel(data plannerData, dataPath string) *tea.Program {
	m := model{
		data:       data,
		dataPath:   dataPath,
		activePane: paneList,
		screen:     screenState{kind: screenInbox},
		timer: pomodoroState{
			phase:     phaseWork,
			remaining: workDuration,
		},
		status: "n quick add • / jump/create • m move • c complete • y analytics • tab switch panes • ? help",
	}
	m.search.input = textinput.New()
	m.search.input.Width = 42
	m.search.input.Placeholder = "Search tasks, goals, milestones"
	m.normalize()
	return tea.NewProgram(m, tea.WithAltScreen())
}

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
	case "y":
		m.setScreen(screenState{kind: screenAnalytics}, false)
		m.status = successStyle.Render("Analytics ready.")
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
	case "c":
		if err := m.toggleCompletion(); err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := m.save(); err != nil {
			m.status = fmt.Sprintf("save failed: %v", err)
			return m, nil
		}
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
