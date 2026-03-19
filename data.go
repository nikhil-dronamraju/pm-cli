package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

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
	for i := range data.Todos {
		if data.Todos[i].CompletedAt != "" {
			data.Todos[i].Completed = true
		}
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
		{label: "Inbox", meta: fmt.Sprintf("%d/%d done", m.countInboxCompletedTodos(), len(m.inboxTodos())), screen: screenState{kind: screenInbox}},
		{label: "All Tasks", meta: fmt.Sprintf("%d/%d done", m.completedTodoCount(), len(m.data.Todos)), screen: screenState{kind: screenAll}},
		{label: "Analytics", meta: fmt.Sprintf("%d done today", m.completedTodosOn(todayDateString())), screen: screenState{kind: screenAnalytics}},
	}
	for _, milestone := range m.data.Milestones {
		entries = append(entries, sidebarEntry{
			label:       milestone.Name,
			meta:        fmt.Sprintf("%d/%d todos done • %d goals", m.countMilestoneCompletedTodos(milestone.ID), m.countMilestoneTodos(milestone.ID), m.countMilestoneGoals(milestone.ID)),
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
	case screenAnalytics:
		return "Analytics"
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
	case screenAnalytics:
		return "Completed tasks per day"
	case screenMilestone:
		milestone := m.mustMilestone(m.screen.milestoneID)
		return fmt.Sprintf("%s • %d top-level goals • %d/%d todos done", dateRange(milestone.StartDate, milestone.EndDate), len(m.visibleItems()), m.countMilestoneCompletedTodos(milestone.ID), m.countMilestoneTodos(milestone.ID))
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
		return "n quick add • / jump or create • m move • c complete • v grab • e edit • x delete • I/u priority • S sort"
	case screenAnalytics:
		return "Analytics tracks todo completions by completed date."
	case screenMilestone:
		return "s add goal • n quick add • enter open goal • m move • v grab • e edit • x delete • I/u priority • S sort"
	case screenGoal:
		return "n add todo • s add subgoal • c complete todo • m move • v grab • e edit • x delete • I/u priority • S sort • h back"
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
	for _, item := range m.data.Todos {
		if item.GoalID == 0 {
			continue
		}
		goal := m.findGoal(item.GoalID)
		if goal != nil && goal.MilestoneID == id {
			count++
		}
	}
	return count
}

func (m model) countMilestoneCompletedTodos(id int) int {
	count := 0
	for _, item := range m.data.Todos {
		if !item.Completed || item.GoalID == 0 {
			continue
		}
		goal := m.findGoal(item.GoalID)
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
	for _, item := range m.data.Todos {
		if item.GoalID == id {
			count++
		}
	}
	return count
}

func (m model) countGoalCompletedTodos(id int) int {
	count := 0
	for _, item := range m.data.Todos {
		if item.GoalID == id && item.Completed {
			count++
		}
	}
	return count
}

func (m model) countInboxCompletedTodos() int {
	count := 0
	for _, item := range m.data.Todos {
		if item.GoalID == 0 && item.Completed {
			count++
		}
	}
	return count
}

func (m model) completedTodoCount() int {
	count := 0
	for _, item := range m.data.Todos {
		if item.Completed {
			count++
		}
	}
	return count
}

func (m model) completedTodosOn(date string) int {
	count := 0
	for _, item := range m.data.Todos {
		if item.Completed && item.CompletedAt == date {
			count++
		}
	}
	return count
}

func (m model) screenExists(screen screenState) bool {
	switch screen.kind {
	case screenInbox, screenAll, screenAnalytics:
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

func todayDateString() string {
	return time.Now().Format(time.DateOnly)
}
