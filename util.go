package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

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

func (m model) todoCheckbox(item todo) string {
	if item.Completed {
		return "[x]"
	}
	return "[ ]"
}

func (m model) todoCompletionLabel(item todo) string {
	if item.Completed && item.CompletedAt != "" {
		return "completed " + item.CompletedAt
	}
	if item.Completed {
		return "completed"
	}
	return "open"
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
