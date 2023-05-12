package tengomod

import (
	"encoding/json"

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
		"parse": &tengo.UserFunction{
			Name:  "parse",
			Value: interop.NewCallable(parseIntrospection, interop.WithExactArgs(1)),
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

func parseIntrospection(args ...tengo.Object) (tengo.Object, error) {
	data, err := interop.TStrToGoStr(args[0], "data")
	if err != nil {
		dataMap, ok := tengo.ToInterface(args[0]).(map[string]interface{})
		if !ok {
			return nil, tengo.ErrInvalidArgumentType{
				Name:     "data",
				Expected: "map(compatible)",
				Found:    args[0].TypeName(),
			}
		}

		bytes, err := json.Marshal(dataMap)
		if err != nil {
			return interop.GoErrToTErr(err), nil
		}

		data = string(bytes)
	}

	var introspection graphql.IntrospectionResponse
	err = json.Unmarshal([]byte(data), &introspection)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	query, mutation, err := graphql.ParseIntrospection(introspection)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	objMap := make(map[string]tengo.Object)
	if query != nil {
		objMap["query"] = makeGraphQLRootQuery(query)
	}

	if mutation != nil {
		objMap["mutation"] = makeGraphQLRootMutation(mutation)
	}

	return &tengo.ImmutableMap{
		Value: objMap,
	}, nil
}
