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
	milestones := m.completionSeriesByMilestone()
	goals := m.completionSeriesByGoal()
	lines := []string{
		panelHeading("Analytics", m.activePane == paneList),
		mutedStyle.Render("Track completed tasks by day, milestone, and goal."),
		"",
	}
	lines = append(lines, m.renderBarChart("By Day · Last 14 Days", overall, dates14, max(8, width-20))...)
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("Summary"))
	lines = append(lines, fmt.Sprintf("Today: %d", m.completedTodosOn(todayDateString())))
	lines = append(lines, fmt.Sprintf("Last 7 days: %d", sumCounts(overall, recentDates(7))))
	lines = append(lines, fmt.Sprintf("All time: %d", m.completedTodoCount()))
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("By Milestone"))
	if len(milestones) == 0 {
		lines = append(lines, mutedStyle.Render("No milestone completions yet."))
	} else {
		for _, series := range milestones {
			lines = append(lines, fmt.Sprintf("%s  total %d • 7d %d", trimLabel(series.Label, width-18), series.Total, sumCounts(series.Counts, recentDates(7))))
		}
	}
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("By Goal"))
	if len(goals) == 0 {
		lines = append(lines, mutedStyle.Render("No goal completions yet."))
	} else {
		for _, series := range goals {
			lines = append(lines, fmt.Sprintf("%s  total %d • 7d %d", trimLabel(series.Label, width-18), series.Total, sumCounts(series.Counts, recentDates(7))))
		}
	}
	return lines
}

func (m model) renderAnalyticsDetail(width int) []string {
	dates14 := recentDates(14)
	milestones := m.completionSeriesByMilestone()
	goals := m.completionSeriesByGoal()
	lines := []string{
		headerStyle.Render("Breakdown"),
		mutedStyle.Render("Sparkline legend: . = 0, : = 1, * = 2-3, # = 4+"),
		"",
		headerStyle.Render("By Day"),
		fmt.Sprintf("Overall    %s  total %d", sparkline(m.completionCounts(nil), dates14), m.completedTodoCount()),
	}

	inbox := m.inboxCompletionSeries()
	if inbox.Total > 0 {
		lines = append(lines, fmt.Sprintf("Inbox      %s  total %d", sparkline(inbox.Counts, dates14), inbox.Total))
	}

	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("By Milestone"))
	if len(milestones) == 0 {
		lines = append(lines, mutedStyle.Render("No milestone completions yet."))
	} else {
		for _, series := range milestones {
			lines = append(lines, fmt.Sprintf("%-14s %s  %d", trimLabel(series.Label, 14), sparkline(series.Counts, dates14), series.Total))
		}
	}

	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("By Goal"))
	if len(goals) == 0 {
		lines = append(lines, mutedStyle.Render("No goal completions yet."))
	} else {
		for _, series := range goals {
			lines = append(lines, fmt.Sprintf("%-14s %s  %d", trimLabel(series.Label, 14), sparkline(series.Counts, dates14), series.Total))
		}
	}

	if m.completedTodoCount() == 0 {
		lines = append(lines, "", mutedStyle.Render("No completed tasks yet. Press c on a todo to start tracking daily completion."))
	}
	return lines
}

func (m model) completionSeriesByMilestone() []analyticsSeries {
	series := []analyticsSeries{}
	for _, milestone := range m.data.Milestones {
		counts := m.completionCounts(func(item todo) bool {
			if todoBelongsToMilestone(item, milestone.ID) {
				return true
			}
			goal := m.findGoal(item.GoalID)
			return goal != nil && goal.MilestoneID == milestone.ID
		})
		total := totalCounts(counts)
		if total == 0 {
			continue
		}
		series = append(series, analyticsSeries{
			Label:  milestone.Name,
			Counts: counts,
			Total:  total,
		})
	}
	return series
}

func (m model) completionSeriesByGoal() []analyticsSeries {
	series := []analyticsSeries{}
	for _, goal := range m.orderedGoals() {
		counts := m.completionCounts(func(item todo) bool {
			return item.GoalID == goal.ID
		})
		total := totalCounts(counts)
		if total == 0 {
			continue
		}
		series = append(series, analyticsSeries{
			Label:  strings.Join(m.goalPath(goal), " / "),
			Counts: counts,
			Total:  total,
		})
	}
	return series
}

func (m model) orderedGoals() []goal {
	goals := append([]goal(nil), m.data.Goals...)
	slices.SortFunc(goals, func(a, b goal) int {
		pathA := strings.Join(m.goalPath(a), " / ")
		pathB := strings.Join(m.goalPath(b), " / ")
		switch {
		case pathA < pathB:
			return -1
		case pathA > pathB:
			return 1
		default:
			return compareOrder(a.Order, b.Order, a.ID, b.ID)
		}
	})
	return goals
}

func (m model) completionCounts(filter func(todo) bool) map[string]int {
	counts := map[string]int{}
	for _, item := range m.data.Todos {
		if !todoIsCompleted(item) || item.CompletedAt == "" {
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
		return todoBelongsToInbox(item)
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
