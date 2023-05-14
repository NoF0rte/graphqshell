package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/NoF0rte/graphqshell/internal/tengomod"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengo/v2/parser"
	"github.com/chzyer/readline"
	"github.com/common-nighthawk/go-figure"
	"github.com/spf13/cobra"
)

var (
	symbolTable *tengo.SymbolTable = tengo.NewSymbolTable()
	globals     []tengo.Object     = make([]tengo.Object, tengo.GlobalsSize)
	constants   []tengo.Object
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "graphqshell [path/to/script]",
	Short: "A GraphQL pentesting scripting engine. Run a script and/or run the REPL.",
	Example: `Run a script:
	graphqshell my-script.tengo

Run the REPL:
	graphqshell

Run a script then break to the REPL:
	graphqshell my-script.tengo -r`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		figure.NewFigure("GraphQShell", "doom", true).Print()
		fmt.Println()

		repl, _ := cmd.Flags().GetBool("repl")

		modules := tengomod.GetModuleMap()

		var bytecode *tengo.Bytecode
		if len(args) == 1 {
			fullPath, err := filepath.Abs(args[0])
			if err != nil {
				panic(err)
			}

			bytes, err := os.ReadFile(fullPath)
			if err != nil {
				panic(err)
			}

			if len(bytes) > 1 && string(bytes[:2]) == "#!" {
				copy(bytes, "//")
			}

			bytecode, err = compileSrc(modules, bytes, fullPath)
			if err != nil {
				panic(err)
			}

			err = run(bytecode)
			if err != nil {
				panic(err)
			}

			if !repl {
				return
			}
		}

		RunREPL(modules, os.Stdin, os.Stdout)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("repl", "r", false, "Run REPL after script runs")

	for idx, fn := range tengo.GetAllBuiltinFunctions() {
		symbolTable.DefineBuiltin(idx, fn.Name)
	}
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

	// embed println function
	symbol := symbolTable.Define("__repl_println__")
	globals[symbol.Index] = &tengo.UserFunction{
		Name: "println",
		Value: func(args ...tengo.Object) (ret tengo.Object, err error) {
			var printArgs []interface{}
			for _, arg := range args {
				if _, isUndefined := arg.(*tengo.Undefined); !isUndefined {
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

	var lines []string
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

		if strings.TrimSpace(line) == "" {
			continue
		}

		if strings.EqualFold(line, "exit") {
			fmt.Println("[!] Exiting...")
			break
		} else if strings.EqualFold(line, "reset") {
			lines = make([]string, 0)
			readline.ClearScreen(l.Stdout())
			continue
		}

		lines = append(lines, line)
		fullLine := strings.Join(lines, "\n")

		srcFile := fileSet.AddFile("repl", -1, len(fullLine))
		p := parser.NewParser(srcFile, []byte(fullLine), nil)
		file, err := p.ParseFile()
		if err != nil {
			if strings.Contains(err.Error(), "found 'EOF'") {
				continue
			}

			lines = make([]string, 0)
			_, _ = fmt.Fprintln(out, err.Error())
			continue
		}

		historyLine := strings.Join(lines, "")
		lines = make([]string, 0)

		file = addPrints(file)
		c := tengo.NewCompiler(srcFile, symbolTable, constants, modules, nil)
		if err := c.Compile(file); err != nil {
			_, _ = fmt.Fprintln(out, err.Error())
			continue
		}

		l.SaveHistory("") // This is needed in order to ensure the exact historyLine is saved
		l.SaveHistory(historyLine)

		bytecode := c.Bytecode()
		machine := tengo.NewVM(bytecode, globals, -1)
		if err := machine.Run(); err != nil {
			_, _ = fmt.Fprintln(out, err.Error())
			continue
		}
		constants = bytecode.Constants
	}
}

// CompileAndRun compiles the source code and executes it.
func CompileAndRun(modules *tengo.ModuleMap, data []byte, inputFile string) (err error) {
	bytecode, err := compileSrc(modules, data, inputFile)
	if err != nil {
		return
	}

	machine := tengo.NewVM(bytecode, nil, -1)
	err = machine.Run()
	return
}

func run(bytecode *tengo.Bytecode) error {
	machine := tengo.NewVM(bytecode, globals, -1)
	return machine.Run()
}

func compileSrc(modules *tengo.ModuleMap, src []byte, inputFile string) (*tengo.Bytecode, error) {
	fileSet := parser.NewFileSet()
	srcFile := fileSet.AddFile(filepath.Base(inputFile), -1, len(src))

	p := parser.NewParser(srcFile, src, nil)
	file, err := p.ParseFile()
	if err != nil {
		return nil, err
	}

	c := tengo.NewCompiler(srcFile, symbolTable, constants, modules, nil)
	if err := c.Compile(file); err != nil {
		return nil, err
	}

	bytecode := c.Bytecode()
	bytecode.RemoveDuplicates()
	return bytecode, nil
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
