package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type bodyLayout struct {
	sidebarWidth int
	listWidth    int
	detailWidth  int
	height       int
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	header := m.renderHeader()
	status := m.renderStatus()
	overlays := []string{}
	if m.showHelp {
		overlays = append(overlays, m.renderHelp())
	}
	if m.search.active {
		overlays = append(overlays, m.renderSearch())
	}
	if m.form.mode != formNone {
		overlays = append(overlays, m.renderForm())
	}

	layout := m.bodyLayout(header, status, overlays)
	sidebar := m.renderSidebar(layout.sidebarWidth, layout.height)
	list := m.renderList(layout.listWidth, layout.height)
	detail := m.renderDetail(layout.detailWidth, layout.height)
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, list, detail)

	parts := append([]string{header, body, status}, overlays...)
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
		mutedStyle.Render("Active tasks and archive"),
		stats,
		mutedStyle.Render(fmt.Sprintf("Pomodoro %s", m.timerSummary())),
	}
	if m.grab.active {
		lines = append(lines, warnStyle.Render("Grab mode is active. Press enter to drop or h to cancel."))
	}
	return panelStyle.Width(max(48, m.innerWidth()-2)).Render(strings.Join(lines, "\n"))
}

func (m model) renderStatus() string {
	return mutedStyle.Width(max(20, m.innerWidth())).Render(m.status)
}

func (m model) renderSidebar(width, height int) string {
	active := m.activePane == paneSidebar
	lines := []string{panelHeading("Spaces", active)}
	entries := m.sidebarEntries()
	selectedLine := -1
	for i, entry := range entries {
		lines = append(lines, "")
		line := entry.label
		if entry.meta != "" {
			meta := entry.meta
			if i != m.sidebarIdx || m.activePane != paneSidebar {
				meta = mutedStyle.Render(meta)
			}
			line = fmt.Sprintf("%s\n%s", line, meta)
		}
		if i == m.sidebarIdx && m.activePane == paneSidebar {
			line = activeRowStyle.Render(line)
		} else if i == m.sidebarIdx {
			line = inactiveRowStyle.Render(line)
		}
		if i == m.sidebarIdx {
			selectedLine = len(lines)
		}
		lines = appendRenderedLines(lines, line)
	}
	visible := viewportLines(lines, 0, selectedLine, paneContentHeight(height), width)
	return renderPane(active, width, height, visible)
}

func (m model) renderList(width, height int) string {
	active := m.activePane == paneList
	if m.screen.kind == screenAnalytics {
		lines := flattenChunks(m.renderAnalyticsList(width))
		visible := viewportLines(lines, m.listScroll, -1, paneContentHeight(height), width)
		return renderPane(active, width, height, visible)
	}

	lines := []string{panelHeading(m.screenTitle(), active)}
	if subtitle := m.screenSubtitle(); subtitle != "" {
		lines = append(lines, mutedStyle.Render(subtitle))
	}

	items := m.visibleItems()
	selectedLine := -1
	if len(items) == 0 {
		message := "Nothing here yet. Press n to add a task."
		if m.screen.kind == screenCompleted {
			message = "No completed tasks yet."
		}
		lines = append(lines, "", mutedStyle.Render(message))
	} else {
		lines = append(lines, "")
		for i, item := range items {
			if i == m.listIdx {
				selectedLine = len(lines)
			}
			if i == m.listIdx && m.activePane == paneList {
				line := m.renderActiveItem(item)
				if m.grab.active && m.grab.item.kind == item.kind && m.grab.item.id == item.id {
					line = activeRowStyle.Render("GRAB " + line)
				} else {
					line = activeRowStyle.Render(line)
				}
				lines = appendRenderedLines(lines, line)
				continue
			}

			line := m.renderItem(item)
			if m.grab.active && m.grab.item.kind == item.kind && m.grab.item.id == item.id {
				line = warnStyle.Render("GRAB ") + line
			}
			if i == m.listIdx {
				lines = appendRenderedLines(lines, inactiveRowStyle.Render(line))
			} else {
				lines = appendRenderedLines(lines, line)
			}
		}
	}

	lines = append(lines, "", mutedStyle.Render(m.contextHint()))
	visible := viewportLines(lines, 0, selectedLine, paneContentHeight(height), width)
	return renderPane(active, width, height, visible)
}

func (m model) renderDetail(width, height int) string {
	active := m.activePane == paneDetail
	lines := m.detailContentLines(width)
	visible := viewportLines(lines, m.detailScroll, -1, paneContentHeight(height), width)
	return renderPane(active, width, height, visible)
}

func (m model) detailContentLines(width int) []string {
	if m.screen.kind == screenAnalytics {
		return flattenChunks(m.renderAnalyticsDetail(width))
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
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s add a task to %s", keyStyle.Render("+"), m.quickAddBrowseDestinationLabel())))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s search or create a task", keyStyle.Render("/"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s move the selection", keyStyle.Render("m"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s mark in progress", keyStyle.Render("t/T"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s toggle important", keyStyle.Render("i/I"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s toggle urgent", keyStyle.Render("u/U"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s complete or reopen", keyStyle.Render("c"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s advance timer phase", keyStyle.Render("n/N"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s undo last change", keyStyle.Render("z"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s open Archive", keyStyle.Render("C"))))
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s open Analytics", keyStyle.Render("y/Y"))))
	return flattenChunks(lines)
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
		"+ quick add task",
		"/ search to jump, or create a task from the query",
		"m move the selected goal or task without navigating first",
		"t/T toggle a task in progress",
		"v start grab mode, move cursor, enter to drop",
		"c toggle milestone, goal, or task completion and stamp today",
		"a or A open Active Tasks",
		"C open Archive",
		"y/Y open Analytics",
		"s add goal in milestone or goal views",
		"M add milestone",
		"e edit selected item",
		"x or d delete selected item",
		"z undo the last saved change",
		"i/I toggle important",
		"u/U toggle urgent",
		"S auto-sort current list by urgent/important",
		"tab switch sidebar/list/details",
		"j/k or arrows move selection or scroll the active pane",
		"enter open selected goal",
		"h go back or cancel grab",
		"p or space start/pause pomodoro",
		"r reset pomodoro",
		"n/N advance pomodoro phase",
		"q/Q quit",
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

func (m model) bodyLayout(header, status string, overlays []string) bodyLayout {
	available := max(64, m.innerWidth()-6)
	sidebarWidth := min(28, max(18, available/5))
	detailWidth := min(42, max(24, available/3))
	listWidth := available - sidebarWidth - detailWidth
	if listWidth < 28 {
		needed := 28 - listWidth
		take := min(needed, max(0, detailWidth-22))
		detailWidth -= take
		needed -= take
		take = min(needed, max(0, sidebarWidth-16))
		sidebarWidth -= take
		listWidth = available - sidebarWidth - detailWidth
	}
	if listWidth < 24 {
		listWidth = 24
	}

	return bodyLayout{
		sidebarWidth: sidebarWidth,
		listWidth:    listWidth,
		detailWidth:  detailWidth,
		height:       m.bodyHeight(header, status, overlays),
	}
}

func (m model) bodyHeight(header, status string, overlays []string) int {
	if m.height <= 0 {
		return 24
	}
	used := 2 + lipgloss.Height(header) + lipgloss.Height(status)
	for _, overlay := range overlays {
		used += lipgloss.Height(overlay)
	}
	used += 2 * (2 + len(overlays))
	return max(8, m.height-used)
}

func (m model) innerWidth() int {
	if m.width <= 0 {
		return 120
	}
	return max(60, m.width-4)
}

func paneContentHeight(totalHeight int) int {
	return max(1, totalHeight-4)
}

func renderPane(active bool, width, height int, lines []string) string {
	return panelFrame(active).
		Width(width).
		Height(max(1, height-2)).
		Render(strings.Join(lines, "\n"))
}

func appendRenderedLines(lines []string, chunk string) []string {
	return append(lines, splitLines(chunk)...)
}

func flattenChunks(chunks []string) []string {
	lines := []string{}
	for _, chunk := range chunks {
		lines = appendRenderedLines(lines, chunk)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func splitLines(value string) []string {
	return strings.Split(value, "\n")
}

func viewportLines(lines []string, offset, selectedLine, height, width int) []string {
	if height <= 0 {
		return nil
	}
	lines = truncateLines(lines, max(1, width-2))
	if len(lines) == 0 {
		lines = []string{""}
	}

	windowHeight := height
	showIndicator := len(lines) > height
	if showIndicator && height > 1 {
		windowHeight = height - 1
	}
	if selectedLine >= 0 {
		if selectedLine < offset {
			offset = selectedLine
		}
		if selectedLine >= offset+windowHeight {
			offset = selectedLine - windowHeight + 1
		}
	}
	offset = clampInt(offset, 0, maxScrollOffset(len(lines), height))
	end := min(len(lines), offset+windowHeight)
	visible := append([]string(nil), lines[offset:end]...)
	for len(visible) < windowHeight {
		visible = append(visible, "")
	}
	if showIndicator && height > 1 {
		visible = append(visible, mutedStyle.Render(fmt.Sprintf("Showing %d-%d of %d", offset+1, end, len(lines))))
	}
	for len(visible) < height {
		visible = append(visible, "")
	}
	return visible
}

func truncateLines(lines []string, width int) []string {
	truncated := make([]string, len(lines))
	for i, line := range lines {
		truncated[i] = ansi.Truncate(line, width, "")
	}
	return truncated
}

func maxScrollOffset(lineCount, height int) int {
	if height <= 0 {
		return 0
	}
	windowHeight := height
	if lineCount > height && height > 1 {
		windowHeight = height - 1
	}
	return max(0, lineCount-windowHeight)
}

func clampInt(value, low, high int) int {
	if high < low {
		return low
	}
	return min(max(value, low), high)
}

func (m *model) scrollList(delta int) {
	m.listScroll = clampInt(m.listScroll+delta, 0, m.maxListScroll())
}

func (m *model) scrollDetail(delta int) {
	m.detailScroll = clampInt(m.detailScroll+delta, 0, m.maxDetailScroll())
}

func (m model) maxListScroll() int {
	if m.screen.kind != screenAnalytics {
		return 0
	}
	layout := m.bodyLayout(m.renderHeader(), m.renderStatus(), nil)
	lines := flattenChunks(m.renderAnalyticsList(layout.listWidth))
	return maxScrollOffset(len(lines), paneContentHeight(layout.height))
}

func (m model) maxDetailScroll() int {
	layout := m.bodyLayout(m.renderHeader(), m.renderStatus(), nil)
	lines := m.detailContentLines(layout.detailWidth)
	return maxScrollOffset(len(lines), paneContentHeight(layout.height))
}
