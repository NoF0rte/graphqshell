package tengomod

import (
	"encoding/json"
	"os"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
)

var defaultClient *graphql.Client

func graphqlModule() map[string]tengo.Object {
	return map[string]tengo.Object{
		"new_client": &interop.AdvFunction{
			Name:    "new_client",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.StrArg("url")},
			Value:   newGraphQLClient,
		},
		"set_debug": &interop.AdvFunction{
			Name:    "set_debug",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.BoolArg("debug")},
			Value:   setDebug,
		},
		"set_client": &interop.AdvFunction{
			Name:    "set_client",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.CustomArg("client", &GraphQLClient{})},
			Value:   setClient,
		},
		"parse": &interop.AdvFunction{
			Name:    "parse",
			NumArgs: interop.ExactArgs(1),
			Args: []interop.AdvArg{interop.UnionArg("data", interop.StrType, func(obj tengo.Object, name string) (interface{}, error) {
				dataMap, ok := tengo.ToInterface(obj).(map[string]interface{})
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{
						Name:     name,
						Expected: "map(compatible)",
						Found:    obj.TypeName(),
					}
				}

				bytes, err := json.Marshal(dataMap)
				if err != nil {
					return nil, err
				}

				return string(bytes), nil
			})},
			Value: parseIntrospection,
		},
		"parse_file": &interop.AdvFunction{
			Name:    "parse_file",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.StrArg("file")},
			Value:   parseIntrospectionFile,
		},
	}
}

func setDebug(args interop.ArgMap) (tengo.Object, error) {
	debug, _ := args.GetBool("debug")
	graphql.Debug = debug

	return nil, nil
}

func newGraphQLClient(args interop.ArgMap) (tengo.Object, error) {
	u, _ := args.GetString("url")
	client := graphql.NewClient(u)
	return makeGraphQLClient(client), nil
}

func parseIntrospection(args interop.ArgMap) (tengo.Object, error) {
	data, _ := args.GetString("data")

	var introspection graphql.IntrospectionResponse
	err := json.Unmarshal([]byte(data), &introspection)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	query, mutation, err := graphql.Parse(introspection)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	objMap := make(map[string]tengo.Object)
	if query != nil {
		objMap["query"] = makeGraphQLRootQuery(query, nil)
	}

	if mutation != nil {
		objMap["mutation"] = makeGraphQLRootMutation(mutation, nil)
	}

	return &tengo.ImmutableMap{
		Value: objMap,
	}, nil
}

func parseIntrospectionFile(args interop.ArgMap) (tengo.Object, error) {
	file, _ := args.GetString("file")

	data, err := os.ReadFile(file)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return parseIntrospection(interop.ArgMap{
		"data": string(data),
	})
}

func setClient(args interop.ArgMap) (tengo.Object, error) {
	obj, _ := args.Get("client")
	client := obj.(*GraphQLClient)

	defaultClient = client.Value
	return nil, nil
}
