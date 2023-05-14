package tengomod

import (
	"bytes"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
)

type GraphQLRootQuery struct {
	tengo.ObjectImpl
	Value     *graphql.RootQuery
	objectMap map[string]tengo.Object
	client    *graphql.Client
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

func (q *GraphQLRootQuery) Iterate() tengo.Iterator {
	immutableMap := &tengo.ImmutableMap{
		Value: q.objectMap,
	}
	return immutableMap.Iterate()
}

func (q *GraphQLRootQuery) IndexGet(index tengo.Object) (tengo.Object, error) {
	strIdx, ok := tengo.ToString(index)
	if !ok {
		return nil, tengo.ErrInvalidIndexType
	}

	res, ok := q.objectMap[strIdx]
	if !ok {
		res = tengo.UndefinedValue
	}
	return res, nil
}

func (q *GraphQLRootQuery) get(args ...tengo.Object) (tengo.Object, error) {
	name, err := interop.TStrToGoStr(args[0], "name")
	if err != nil {
		return nil, err
	}

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
		"get": &tengo.UserFunction{
			Name:  "get",
			Value: interop.NewCallable(rootQuery.get, interop.WithExactArgs(1)),
		},
	}

	for _, obj := range query.Queries {
		objectMap[obj.Name] = makeGraphQLObject(obj, client)
	}

	rootQuery.objectMap = objectMap
	return rootQuery
}
