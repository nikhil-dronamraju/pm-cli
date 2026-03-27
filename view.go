package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
		"%s  %s  %s  %s  %s  %s",
		highlightStyle.Render(fmt.Sprintf("%d milestones", len(m.data.Milestones))),
		highlightStyle.Render(fmt.Sprintf("%d goals", len(m.data.Goals))),
		highlightStyle.Render(fmt.Sprintf("%d active", len(m.allTodos()))),
		highlightStyle.Render(fmt.Sprintf("%d archived", m.completedTodoCount())),
		highlightStyle.Render(fmt.Sprintf("%d in progress", m.inProgressTodoCount())),
		highlightStyle.Render(m.timerBadge()),
	)

	lines := []string{
		titleStyle.Render("Planner"),
		mutedStyle.Render("Active work stays in focus. Completed tasks live in Archive."),
		stats,
		mutedStyle.Render(fmt.Sprintf("Pomodoro %s", m.timerSummary())),
	}
	if m.grab.active {
		lines = append(lines, warnStyle.Render("Grab mode is active. Press enter to drop or h to cancel."))
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
	if m.screen.kind == screenAnalytics {
		return panelFrame(active).Width(width).Render(strings.Join(m.renderAnalyticsList(width), "\n"))
	}

	lines := []string{panelHeading(m.screenTitle(), active)}
	if subtitle := m.screenSubtitle(); subtitle != "" {
		lines = append(lines, mutedStyle.Render(subtitle))
	}

	items := m.visibleItems()
	if len(items) == 0 {
		message := "Nothing here yet. Press n to add a task."
		if m.screen.kind == screenCompleted {
			message = "No completed tasks yet."
		}
		lines = append(lines, "", mutedStyle.Render(message))
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
				continue
			}

			line := m.renderItem(item)
			if m.grab.active && m.grab.item.kind == item.kind && m.grab.item.id == item.id {
				line = warnStyle.Render("GRAB ") + line
			}
			if i == m.listIdx {
				lines = append(lines, inactiveRowStyle.Render(line))
			} else {
				lines = append(lines, line)
			}
		}
	}

	lines = append(lines, "", mutedStyle.Render(m.contextHint()))
	return panelFrame(active).Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderDetail() string {
	sidebarWidth := max(24, min(30, m.width/5))
	listWidth := max(42, m.width/2-10)
	width := max(34, m.width-sidebarWidth-listWidth-12)

	if m.screen.kind == screenAnalytics {
		return panelStyle.Width(width).Render(strings.Join(m.renderAnalyticsDetail(width), "\n"))
	}

	lines := []string{sectionStyle.Render("Details")}
	if item, ok := m.selectedItem(); ok {
		lines = append(lines, m.renderSelectionDetail(item)...)
	} else if m.screen.kind == screenMilestone {
		lines = append(lines, m.renderMilestoneDetail(m.mustMilestone(m.screen.milestoneID))...)
	} else {
		if m.screen.kind == screenCompleted {
			lines = append(lines, mutedStyle.Render("Select a completed task to inspect or reopen it."))
		} else {
			lines = append(lines, mutedStyle.Render("Select a task or goal to inspect and edit it."))
		}
	}

	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("Shortcuts"))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s add a task to %s", keyStyle.Render("n"), m.quickAddBrowseDestinationLabel())))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s search or create a task", keyStyle.Render("/"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s move the selection", keyStyle.Render("m"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s mark in progress", keyStyle.Render("t"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s complete or reopen", keyStyle.Render("c"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s open Archive", keyStyle.Render("C"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s open Analytics", keyStyle.Render("y"))))
	return panelStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderMilestoneDetail(item milestone) []string {
	lines := []string{
		headerStyle.Render(item.Name),
		mutedStyle.Render(fmt.Sprintf("Milestone • %s", dateRange(item.StartDate, item.EndDate))),
	}
	meta := []string{
		milestoneCompletionLabel(item),
		fmt.Sprintf("%d top-level goals", m.countTopLevelGoals(item.ID)),
		fmt.Sprintf("%d active", m.countMilestoneOpenTodos(item.ID)),
		fmt.Sprintf("%d done", m.countMilestoneCompletedTodos(item.ID)),
	}
	lines = append(lines, strings.Join(meta, " • "))
	return lines
}

func (m model) renderSelectionDetail(item focusItem) []string {
	switch item.kind {
	case itemGoal:
		goal := m.mustGoal(item.id)
		lines := []string{
			headerStyle.Render(goal.Name),
			mutedStyle.Render(fmt.Sprintf("Goal • %s", dateRange(goal.StartDate, goal.EndDate))),
			mutedStyle.Render(strings.Join(m.goalPath(goal), " / ")),
		}
		meta := append(
			m.priorityMeta(goal.Important, goal.Urgent),
			goalCompletionLabel(goal),
			fmt.Sprintf("%d subgoals", m.countChildGoals(goal.ID)),
			fmt.Sprintf("%d active", m.countGoalOpenTodos(goal.ID)),
			fmt.Sprintf("%d done", m.countGoalCompletedTodos(goal.ID)),
		)
		lines = append(lines, strings.Join(meta, " • "))
		return lines
	case itemTodo:
		todo := m.mustTodo(item.id)
		lines := []string{
			headerStyle.Render(todo.Name),
			mutedStyle.Render(fmt.Sprintf("Task • %s", dateRange(todo.StartDate, todo.EndDate))),
			mutedStyle.Render(m.todoContext(todo)),
		}
		meta := m.priorityMeta(todo.Important, todo.Urgent)
		if len(meta) == 0 {
			meta = []string{"normal priority"}
		}
		meta = append(meta, m.todoCompletionLabel(todo))
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
		meta := fmt.Sprintf("%s • %d subgoals • %d active • %d done", goalCompletionLabel(goal), m.countChildGoals(goal.ID), m.countGoalOpenTodos(goal.ID), m.countGoalCompletedTodos(goal.ID))
		line := fmt.Sprintf("%s %s%s\n%s", highlightStyle.Render("G"), goal.Name, m.prioritySuffix(goal.Important, goal.Urgent), mutedStyle.Render(meta))
		if goal.Completed {
			return completedTodoStyle.Render(line)
		}
		return line
	case itemTodo:
		todo := m.mustTodo(item.id)
		line := fmt.Sprintf("%s %s%s\n%s", m.todoCheckbox(todo), todo.Name, m.prioritySuffix(todo.Important, todo.Urgent), mutedStyle.Render(fmt.Sprintf("%s • %s • %s", m.todoContext(todo), dateRange(todo.StartDate, todo.EndDate), m.todoCompletionLabel(todo))))
		if todoIsCompleted(todo) {
			return completedTodoStyle.Render(line)
		}
		if todoIsInProgress(todo) {
			return inProgressTodoStyle.Render(line)
		}
		return line
	default:
		return ""
	}
}

func (m model) renderActiveItem(item focusItem) string {
	switch item.kind {
	case itemGoal:
		goal := m.mustGoal(item.id)
		suffix := strings.Join(m.priorityMeta(goal.Important, goal.Urgent), " ")
		if suffix != "" {
			suffix = " " + suffix
		}
		return fmt.Sprintf("G %s%s\n%s • %d subgoals • %d active • %d done", goal.Name, suffix, goalCompletionLabel(goal), m.countChildGoals(goal.ID), m.countGoalOpenTodos(goal.ID), m.countGoalCompletedTodos(goal.ID))
	case itemTodo:
		todo := m.mustTodo(item.id)
		suffix := strings.Join(m.priorityMeta(todo.Important, todo.Urgent), " ")
		if suffix != "" {
			suffix = " " + suffix
		}
		return fmt.Sprintf("%s %s%s\n%s • %s • %s", m.todoCheckbox(todo), todo.Name, suffix, m.todoContext(todo), dateRange(todo.StartDate, todo.EndDate), m.todoCompletionLabel(todo))
	default:
		return ""
	}
}

func (m model) renderHelp() string {
	help := []string{
		"n or a quick add task",
		"/ search to jump, or create a task from the query",
		"m move the selected goal or task without navigating first",
		"t toggle a task in progress",
		"v start grab mode, move cursor, enter to drop",
		"c toggle milestone, goal, or task completion and stamp today",
		"i open Inbox",
		"A open Active Tasks",
		"C open Archive",
		"y open Analytics",
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
			label = "Task"
		} else if i == 1 {
			label = "Start date"
		} else if i == 2 {
			label = "End date"
		}
		lines = append(lines, fmt.Sprintf("%s\n%s", mutedStyle.Render(label), input.View()))
	}
	if m.form.mode == formQuickAdd {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("Creates a task in %s.", m.quickAddDestinationLabel())))
	}
	lines = append(lines, mutedStyle.Render("enter submit • tab move • esc cancel"))
	return formStyle.Width(max(46, m.width/2)).Render(strings.Join(lines, "\n\n"))
}
