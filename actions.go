package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

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
		goalID := m.form.targetGoalID
		milestoneID := m.form.targetMilestoneID
		newTodo := todo{
			ID:          m.nextID(),
			MilestoneID: milestoneID,
			GoalID:      goalID,
			Name:        name,
			Order:       m.nextTodoOrderFor(goalID, milestoneID),
			GlobalOrder: m.nextTodoGlobalOrder(),
			Status:      todoStatusOpen,
		}
		m.data.Todos = append(m.data.Todos, newTodo)
		if goalID != 0 {
			m.setScreen(screenState{kind: screenGoal, goalID: goalID}, true)
			m.status = successStyle.Render("Todo added to goal.")
		} else if milestoneID != 0 {
			m.setScreen(screenState{kind: screenMilestone, milestoneID: milestoneID}, true)
			m.status = successStyle.Render("Todo added to milestone.")
		} else {
			m.setScreen(screenState{kind: screenInbox}, false)
			m.status = successStyle.Render("Todo captured in Inbox.")
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
			MilestoneID: 0,
			Name:        result.query,
			Order:       m.nextTodoOrderFor(0, 0),
			GlobalOrder: m.nextTodoGlobalOrder(),
			Status:      todoStatusOpen,
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
		todo := m.mustTodo(result.id)
		if todoIsCompleted(todo) {
			m.setScreen(screenState{kind: screenCompleted}, false)
		} else if result.goalID != 0 {
			m.screenBack = nil
			m.setScreen(screenState{kind: screenGoal, goalID: result.goalID}, true)
		} else if result.milestoneID != 0 {
			m.setScreen(screenState{kind: screenMilestone, milestoneID: result.milestoneID}, false)
		} else {
			m.setScreen(screenState{kind: screenInbox}, false)
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
			target.MilestoneID = 0
			target.GoalID = 0
			target.Order = m.nextTodoOrderFor(0, 0)
			m.setScreen(screenState{kind: screenInbox}, false)
			m.selectTodo(target.ID)
		case "milestone":
			target.MilestoneID = result.id
			target.GoalID = 0
			target.Order = m.nextTodoOrderFor(0, result.id)
			m.setScreen(screenState{kind: screenMilestone, milestoneID: result.id}, false)
			m.selectTodo(target.ID)
		case "goal":
			target.MilestoneID = 0
			target.GoalID = result.id
			target.Order = m.nextTodoOrderFor(result.id, 0)
			m.setScreen(screenState{kind: screenGoal, goalID: result.id}, true)
			m.selectTodo(target.ID)
		default:
			return fmt.Errorf("todo can only move to Inbox, a milestone, or a goal")
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
			m.syncGoalSubtreeMilestone(target.ID, result.id)
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
			m.syncGoalSubtreeMilestone(target.ID, parent.MilestoneID)
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

func (m *model) toggleCompletion() error {
	item, ok := m.selectedItem()
	if !ok || item.kind != itemTodo {
		return fmt.Errorf("select a todo first")
	}
	target := m.mustTodoPtr(item.id)
	if target == nil {
		return fmt.Errorf("todo not found")
	}
	if todoIsCompleted(*target) {
		setTodoStatus(target, todoStatusOpen)
		m.status = successStyle.Render("Todo marked incomplete.")
		return nil
	}
	setTodoStatus(target, todoStatusCompleted)
	target.CompletedAt = todayDateString()
	m.status = successStyle.Render(fmt.Sprintf("Todo completed on %s.", target.CompletedAt))
	return nil
}

func (m *model) toggleInProgress() error {
	item, ok := m.selectedItem()
	if !ok || item.kind != itemTodo {
		return fmt.Errorf("select a todo first")
	}
	target := m.mustTodoPtr(item.id)
	if target == nil {
		return fmt.Errorf("todo not found")
	}
	if todoIsInProgress(*target) {
		setTodoStatus(target, todoStatusOpen)
		m.status = successStyle.Render("Todo marked open.")
		return nil
	}
	setTodoStatus(target, todoStatusInProgress)
	m.status = successStyle.Render("Todo marked in progress.")
	return nil
}

func (m *model) bumpSelection(direction int) error {
	if direction != -1 && direction != 1 {
		return fmt.Errorf("invalid move direction")
	}
	if m.activePane == paneSidebar {
		entries := m.sidebarEntries()
		if m.sidebarIdx < fixedSidebarEntries || m.sidebarIdx >= len(entries) {
			return fmt.Errorf("reorder milestones from the milestone list only")
		}
		next := m.sidebarIdx + direction
		if next < fixedSidebarEntries || next >= len(entries) {
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
		return m.todoParentKey(m.mustTodo(a.id)) == m.todoParentKey(m.mustTodo(b.id))
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
	case screenCompleted:
		m.sidebarIdx = 2
	case screenAnalytics:
		m.sidebarIdx = 3
	}

	items := m.visibleItems()
	switch {
	case len(items) == 0:
		m.listIdx = 0
	case m.listIdx < 0:
		m.listIdx = 0
	case m.listIdx >= len(items):
		m.listIdx = len(items) - 1
	}
}
