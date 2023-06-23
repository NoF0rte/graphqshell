package tengomod

import (
	"fmt"
	"io"

	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengo/v2/stdlib"
	"github.com/analog-substance/tengomod"
	"github.com/analog-substance/tengomod/interop"
)

func GetModuleMap() *tengo.ModuleMap {
	moduleMap := stdlib.GetModuleMap(stdlib.AllModuleNames()...)
	moduleMap.AddMap(tengomod.GetModuleMap())
	moduleMap.AddBuiltinModule("graphql", graphqlModule())

	return moduleMap
}

func Spew(out io.Writer) *interop.AdvFunction {
	callable := func(args interop.ArgMap) (tengo.Object, error) {
		arg, _ := args.GetObject("obj")
		if arg.CanIterate() {
			iterator := arg.Iterate()
			for iterator.Next() {
				keyObj := iterator.Key()
				key, _ := tengo.ToString(keyObj)

				valueObj := iterator.Value()

				if keyObj != nil && valueObj != nil {
					fmt.Fprintf(out, "%s: %s\n", key, valueObj.TypeName())
				} else if keyObj != nil {
					fmt.Fprintf(out, "%s: %s\n", key, keyObj.TypeName())
				} else if valueObj != nil {
					value, _ := tengo.ToString(valueObj)
					fmt.Fprintf(out, "%s: %s\n", value, valueObj.TypeName())
				}
			}
		}
		return nil, nil
	}

	return &interop.AdvFunction{
		Name:    "spew",
		NumArgs: interop.ExactArgs(1),
		Args:    []interop.AdvArg{interop.ObjectArg("obj")},
		Value:   callable,
	}
}

func getterSetter(get func() tengo.Object, set func(tengo.Object) error) func(...tengo.Object) (tengo.Object, error) {
	f := &interop.AdvFunction{
		NumArgs: interop.MaxArgs(1),
		Args:    []interop.AdvArg{interop.ObjectArg("arg")},
		Value: func(args interop.ArgMap) (tengo.Object, error) {
			arg, ok := args.GetObject("arg")
			if !ok {
				return get(), nil
			}

			err := set(arg)
			if err != nil {
				return interop.GoErrToTErr(err), nil
			}
			return nil, nil
		},
	}
	return f.Call
}

func getAllOrSingle(allFn func() tengo.Object, singleFn func(tengo.Object) (tengo.Object, error)) func(...tengo.Object) (tengo.Object, error) {
	f := &interop.AdvFunction{
		NumArgs: interop.MaxArgs(1),
		Args:    []interop.AdvArg{interop.ObjectArg("arg")},
		Value: func(args interop.ArgMap) (tengo.Object, error) {
			arg, ok := args.GetObject("arg")
			if !ok {
				return allFn(), nil
			}

			single, err := singleFn(arg)
			if err != nil {
				return interop.GoErrToTErr(err), nil
			}
			return single, nil
		},
	}
	return f.Call
}
