package tengomod

import (
	"bytes"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
	"github.com/analog-substance/tengomod/types"
)

type GraphQLRootMutation struct {
	types.PropObject
	Value  *graphql.RootMutation
	client *graphql.Client
}

func (m *GraphQLRootMutation) TypeName() string {
	return "graphql-root-mutation"
}

// String should return a string representation of the type's value.
func (m *GraphQLRootMutation) String() string {
	context := struct {
		Name  string
		Items []*graphql.Object
	}{
		Name:  m.Value.Name,
		Items: m.Value.Mutations,
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
func (m *GraphQLRootMutation) IsFalsy() bool {
	return m.Value == nil
}

// CanIterate should return whether the Object can be Iterated.
func (m *GraphQLRootMutation) CanIterate() bool {
	return true
}

func (m *GraphQLRootMutation) Iterate() tengo.Iterator {
	immutableMap := &tengo.ImmutableMap{
		Value: m.ObjectMap,
	}
	return immutableMap.Iterate()
}

func (m *GraphQLRootMutation) get(args interop.ArgMap) (tengo.Object, error) {
	name, _ := args.GetString("name")
	obj := m.Value.Get(name)
	if obj == nil {
		return nil, nil
	}

	return makeGraphQLObject(obj, m.client), nil
}

func (m *GraphQLRootMutation) mutations(args ...tengo.Object) (tengo.Object, error) {
	var objs []tengo.Object
	for _, obj := range m.Value.Mutations {
		objs = append(objs, makeGraphQLObject(obj, m.client))
	}

	return &tengo.ImmutableArray{
		Value: objs,
	}, nil
}

func makeGraphQLRootMutation(mutation *graphql.RootMutation, client *graphql.Client) *GraphQLRootMutation {
	if mutation == nil {
		return nil
	}

	rootMutation := &GraphQLRootMutation{
		Value:  mutation,
		client: client,
	}

	objectMap := map[string]tengo.Object{
		"name": &tengo.String{
			Value: mutation.Name,
		},
		"mutations": &tengo.UserFunction{
			Name:  "mutations",
			Value: rootMutation.mutations,
		},
		"get": &interop.AdvFunction{
			Name:    "get",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.StrArg("name")},
			Value:   rootMutation.get,
		},
	}

	for _, obj := range mutation.Mutations {
		objectMap[obj.Name] = makeGraphQLObject(obj, client)
	}

	rootMutation.PropObject = types.PropObject{
		ObjectMap:  objectMap,
		Properties: make(map[string]types.Property),
	}

	return rootMutation
}
