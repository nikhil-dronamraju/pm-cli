package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

func configureTerminalColors() {
	if dark, ok := terminalBackgroundHint(); ok {
		lipgloss.SetHasDarkBackground(dark)
	}
}

func newThemedTextInput() textinput.Model {
	input := textinput.New()
	input.PromptStyle = keyStyle
	input.TextStyle = lipgloss.NewStyle().Foreground(bodyColor)
	input.PlaceholderStyle = mutedStyle
	input.CompletionStyle = mutedStyle
	input.Cursor.Style = lipgloss.NewStyle().Foreground(accentColor)
	input.Cursor.TextStyle = input.TextStyle
	input.CursorStyle = input.Cursor.Style
	return input
}

func terminalBackgroundHint() (bool, bool) {
	if dark, ok := colorFGBGDarkBackground(os.Getenv("COLORFGBG")); ok {
		return dark, true
	}

	for _, key := range []string{"TERMINAL_BACKGROUND", "COLOR_SCHEME", "ITERM_PROFILE"} {
		if dark, ok := namedBackgroundHint(os.Getenv(key)); ok {
			return dark, true
		}
	}

	return false, false
}

func colorFGBGDarkBackground(value string) (bool, bool) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ':'
	})
	for i := len(parts) - 1; i >= 0; i-- {
		index, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err == nil {
			return ansiColorIndexIsDark(index), true
		}
	}
	return false, false
}

func namedBackgroundHint(value string) (bool, bool) {
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "light"):
		return false, true
	case strings.Contains(value, "dark"):
		return true, true
	default:
		return false, false
	}
}

func ansiColorIndexIsDark(index int) bool {
	switch {
	case index < 0:
		return true
	case index < 8:
		return index != 7
	case index < 16:
		return index == 8
	case index < 232:
		r, g, b := ansi256Color(index)
		return relativeLuminance(r, g, b) < 0.5
	case index < 256:
		level := 8 + (index-232)*10
		return relativeLuminance(level, level, level) < 0.5
	default:
		return true
	}
}

func ansi256Color(index int) (int, int, int) {
	level := func(value int) int {
		if value == 0 {
			return 0
		}
		return 55 + value*40
	}
	index -= 16
	return level(index / 36), level((index / 6) % 6), level(index % 6)
}

func relativeLuminance(r, g, b int) float64 {
	return 0.2126*float64(r)/255 + 0.7152*float64(g)/255 + 0.0722*float64(b)/255
}
