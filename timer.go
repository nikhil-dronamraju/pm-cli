package main

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateTimerTick() (tea.Model, tea.Cmd) {
	if !m.timer.running {
		return m, nil
	}
	m.timer.remaining -= time.Second
	if m.timer.phase == phaseWork {
		m.timer.workAccumulated += time.Second
	}
	if m.timer.remaining <= 0 {
		m.advanceTimer()
		m.status = successStyle.Render(fmt.Sprintf("Pomodoro switched to %s.", m.phaseLabel()))
		notifyPomodoroPhase(m.phaseLabel(), m.timer.remaining)
	}
	if m.timer.running {
		return m, timerTick()
	}
	return m, nil
}

func (m *model) advanceTimer() {
	switch m.timer.phase {
	case phaseWork:
		if m.timer.workAccumulated > 0 && m.timer.workAccumulated%longBreakEveryWork == 0 {
			m.timer.phase = phaseLongBreak
			m.timer.remaining = longBreakDuration
		} else {
			m.timer.phase = phaseShortBreak
			m.timer.remaining = shortBreakDuration
		}
	case phaseShortBreak, phaseLongBreak:
		m.timer.phase = phaseWork
		m.timer.remaining = workDuration
	}
}

func (m *model) resetTimer() {
	m.timer.running = false
	switch m.timer.phase {
	case phaseWork:
		m.timer.remaining = workDuration
	case phaseShortBreak:
		m.timer.remaining = shortBreakDuration
	case phaseLongBreak:
		m.timer.remaining = longBreakDuration
	}
}

func (m model) timerBadge() string {
	label := "paused"
	if m.timer.running {
		label = "running"
	}
	return fmt.Sprintf("%s %s", m.phaseLabel(), label)
}

func (m model) timerSummary() string {
	return fmt.Sprintf("%s • %s remaining • %s work banked", m.phaseLabel(), formatDuration(m.timer.remaining), formatDuration(m.timer.workAccumulated))
}

func (m model) phaseLabel() string {
	switch m.timer.phase {
	case phaseWork:
		return "work"
	case phaseShortBreak:
		return "short break"
	case phaseLongBreak:
		return "long break"
	default:
		return "timer"
	}
}

func timerTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func notifyPomodoroPhase(phase string, duration time.Duration) {
	message := fmt.Sprintf("Switched to %s for %s.", phase, formatDuration(duration))
	script := fmt.Sprintf(`display dialog %q with title %q buttons {"OK"} default button "OK"`, message, "PM Go")
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Start(); err != nil {
		fmt.Printf("\a")
	}
}
