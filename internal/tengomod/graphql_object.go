package tengomod

import (
	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
)

type GraphQLObject struct {
	tengo.ObjectImpl
	Value     *graphql.Object
	objectMap map[string]tengo.Object
}

func (o *GraphQLObject) TypeName() string {
	return "graphql-obj"
}

// String should return a string representation of the type's value.
func (o *GraphQLObject) String() string {
	return o.Value.Name
}

// IsFalsy should return true if the value of the type should be considered
// as falsy.
func (o *GraphQLObject) IsFalsy() bool {
	return o.Value == nil
}

// CanIterate should return whether the Object can be Iterated.
func (o *GraphQLObject) CanIterate() bool {
	return false
}

func (o *GraphQLObject) IndexGet(index tengo.Object) (tengo.Object, error) {
	strIdx, ok := tengo.ToString(index)
	if !ok {
		return nil, tengo.ErrInvalidIndexType
	}

	res, ok := o.objectMap[strIdx]
	if !ok {
		res = tengo.UndefinedValue
	}
	return res, nil
}

func makeGraphQLObject(obj *graphql.Object) *GraphQLObject {
	gqlObj := &GraphQLObject{
		Value: obj,
	}

	objectMap := map[string]tengo.Object{

		// "gen_value": &tengo.UserFunction{
		// 	Name:  "gen_value",
		// 	Value: interop.NewCallable(gqlObj.setHeaders, interop.WithExactArgs(1)),
		// },
		// "set_value": &tengo.UserFunction{
		// 	Name:  "set_authorization",
		// 	Value: interop.NewCallable(gqlObj.setAuthorization, interop.WithExactArgs(1)),
		// },
		// "get_field": &tengo.UserFunction{
		// 	Name:  "set_bearer",
		// 	Value: interop.NewCallable(gqlObj.setBearer, interop.WithExactArgs(1)),
		// },
		// "get_arg": &tengo.UserFunction{
		// 	Name:  "set_cookies",
		// 	Value: interop.NewCallable(gqlObj.setCookies, interop.WithExactArgs(1)),
		// },
		// "gen_args": &tengo.UserFunction{
		// 	Name:  "set_proxy",
		// 	Value: interop.NewCallable(gqlObj.setProxy, interop.WithExactArgs(1)),
		// },
		// "to_graphql": &tengo.UserFunction{
		// 	Name:  "set_proxy",
		// 	Value: interop.NewCallable(gqlObj.setProxy, interop.WithExactArgs(1)),
		// },
	}

	gqlObj.objectMap = objectMap
	return gqlObj
}
