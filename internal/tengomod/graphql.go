package tengomod

import (
	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
)

func graphqlModule() map[string]tengo.Object {
	return map[string]tengo.Object{
		"new_client": &tengo.UserFunction{
			Name:  "new_client",
			Value: interop.NewCallable(newGraphQLClient, interop.WithExactArgs(1)),
		},
		"set_debug": &tengo.UserFunction{
			Name: "set_debug",
			Value: interop.NewCallable(func(args ...tengo.Object) (ret tengo.Object, err error) {
				debug, ok := tengo.ToBool(args[0])
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{
						Name:     "debug",
						Expected: "bool(compatible)",
						Found:    args[0].TypeName(),
					}
				}

				graphql.Debug = debug

				return nil, nil
			}, interop.WithExactArgs(1)),
		},
	}
}

func newGraphQLClient(args ...tengo.Object) (tengo.Object, error) {
	u, err := interop.TStrToGoStr(args[0], "url")
	if err != nil {
		return nil, err
	}

	client := graphql.NewClient(u)
	return makeGraphQLClient(client), nil
}
