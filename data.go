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
	m.syncGoalMilestones()
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
	m.syncGoalMilestones()
	for i := range m.data.Milestones {
		normalizeMilestone(&m.data.Milestones[i])
	}
	for i := range m.data.Goals {
		normalizeGoal(&m.data.Goals[i])
	}
	for i := range m.data.Todos {
		normalizeTodo(&m.data.Todos[i])
	}
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
	for i := range data.Milestones {
		normalizeMilestone(&data.Milestones[i])
	}
	for i := range data.Goals {
		normalizeGoal(&data.Goals[i])
	}
	for i := range data.Todos {
		normalizeTodo(&data.Todos[i])
	}
	return data, nil
}

func (m *model) syncGoalMilestones() {
	for {
		changed := false
		for i := range m.data.Goals {
			parentID := m.data.Goals[i].ParentGoalID
			if parentID == 0 {
				continue
			}
			parent := m.findGoal(parentID)
			if parent == nil || m.data.Goals[i].MilestoneID == parent.MilestoneID {
				continue
			}
			m.data.Goals[i].MilestoneID = parent.MilestoneID
			changed = true
		}
		if !changed {
			return
		}
	}
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
		todoBuckets[m.todoParentKey(m.data.Todos[i])] = append(todoBuckets[m.todoParentKey(m.data.Todos[i])], i)
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

func (m model) todoParentKey(item todo) int {
	switch {
	case item.GoalID != 0:
		return item.GoalID
	case item.MilestoneID != 0:
		return -item.MilestoneID
	default:
		return 0
	}
}

func (m *model) nextID() int {
	id := m.data.NextID
	m.data.NextID++
	return id
}

func (m model) sidebarEntries() []sidebarEntry {
	entries := []sidebarEntry{
		{label: "Inbox", meta: fmt.Sprintf("%d active • %d done", len(m.inboxTodos()), m.countInboxCompletedTodos()), screen: screenState{kind: screenInbox}},
		{label: "Active Tasks", meta: fmt.Sprintf("%d open • %d in progress", m.openTodoCount(), m.inProgressTodoCount()), screen: screenState{kind: screenAll}},
		{label: "Archive", meta: fmt.Sprintf("%d completed", len(m.completedTodos())), screen: screenState{kind: screenCompleted}},
		{label: "Analytics", meta: fmt.Sprintf("%d done today", m.completedTodosOn(todayDateString())), screen: screenState{kind: screenAnalytics}},
	}
	for _, milestone := range m.data.Milestones {
		label := milestone.Name
		if milestone.Completed {
			label += " [done]"
		}
		entries = append(entries, sidebarEntry{
			label:       label,
			meta:        fmt.Sprintf("%d active • %d done • %d goals", m.countMilestoneOpenTodos(milestone.ID), m.countMilestoneCompletedTodos(milestone.ID), m.countMilestoneGoals(milestone.ID)),
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
	case screenCompleted:
		items := make([]focusItem, 0, len(m.data.Todos))
		for _, item := range m.completedTodos() {
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
		for _, todo := range m.data.Todos {
			if todoBelongsToMilestone(todo, m.screen.milestoneID) && !todoIsCompleted(todo) {
				items = append(items, focusItem{kind: itemTodo, id: todo.ID, order: todo.Order})
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
			if todo.GoalID == m.screen.goalID && !todoIsCompleted(todo) {
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
		return "Active Tasks"
	case screenCompleted:
		return "Archive"
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
		return fmt.Sprintf("%d active tasks waiting in the inbox", len(m.inboxTodos()))
	case screenAll:
		return fmt.Sprintf("%d active tasks across every goal and inbox", len(m.allTodos()))
	case screenCompleted:
		return fmt.Sprintf("%d completed tasks, tucked away from the working views", len(m.completedTodos()))
	case screenAnalytics:
		return "Completed tasks by day, milestone, and goal"
	case screenMilestone:
		milestone := m.mustMilestone(m.screen.milestoneID)
		return fmt.Sprintf("%s • %s • %d top-level goals • %d milestone tasks • %d active • %d done", dateRange(milestone.StartDate, milestone.EndDate), milestoneCompletionLabel(milestone), m.countTopLevelGoals(milestone.ID), m.countDirectMilestoneTodos(milestone.ID), m.countMilestoneOpenTodos(milestone.ID), m.countMilestoneCompletedTodos(milestone.ID))
	case screenGoal:
		goal := m.mustGoal(m.screen.goalID)
		return fmt.Sprintf("%s • %s • %s • %d active • %d done", strings.Join(m.goalPath(goal), " / "), dateRange(goal.StartDate, goal.EndDate), goalCompletionLabel(goal), m.countGoalOpenTodos(goal.ID), m.countGoalCompletedTodos(goal.ID))
	default:
		return ""
	}
}

func (m model) contextHint() string {
	switch m.screen.kind {
	case screenInbox, screenAll:
		return "n add task • / jump or create • t in progress • c complete • m move • v grab • e edit • x delete • I/u priority • S sort • C archive"
	case screenCompleted:
		return "c reopen • m move • e edit • x delete • / jump • y analytics"
	case screenAnalytics:
		return "Analytics tracks completed todos by day, milestone, and goal."
	case screenMilestone:
		return "s add goal • n add task • c complete milestone/goal/task • enter open goal • m move • v grab • e edit • x delete • I/u priority • S sort • C archive"
	case screenGoal:
		return "n add task • s add subgoal • t in progress • c complete goal/task • m move • v grab • e edit • x delete • I/u priority • S sort • h back • C archive"
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

func (m model) quickAddMilestoneID() int {
	if m.screen.kind == screenMilestone {
		return m.screen.milestoneID
	}
	return 0
}

func (m model) quickAddDestinationLabel() string {
	if m.form.targetGoalID != 0 {
		return fmt.Sprintf("goal %q", m.mustGoal(m.form.targetGoalID).Name)
	}
	if m.form.targetMilestoneID != 0 {
		return fmt.Sprintf("milestone %q", m.mustMilestone(m.form.targetMilestoneID).Name)
	}
	if m.form.target == 0 {
		return "Inbox"
	}
	return fmt.Sprintf("goal %q", m.mustGoal(m.form.target).Name)
}

func (m model) quickAddBrowseDestinationLabel() string {
	if m.screen.kind == screenGoal {
		return fmt.Sprintf("goal %q", m.mustGoal(m.screen.goalID).Name)
	}
	if m.screen.kind == screenMilestone {
		return fmt.Sprintf("milestone %q", m.mustMilestone(m.screen.milestoneID).Name)
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
			label: fmt.Sprintf(`+ Create task "%s" in Inbox`, raw),
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
					kind:        "todo",
					id:          todo.ID,
					milestoneID: todo.MilestoneID,
					goalID:      todo.GoalID,
					label:       fmt.Sprintf("T  %s • %s • %s", todo.Name, m.todoContext(todo), m.todoCompletionLabel(todo)),
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
		if todoBelongsToInbox(item) && !todoIsCompleted(item) {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b todo) int {
		return compareOrder(a.Order, b.Order, a.ID, b.ID)
	})
	return items
}

func (m model) allTodos() []todo {
	items := []todo{}
	for _, item := range m.data.Todos {
		if !todoIsCompleted(item) {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b todo) int {
		return compareOrder(a.GlobalOrder, b.GlobalOrder, a.ID, b.ID)
	})
	return items
}

func (m model) completedTodos() []todo {
	items := []todo{}
	for _, item := range m.data.Todos {
		if todoIsCompleted(item) {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b todo) int {
		if a.CompletedAt != b.CompletedAt {
			if a.CompletedAt > b.CompletedAt {
				return -1
			}
			return 1
		}
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
	if todoBelongsToInbox(item) {
		return "Inbox"
	}
	if item.MilestoneID != 0 {
		milestone := m.mustMilestone(item.MilestoneID)
		if milestone.ID != 0 {
			return milestone.Name
		}
		return "Unknown milestone"
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

func (m model) countTopLevelGoals(id int) int {
	count := 0
	for _, goal := range m.data.Goals {
		if goal.MilestoneID == id && goal.ParentGoalID == 0 {
			count++
		}
	}
	return count
}

func (m model) countDirectMilestoneTodos(id int) int {
	count := 0
	for _, item := range m.data.Todos {
		if todoBelongsToMilestone(item, id) {
			count++
		}
	}
	return count
}

func (m model) countMilestoneTodos(id int) int {
	count := 0
	for _, item := range m.data.Todos {
		if todoBelongsToMilestone(item, id) {
			count++
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
		if !todoIsCompleted(item) {
			continue
		}
		if todoBelongsToMilestone(item, id) {
			count++
			continue
		}
		goal := m.findGoal(item.GoalID)
		if goal != nil && goal.MilestoneID == id {
			count++
		}
	}
	return count
}

func (m model) countMilestoneOpenTodos(id int) int {
	return m.countMilestoneTodos(id) - m.countMilestoneCompletedTodos(id)
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

func (m model) goalSubtreeIDs(id int) map[int]struct{} {
	ids := map[int]struct{}{id: {}}
	for {
		changed := false
		for _, goal := range m.data.Goals {
			if _, ok := ids[goal.ParentGoalID]; ok {
				if _, exists := ids[goal.ID]; !exists {
					ids[goal.ID] = struct{}{}
					changed = true
				}
			}
		}
		if !changed {
			return ids
		}
	}
}

func (m model) countGoalTodos(id int) int {
	subtree := m.goalSubtreeIDs(id)
	count := 0
	for _, item := range m.data.Todos {
		if _, ok := subtree[item.GoalID]; ok {
			count++
		}
	}
	return count
}

func (m model) countGoalCompletedTodos(id int) int {
	subtree := m.goalSubtreeIDs(id)
	count := 0
	for _, item := range m.data.Todos {
		if _, ok := subtree[item.GoalID]; ok && todoIsCompleted(item) {
			count++
		}
	}
	return count
}

func (m model) countGoalOpenTodos(id int) int {
	return m.countGoalTodos(id) - m.countGoalCompletedTodos(id)
}

func (m model) countInboxCompletedTodos() int {
	count := 0
	for _, item := range m.data.Todos {
		if todoBelongsToInbox(item) && todoIsCompleted(item) {
			count++
		}
	}
	return count
}

func (m model) openTodoCount() int {
	count := 0
	for _, item := range m.data.Todos {
		if todoStatusValue(item) == todoStatusOpen {
			count++
		}
	}
	return count
}

func (m model) inProgressTodoCount() int {
	count := 0
	for _, item := range m.data.Todos {
		if todoIsInProgress(item) {
			count++
		}
	}
	return count
}

func (m model) completedTodoCount() int {
	count := 0
	for _, item := range m.data.Todos {
		if todoIsCompleted(item) {
			count++
		}
	}
	return count
}

func (m model) completedTodosOn(date string) int {
	count := 0
	for _, item := range m.data.Todos {
		if todoIsCompleted(item) && item.CompletedAt == date {
			count++
		}
	}
	return count
}

func (m model) screenExists(screen screenState) bool {
	switch screen.kind {
	case screenInbox, screenAll, screenCompleted, screenAnalytics:
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
		if item.MilestoneID == id {
			return true
		}
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

func (m *model) syncGoalSubtreeMilestone(rootID, milestoneID int) {
	for i := range m.data.Goals {
		if m.data.Goals[i].ID == rootID || m.goalIsDescendant(m.data.Goals[i].ID, rootID) {
			m.data.Goals[i].MilestoneID = milestoneID
		}
	}
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
