package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

func (m model) renderAnalyticsList(width int) []string {
	dates14 := recentDates(14)
	overall := m.completionCounts(nil)
	lines := []string{
		panelHeading("Analytics", m.activePane == paneList),
		mutedStyle.Render("Daily completion tracking by milestone and goal."),
		"",
	}
	lines = append(lines, m.renderBarChart("Last 14 days", overall, dates14, max(8, width-20))...)
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("Summary"))
	lines = append(lines, fmt.Sprintf("Today: %d", m.completedTodosOn(todayDateString())))
	lines = append(lines, fmt.Sprintf("Last 7 days: %d", sumCounts(overall, recentDates(7))))
	lines = append(lines, fmt.Sprintf("All time: %d", m.completedTodoCount()))
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("Milestones"))
	for _, group := range m.analyticsGroups() {
		lines = append(lines, fmt.Sprintf("%s  total %d • 7d %d", trimLabel(group.Label, width-18), group.Total, sumCounts(group.Counts, recentDates(7))))
	}
	return lines
}

func (m model) renderAnalyticsDetail(width int) []string {
	dates14 := recentDates(14)
	lines := []string{
		headerStyle.Render("Breakdown"),
		mutedStyle.Render("Sparkline legend: . = 0, : = 1, * = 2-3, # = 4+"),
		"",
		headerStyle.Render("Overall"),
		fmt.Sprintf("Last 14d %s  total %d", sparkline(m.completionCounts(nil), dates14), m.completedTodoCount()),
	}

	inbox := m.inboxCompletionSeries()
	if inbox.Total > 0 {
		lines = append(lines, fmt.Sprintf("Inbox      %s  total %d", sparkline(inbox.Counts, dates14), inbox.Total))
	}

	for _, group := range m.analyticsGroups() {
		lines = append(lines, "")
		lines = append(lines, headerStyle.Render(trimLabel(group.Label, width-2)))
		lines = append(lines, fmt.Sprintf("Milestone  %s  total %d • 7d %d", sparkline(group.Counts, dates14), group.Total, sumCounts(group.Counts, recentDates(7))))
		for _, goal := range group.Goals {
			lines = append(lines, fmt.Sprintf("Goal %-14s %s  %d", trimLabel(goal.Label, 14), sparkline(goal.Counts, dates14), goal.Total))
		}
	}
	if len(lines) == 5 {
		lines = append(lines, "", mutedStyle.Render("No completed tasks yet. Press c on a todo to start tracking daily completion."))
	}
	return lines
}

func (m model) analyticsGroups() []analyticsGroup {
	groups := []analyticsGroup{}

	for _, milestone := range m.data.Milestones {
		group := analyticsGroup{
			Label:  milestone.Name,
			Counts: map[string]int{},
		}
		for _, item := range m.data.Todos {
			if !item.Completed || item.GoalID == 0 || item.CompletedAt == "" {
				continue
			}
			goal := m.findGoal(item.GoalID)
			if goal == nil || goal.MilestoneID != milestone.ID {
				continue
			}
			group.Counts[item.CompletedAt]++
		}
		group.Total = totalCounts(group.Counts)
		if group.Total == 0 {
			continue
		}

		for _, goal := range m.orderedGoalsForMilestone(milestone.ID) {
			series := analyticsSeries{
				Label:  strings.Join(m.goalPath(goal), " / "),
				Counts: map[string]int{},
			}
			for _, item := range m.data.Todos {
				if item.Completed && item.GoalID == goal.ID && item.CompletedAt != "" {
					series.Counts[item.CompletedAt]++
				}
			}
			series.Total = totalCounts(series.Counts)
			if series.Total > 0 {
				group.Goals = append(group.Goals, series)
			}
		}
		groups = append(groups, group)
	}

	return groups
}

func (m model) orderedGoalsForMilestone(milestoneID int) []goal {
	goals := []goal{}
	for _, item := range m.data.Goals {
		if item.MilestoneID == milestoneID {
			goals = append(goals, item)
		}
	}
	slices.SortFunc(goals, func(a, b goal) int {
		if a.ParentGoalID != b.ParentGoalID {
			return compareOrder(a.ParentGoalID, b.ParentGoalID, a.ID, b.ID)
		}
		return compareOrder(a.Order, b.Order, a.ID, b.ID)
	})
	return goals
}

func (m model) completionCounts(filter func(todo) bool) map[string]int {
	counts := map[string]int{}
	for _, item := range m.data.Todos {
		if !item.Completed || item.CompletedAt == "" {
			continue
		}
		if filter != nil && !filter(item) {
			continue
		}
		counts[item.CompletedAt]++
	}
	return counts
}

func (m model) inboxCompletionSeries() analyticsSeries {
	counts := m.completionCounts(func(item todo) bool {
		return item.GoalID == 0
	})
	return analyticsSeries{
		Label:  "Inbox",
		Counts: counts,
		Total:  totalCounts(counts),
	}
}

func (m model) renderBarChart(title string, counts map[string]int, dates []string, width int) []string {
	lines := []string{headerStyle.Render(title)}
	maxCount := 0
	for _, date := range dates {
		if counts[date] > maxCount {
			maxCount = counts[date]
		}
	}
	if maxCount == 0 {
		lines = append(lines, mutedStyle.Render("No completions recorded in this window."))
		return lines
	}
	for _, date := range dates {
		count := counts[date]
		barLen := 0
		if count > 0 {
			barLen = max(1, count*width/maxCount)
		}
		lines = append(lines, fmt.Sprintf("%s | %-*s %d", shortDate(date), width, strings.Repeat("#", barLen), count))
	}
	return lines
}

func recentDates(days int) []string {
	result := make([]string, 0, days)
	today := time.Now()
	for i := days - 1; i >= 0; i-- {
		result = append(result, today.AddDate(0, 0, -i).Format(time.DateOnly))
	}
	return result
}

func sparkline(counts map[string]int, dates []string) string {
	var builder strings.Builder
	for _, date := range dates {
		switch value := counts[date]; {
		case value == 0:
			builder.WriteByte('.')
		case value == 1:
			builder.WriteByte(':')
		case value <= 3:
			builder.WriteByte('*')
		default:
			builder.WriteByte('#')
		}
	}
	return builder.String()
}

func sumCounts(counts map[string]int, dates []string) int {
	total := 0
	for _, date := range dates {
		total += counts[date]
	}
	return total
}

func totalCounts(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func shortDate(date string) string {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}
	return parsed.Format("01-02")
}

func trimLabel(label string, width int) string {
	if width <= 3 || len(label) <= width {
		return label
	}
	return label[:width-3] + "..."
}
