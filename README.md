# GraphQShell
GraphQShell is a GraphQL pentesting scripting engine. Using the Go scripting engine [Tengo](https://github.com/d5/tengo) and custom Tengo modules, GraphQShell enables you to interact with GraphQL endpoints with ease. Either by writing a script or using the REPL, you can use GraphQShell to more easily fuzz GraphQL queries and mutations.

## Getting Started
### Install
GraphQShell requires Go v1.20+ to install

```
go install github.com/NoF0rte/graphqshell/cmd/graphqshell@latest
```
```
$ graphqshell --help
A GraphQL pentesting scripting engine. Run a script and/or run the REPL.

Usage:
  graphqshell [path/to/script] [flags]

Examples:
Run a script:
	graphqshell my-script.tengo

Run the REPL:
	graphqshell

Run a script then break to the REPL:
	graphqshell my-script.tengo -r

Flags:
  -h, --help   help for graphqshell
  -r, --repl   Run REPL after script runs

```

### REPL
By default, GraphQShell runs a Tengo REPL. This allows you to run code and get immediate feedback.
```
$ graphqshell
 _____                      _      _____  _____  _            _  _
|  __ \                    | |    |  _  |/  ___|| |          | || |
| |  \/ _ __   __ _  _ __  | |__  | | | |\ `--. | |__    ___ | || |
| | __ | '__| / _` || '_ \ | '_ \ | | | | `--. \| '_ \  / _ \| || |
| |_\ \| |   | (_| || |_) || | | |\ \/' //\__/ /| | | ||  __/| || |
 \____/|_|    \__,_|| .__/ |_| |_| \_/\_\\____/ |_| |_| \___||_||_|
                    | |
                    |_|

» 
```

To get started, let's first create a GraphQL client
```
» client := graphql.new_client("")
```

### Examples

## Scripting
GraphQShell uses the Go scripting engine [Tengo](https://github.com/d5/tengo) and the custom Tengo modules in [tengomod](https://github.com/analog-substance/tengomod). Refer to their documentation for general usage and examples.

### Module - "graphql"

```golang
graphql := import("graphql")
```

**Note:** This module is auto imported when using the REPL

#### Functions


## Roadmap
- [ ] Fuzzing values via Go templates
- [ ] Tab completions (maybe, could more effort than it is worth)
- [ ] Smarter/configurable default argument values
- [ ] GraphQL variables
- [ ] PostJSON/GraphQL should return a result object
- [ ] More intuitive Tengo functionality
  - [ ] Settings arg default values
- [ ] Create/update schema from user supplied information
