package fuzz

import (
	"fmt"

	"github.com/NoF0rte/graphqshell/internal/graphql"
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
		currentPath = o.Name
		if t != "" {
			currentPath = fmt.Sprintf("%s %s", currentPath, t)
		}
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
	fmt.Printf("[%s] %s\n", l.job.Type, fmt.Sprintf(format, v...))
}

// func (l *Logger) Arg(o *graphql.Object) {
// 	p := l.Path(o)
// 	l.Log("Found: %s", p)
// }

func (l *Logger) Enum(o *graphql.Object, enum *graphql.Object) {
	p := l.Path(o)
	l.Log("Found: %s => %s", p, enum.Name)
}

// func (l *Logger) Field(o *graphql.Object) {
// 	p := l.Path(o)
// 	l.Log("Found: %s", p)
// }

// func (l *Logger) Type(o *graphql.Object, t string) {
// 	p := l.Path(o)
// 	l.Log("Found: %s %s", p, t)
// }

// func (l *Logger) RequiredArg(o *graphql.Object, arg *graphql.Object, t string) {
// 	p := l.Path(o)
// 	l.Log("Found: %s(%s %s)", p, arg.Name, t)
// }

// func (l *Logger) RequiredArgField(o *graphql.Object, t string) {
// 	p := l.Path(o)
// 	l.Log("Found: %s %s", p, t)
// }

func (l *Logger) Found(o *graphql.Object) {
	p := l.Path(o)
	l.Log("Found: %s", p)
}

func (l *Logger) FoundWithType(o *graphql.Object, t string) {
	p := l.path(o, "", t)
	// if strings.HasSuffix(p, ")") {
	// 	p = strings.Replace(p, ")", fmt.Sprintf(" %s)", t), 1)
	// } else {
	// 	p = fmt.Sprintf("%s %s", p, t)
	// }

	l.Log("Found: %s", p)
}
