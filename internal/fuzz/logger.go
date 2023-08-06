package fuzz

import (
	"fmt"
	"strings"

	"github.com/NoF0rte/graphqshell/internal/graphql"
	"github.com/charmbracelet/lipgloss"
)

var (
	jobStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#444ce3"))
	fieldStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#44e3d6"))
	typeStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#44e384"))
	argStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e3be44"))
)

type Logger struct {
	job *Job
}

func (l *Logger) Path(o *graphql.Object) string {
	return l.path(o, "", "")
}

func (l *Logger) path(o *graphql.Object, input string, t string) string {
	currentPath := input
	if currentPath == "" {
		currentPath = fieldStyle.Render(o.Name)
		if o.Caller != nil {
			currentPath = argStyle.Render(o.Name)
		}

		if t != "" {
			currentPath = fmt.Sprintf("%s %s", currentPath, typeStyle.Render(t))
		}
	} else if strings.HasPrefix(currentPath, "(") {
		currentPath = fmt.Sprintf("%s%s", o.Name, currentPath)
	} else {
		currentPath = fmt.Sprintf("%s.%s", o.Name, currentPath)
	}

	if o.Parent == nil && o.Caller == nil {
		return currentPath
	}

	if o.Parent != nil {
		return l.path(o.Parent, currentPath, "")
	}

	return l.path(o.Caller, fmt.Sprintf("(%s)", currentPath), "")
}

func (l *Logger) Log(format string, v ...interface{}) {
	fmt.Printf("[%s] %s\n", jobStyle.Render(string(l.job.Type)), fmt.Sprintf(format, v...))
}

func (l *Logger) Enum(o *graphql.Object, enum *graphql.Object) {
	p := l.Path(o)
	l.Log("Found: %s => %s", p, enum.Name)
}

func (l *Logger) Found(o *graphql.Object) {
	p := l.Path(o)
	l.Log("Found: %s", p)
}

func (l *Logger) FoundWithType(o *graphql.Object, t string) {
	p := l.path(o, "", t)
	l.Log("Found: %s", p)
}
