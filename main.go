package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/NoF0rte/graphqshell/internal/tengomod"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengo/v2/parser"
	"github.com/chzyer/readline"
)

const (
	replPrompt = ">> "
)

func main() {
	modules := tengomod.GetModuleMap()
	RunREPL(modules, os.Stdin, os.Stdout)
}

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

func RunREPL(modules *tengo.ModuleMap, in io.ReadCloser, out io.Writer) {
	l, err := readline.NewEx(&readline.Config{
		Prompt:                 "\033[31m»\033[0m ",
		HistoryFile:            "/tmp/graphqshell.tmp",
		DisableAutoSaveHistory: true,
		// AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
		Stdin:               in,
		Stdout:              out,
	})
	if err != nil {
		panic(err)
	}
	defer l.Close()
	l.CaptureExitSignal()

	fileSet := parser.NewFileSet()
	globals := make([]tengo.Object, tengo.GlobalsSize)
	symbolTable := tengo.NewSymbolTable()
	for idx, fn := range tengo.GetAllBuiltinFunctions() {
		symbolTable.DefineBuiltin(idx, fn.Name)
	}

	// embed println function
	symbol := symbolTable.Define("__repl_println__")
	globals[symbol.Index] = &tengo.UserFunction{
		Name: "println",
		Value: func(args ...tengo.Object) (ret tengo.Object, err error) {
			var printArgs []interface{}
			for _, arg := range args {
				if _, isUndefined := arg.(*tengo.Undefined); isUndefined {
					printArgs = append(printArgs, "<undefined>")
				} else {
					s, _ := tengo.ToString(arg)
					printArgs = append(printArgs, s)
				}
			}
			printArgs = append(printArgs, "\n")
			_, _ = fmt.Print(printArgs...)
			return
		},
	}

	gqlSymbol := symbolTable.Define("graphql")
	globals[gqlSymbol.Index] = modules.GetBuiltinModule("graphql").AsImmutableMap("graphql")

	spewSymbol := symbolTable.Define("spew")
	globals[spewSymbol.Index] = tengomod.Spew(out)

	var constants []tengo.Object
	for {
		line, err := l.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)

		srcFile := fileSet.AddFile("repl", -1, len(line))
		p := parser.NewParser(srcFile, []byte(line), nil)
		file, err := p.ParseFile()
		if err != nil {
			_, _ = fmt.Fprintln(out, err.Error())
			continue
		}

		file = addPrints(file)
		c := tengo.NewCompiler(srcFile, symbolTable, constants, modules, nil)
		if err := c.Compile(file); err != nil {
			_, _ = fmt.Fprintln(out, err.Error())
			continue
		}

		if line != "" {
			l.SaveHistory(line)
		}

		bytecode := c.Bytecode()
		machine := tengo.NewVM(bytecode, globals, -1)
		if err := machine.Run(); err != nil {
			_, _ = fmt.Fprintln(out, err.Error())
			continue
		}
		constants = bytecode.Constants
	}
}

func addPrints(file *parser.File) *parser.File {
	var stmts []parser.Stmt
	for _, s := range file.Stmts {
		switch s := s.(type) {
		case *parser.ExprStmt:
			stmts = append(stmts, &parser.ExprStmt{
				Expr: &parser.CallExpr{
					Func: &parser.Ident{Name: "__repl_println__"},
					Args: []parser.Expr{s.Expr},
				},
			})
		// case *parser.AssignStmt:
		// 	stmts = append(stmts, s)

		// 	stmts = append(stmts, &parser.ExprStmt{
		// 		Expr: &parser.CallExpr{
		// 			Func: &parser.Ident{
		// 				Name: "__repl_println__",
		// 			},
		// 			Args: s.LHS,
		// 		},
		// 	})
		default:
			stmts = append(stmts, s)
		}
	}
	return &parser.File{
		InputFile: file.InputFile,
		Stmts:     stmts,
	}
}
