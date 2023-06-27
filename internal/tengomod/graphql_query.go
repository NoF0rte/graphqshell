package tengomod

import (
	"bytes"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
	"github.com/analog-substance/tengomod/types"
)

type GraphQLRootQuery struct {
	types.PropObject
	Value  *graphql.RootQuery
	client *graphql.Client
}

func (q *GraphQLRootQuery) TypeName() string {
	return "graphql-root-query"
}

// String should return a string representation of the type's value.
func (q *GraphQLRootQuery) String() string {
	context := struct {
		Name  string
		Items []*graphql.Object
	}{
		Name:  q.Value.Name,
		Items: q.Value.Queries,
	}

	buf := new(bytes.Buffer)
	err := rootSigTemplate.Execute(buf, context)
	if err != nil {
		return "error ocurred during template execution"
	}

	return buf.String()
}

// IsFalsy should return true if the value of the type should be considered
// as falsy.
func (q *GraphQLRootQuery) IsFalsy() bool {
	return q.Value == nil
}

// CanIterate should return whether the Object can be Iterated.
func (q *GraphQLRootQuery) CanIterate() bool {
	return true
}

func (q *GraphQLRootQuery) get(args interop.ArgMap) (tengo.Object, error) {
	name, _ := args.GetString("name")
	obj := q.Value.Get(name)
	if obj == nil {
		return nil, nil
	}

	return makeGraphQLObject(obj, q.client), nil
}

func (q *GraphQLRootQuery) queries(args ...tengo.Object) (tengo.Object, error) {
	var objs []tengo.Object
	for _, obj := range q.Value.Queries {
		objs = append(objs, makeGraphQLObject(obj, q.client))
	}

	return &tengo.ImmutableArray{
		Value: objs,
	}, nil
}

func makeGraphQLRootQuery(query *graphql.RootQuery, client *graphql.Client) *GraphQLRootQuery {
	if query == nil {
		return nil
	}

	rootQuery := &GraphQLRootQuery{
		Value:  query,
		client: client,
	}

	objectMap := map[string]tengo.Object{
		"name": &tengo.String{
			Value: query.Name,
		},
		"queries": &tengo.UserFunction{
			Name:  "queries",
			Value: rootQuery.queries,
		},
		"get": &interop.AdvFunction{
			Name:    "get",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.StrArg("name")},
			Value:   rootQuery.get,
		},
	}

	for _, obj := range query.Queries {
		objectMap[obj.Name] = makeGraphQLObject(obj, client)
	}

	rootQuery.PropObject = types.PropObject{
		ObjectMap:  objectMap,
		Properties: make(map[string]types.Property),
	}

	return rootQuery
}
