package tengomod

import (
	"encoding/json"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
	"github.com/analog-substance/tengomod/types"
)

type GraphQLClient struct {
	types.PropObject
	Value *graphql.Client
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
		Value: c.ObjectMap,
	}
	return immutableMap.Iterate()
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

func (c *GraphQLClient) postJSON(args interop.ArgMap) (tengo.Object, error) {
	obj, _ := args.GetObject("obj")
	graphqlObj := obj.(*GraphQLObject)

	body, _, err := c.Value.PostJSON(graphqlObj.Value)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	data := make(map[string]interface{})
	err = json.Unmarshal([]byte(body), &data)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return tengo.FromInterface(data)
}

func (c *GraphQLClient) postGraphQL(args interop.ArgMap) (tengo.Object, error) {
	obj, _ := args.GetObject("obj")
	graphqlObj := obj.(*GraphQLObject)

	body, _, err := c.Value.PostGraphQL(graphqlObj.Value)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	data := make(map[string]interface{})
	err = json.Unmarshal([]byte(body), &data)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return tengo.FromInterface(data)
}

func (c *GraphQLClient) introspectAndParse(args ...tengo.Object) (tengo.Object, error) {
	query, mutation, err := c.Value.IntrospectAndParse()
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	objMap := make(map[string]tengo.Object)
	if query != nil {
		objMap["query"] = makeGraphQLRootQuery(query, c.Value)
	}

	if mutation != nil {
		objMap["mutation"] = makeGraphQLRootMutation(mutation, c.Value)
	}

	return &tengo.ImmutableMap{
		Value: objMap,
	}, nil
}

func (c *GraphQLClient) introspect(args ...tengo.Object) (tengo.Object, error) {
	body, _, err := c.Value.IntrospectRaw()
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	introspection := make(map[string]interface{})
	err = json.Unmarshal([]byte(body), &introspection)
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return tengo.FromInterface(introspection)
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
		"post_json": &interop.AdvFunction{
			Name:    "post_json",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.CustomArg("obj", &GraphQLObject{})},
			Value:   gqlClient.postJSON,
		},
		"post_graphql": &interop.AdvFunction{
			Name:    "post_graphql",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.CustomArg("obj", &GraphQLObject{})},
			Value:   gqlClient.postGraphQL,
		},
		"introspect_and_parse": &tengo.UserFunction{
			Name:  "introspect_and_parse",
			Value: gqlClient.introspectAndParse,
		},
		"introspect": &tengo.UserFunction{
			Name:  "introspect",
			Value: gqlClient.introspect,
		},
	}

	gqlClient.PropObject = types.PropObject{
		ObjectMap:  objectMap,
		Properties: make(map[string]types.Property),
	}

	return gqlClient
}
