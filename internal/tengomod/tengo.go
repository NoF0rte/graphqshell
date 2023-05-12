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

func Spew(out io.Writer) *tengo.UserFunction {
	callable := func(args ...tengo.Object) (tengo.Object, error) {
		arg := args[0]
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

	return &tengo.UserFunction{
		Name:  "spew",
		Value: interop.NewCallable(callable, interop.WithExactArgs(1)),
	}
}

func getterSetter(get func() tengo.Object, set func(tengo.Object) error) func(...tengo.Object) (tengo.Object, error) {
	callable := func(args ...tengo.Object) (tengo.Object, error) {
		if len(args) == 0 {
			return get(), nil
		}

		err := set(args[0])
		if err != nil {
			return interop.GoErrToTErr(err), nil
		}
		return nil, nil
	}
	return interop.NewCallable(callable, interop.WithMaxArgs(1))
}
