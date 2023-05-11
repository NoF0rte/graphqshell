package tengomod

import (
	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
)

type GraphQLClient struct {
	tengo.ObjectImpl
	Value     *graphql.Client
	objectMap map[string]tengo.Object
}

func (c *GraphQLClient) TypeName() string {
	return "graphql-client"
}

// String should return a string representation of the type's value.
func (c *GraphQLClient) String() string {
	return "<graphql-client>"
}

// IsFalsy should return true if the value of the type should be considered
// as falsy.
func (c *GraphQLClient) IsFalsy() bool {
	return c.Value == nil
}

// CanIterate should return whether the Object can be Iterated.
func (c *GraphQLClient) CanIterate() bool {
	return true
}

func (c *GraphQLClient) Iterate() tengo.Iterator {
	immutableMap := &tengo.ImmutableMap{
		Value: c.objectMap,
	}
	return immutableMap.Iterate()
}

func (c *GraphQLClient) IndexGet(index tengo.Object) (tengo.Object, error) {
	strIdx, ok := tengo.ToString(index)
	if !ok {
		return nil, tengo.ErrInvalidIndexType
	}

	res, ok := c.objectMap[strIdx]
	if !ok {
		res = tengo.UndefinedValue
	}
	return res, nil
}

func (c *GraphQLClient) setHeaders(args ...tengo.Object) (tengo.Object, error) {
	headers, err := interop.TMapToGoStrMapStr(args[0], "headers")
	if err != nil {
		return nil, err
	}

	c.Value.SetHeaders(headers)

	return nil, nil
}

func (c *GraphQLClient) setAuthorization(args ...tengo.Object) (tengo.Object, error) {
	auth, err := interop.TStrToGoStr(args[0], "auth")
	if err != nil {
		return nil, err
	}

	c.Value.SetAuthorization(auth)

	return nil, nil
}

func (c *GraphQLClient) setBearer(args ...tengo.Object) (tengo.Object, error) {
	token, err := interop.TStrToGoStr(args[0], "token")
	if err != nil {
		return nil, err
	}

	c.Value.SetBearer(token)

	return nil, nil
}

func (c *GraphQLClient) setCookies(args ...tengo.Object) (tengo.Object, error) {
	cookies, err := interop.TStrToGoStr(args[0], "cookies")
	if err != nil {
		return nil, err
	}

	c.Value.SetCookies(cookies)

	return nil, nil
}

func (c *GraphQLClient) setProxy(args ...tengo.Object) (tengo.Object, error) {
	proxyURL, err := interop.TStrToGoStr(args[0], "url")
	if err != nil {
		return nil, err
	}

	err = c.Value.SetProxy(proxyURL)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return nil, nil
}

func (c *GraphQLClient) postJSON(args ...tengo.Object) (tengo.Object, error) {
	obj, ok := args[0].(*GraphQLObject)
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "object",
			Expected: "graphql-obj",
			Found:    args[0].TypeName(),
		}
	}

	body, _, err := c.Value.PostJSON(obj.Value)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"body": &tengo.String{
				Value: body,
			},
		},
	}, nil
}

func (c *GraphQLClient) postGraphQL(args ...tengo.Object) (tengo.Object, error) {
	obj, ok := args[0].(*GraphQLObject)
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "object",
			Expected: "graphql-obj",
			Found:    args[0].TypeName(),
		}
	}

	body, _, err := c.Value.PostGraphQL(obj.Value)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"body": &tengo.String{
				Value: body,
			},
		},
	}, nil
}

func (c *GraphQLClient) sendAndParseIntrospection(args ...tengo.Object) (tengo.Object, error) {
	query, mutation, err := c.Value.SendAndParseIntrospection()
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	objMap := make(map[string]tengo.Object)
	if query != nil {
		objMap["root_query"] = makeGraphQLRootQuery(query)
	}

	if mutation != nil {
		objMap["root_mutation"] = makeGraphQLRootMutation(mutation)
	}

	return &tengo.ImmutableMap{
		Value: objMap,
	}, nil
}

func makeGraphQLClient(client *graphql.Client) *GraphQLClient {
	gqlClient := &GraphQLClient{
		Value: client,
	}

	objectMap := map[string]tengo.Object{
		"set_headers": &tengo.UserFunction{
			Name:  "set_headers",
			Value: interop.NewCallable(gqlClient.setHeaders, interop.WithExactArgs(1)),
		},
		"set_authorization": &tengo.UserFunction{
			Name:  "set_authorization",
			Value: interop.NewCallable(gqlClient.setAuthorization, interop.WithExactArgs(1)),
		},
		"set_bearer": &tengo.UserFunction{
			Name:  "set_bearer",
			Value: interop.NewCallable(gqlClient.setBearer, interop.WithExactArgs(1)),
		},
		"set_cookies": &tengo.UserFunction{
			Name:  "set_cookies",
			Value: interop.NewCallable(gqlClient.setCookies, interop.WithExactArgs(1)),
		},
		"set_proxy": &tengo.UserFunction{
			Name:  "set_proxy",
			Value: interop.NewCallable(gqlClient.setProxy, interop.WithExactArgs(1)),
		},
		"post_json": &tengo.UserFunction{
			Name:  "post_json",
			Value: interop.NewCallable(gqlClient.postJSON, interop.WithExactArgs(1)),
		},
		"post_graphql": &tengo.UserFunction{
			Name:  "post_graphql",
			Value: interop.NewCallable(gqlClient.postGraphQL, interop.WithExactArgs(1)),
		},
		"send_and_parse_introspection": &tengo.UserFunction{
			Name:  "send_and_parse_introspection",
			Value: gqlClient.sendAndParseIntrospection,
		},
	}

	gqlClient.objectMap = objectMap
	return gqlClient
}
