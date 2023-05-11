package tengomod

import (
	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
)

type GraphQLRootMutation struct {
	tengo.ObjectImpl
	Value     *graphql.RootMutation
	objectMap map[string]tengo.Object
}

func (m *GraphQLRootMutation) TypeName() string {
	return "graphql-root-mutation"
}

// String should return a string representation of the type's value.
func (m *GraphQLRootMutation) String() string {
	return m.Value.Name
}

// IsFalsy should return true if the value of the type should be considered
// as falsy.
func (m *GraphQLRootMutation) IsFalsy() bool {
	return m.Value == nil
}

// CanIterate should return whether the Object can be Iterated.
func (m *GraphQLRootMutation) CanIterate() bool {
	return true
}

func (m *GraphQLRootMutation) Iterate() tengo.Iterator {
	immutableMap := &tengo.ImmutableMap{
		Value: m.objectMap,
	}
	return immutableMap.Iterate()
}

func (m *GraphQLRootMutation) IndexGet(index tengo.Object) (tengo.Object, error) {
	strIdx, ok := tengo.ToString(index)
	if !ok {
		return nil, tengo.ErrInvalidIndexType
	}

	res, ok := m.objectMap[strIdx]
	if !ok {
		res = tengo.UndefinedValue
	}
	return res, nil
}

func (m *GraphQLRootMutation) get(args ...tengo.Object) (tengo.Object, error) {
	name, err := interop.TStrToGoStr(args[0], "name")
	if err != nil {
		return nil, err
	}

	obj := m.Value.Get(name)
	if obj == nil {
		return nil, nil
	}

	return makeGraphQLObject(obj), nil
}

func (q *GraphQLRootMutation) mutations(args ...tengo.Object) (tengo.Object, error) {
	var objs []tengo.Object
	for _, obj := range q.Value.Mutations {
		objs = append(objs, makeGraphQLObject(obj))
	}

	return &tengo.ImmutableArray{
		Value: objs,
	}, nil
}

func makeGraphQLRootMutation(mutation *graphql.RootMutation) *GraphQLRootMutation {
	if mutation == nil {
		return nil
	}

	rootMutation := &GraphQLRootMutation{
		Value: mutation,
	}

	objectMap := map[string]tengo.Object{
		"name": &tengo.String{
			Value: mutation.Name,
		},
		"mutations": &tengo.UserFunction{
			Name:  "mutations",
			Value: rootMutation.mutations,
		},
		"get": &tengo.UserFunction{
			Name:  "get",
			Value: interop.NewCallable(rootMutation.get, interop.WithExactArgs(1)),
		},
	}

	rootMutation.objectMap = objectMap
	return rootMutation
}