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

func (c *GraphQLClient) setHeaders(arg tengo.Object) error {
	headers, err := interop.TMapToGoStrMapStr(arg, "headers")
	if err != nil {
		return err
	}

	c.Value.SetHeaders(headers)

	return nil
}

func (c *GraphQLClient) getHeaders() tengo.Object {
	return interop.GoStrMapStrToTMap(c.Value.GetHeaders())
}

func (c *GraphQLClient) setAuthorization(arg tengo.Object) error {
	auth, err := interop.TStrToGoStr(arg, "auth")
	if err != nil {
		return err
	}

	c.Value.SetAuth(auth)

	return nil
}

func (c *GraphQLClient) getAuthorization() tengo.Object {
	return interop.GoStrToTStr(c.Value.GetAuth())
}

func (c *GraphQLClient) setBearer(arg tengo.Object) error {
	token, err := interop.TStrToGoStr(arg, "token")
	if err != nil {
		return err
	}

	c.Value.SetBearer(token)

	return nil
}

func (c *GraphQLClient) getBearer() tengo.Object {
	return interop.GoStrToTStr(c.Value.GetBearer())
}

func (c *GraphQLClient) setCookies(arg tengo.Object) error {
	cookies, err := interop.TStrToGoStr(arg, "cookies")
	if err != nil {
		return err
	}

	c.Value.SetCookies(cookies)

	return nil
}

func (c *GraphQLClient) getCookies() tengo.Object {
	return interop.GoStrToTStr(c.Value.GetCookies())
}

func (c *GraphQLClient) setProxy(arg tengo.Object) error {
	proxyURL, err := interop.TStrToGoStr(arg, "url")
	if err != nil {
		return err
	}

	err = c.Value.SetProxy(proxyURL)
	if err != nil {
		return err
	}

	return nil
}

func (c *GraphQLClient) getProxy() tengo.Object {
	return interop.GoStrToTStr(c.Value.GetProxy())
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

func (c *GraphQLClient) introspect(args ...tengo.Object) (tengo.Object, error) {
	query, mutation, err := c.Value.Introspect()
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

func makeGraphQLClient(client *graphql.Client) *GraphQLClient {
	gqlClient := &GraphQLClient{
		Value: client,
	}

	objectMap := map[string]tengo.Object{
		"headers": &tengo.UserFunction{
			Name:  "headers",
			Value: getterSetter(gqlClient.getHeaders, gqlClient.setHeaders),
		},
		"auth": &tengo.UserFunction{
			Name:  "auth",
			Value: getterSetter(gqlClient.getAuthorization, gqlClient.setAuthorization),
		},
		"bearer": &tengo.UserFunction{
			Name:  "bearer",
			Value: getterSetter(gqlClient.getBearer, gqlClient.setBearer),
		},
		"cookies": &tengo.UserFunction{
			Name:  "cookies",
			Value: getterSetter(gqlClient.getCookies, gqlClient.setCookies),
		},
		"proxy": &tengo.UserFunction{
			Name:  "proxy",
			Value: getterSetter(gqlClient.getProxy, gqlClient.setProxy),
		},
		"post_json": &tengo.UserFunction{
			Name:  "post_json",
			Value: interop.NewCallable(gqlClient.postJSON, interop.WithExactArgs(1)),
		},
		"post_graphql": &tengo.UserFunction{
			Name:  "post_graphql",
			Value: interop.NewCallable(gqlClient.postGraphQL, interop.WithExactArgs(1)),
		},
		"introspect": &tengo.UserFunction{
			Name:  "introspect",
			Value: gqlClient.introspect,
		},
	}

	gqlClient.objectMap = objectMap
	return gqlClient
}
