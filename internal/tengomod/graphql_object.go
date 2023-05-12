package tengomod

import (
	"bytes"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
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
	buf := new(bytes.Buffer)
	err := objSigTemplate.Execute(buf, o.Value)
	if err != nil {
		return "error ocurred during template execution"
	}

	return buf.String()
}

// IsFalsy should return true if the value of the type should be considered
// as falsy.
func (o *GraphQLObject) IsFalsy() bool {
	return o.Value == nil
}

// CanIterate should return whether the Object can be Iterated.
func (o *GraphQLObject) CanIterate() bool {
	return true
}

func (o *GraphQLObject) Iterate() tengo.Iterator {
	immutableMap := &tengo.ImmutableMap{
		Value: o.objectMap,
	}
	return immutableMap.Iterate()
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

func (o *GraphQLObject) IndexSet(index tengo.Object, val tengo.Object) error {
	strIdx, ok := tengo.ToString(index)
	if !ok {
		return tengo.ErrInvalidIndexType
	}

	if strIdx != "description" {
		return tengo.ErrInvalidIndexOnError
	}

	value, ok := tengo.ToString(val)
	if !ok {
		return tengo.ErrInvalidIndexValueType
	}

	o.Value.Description = value
	o.objectMap[strIdx] = val

	return nil
}

func (o *GraphQLObject) funcAROs(fn func() []*graphql.Object) func(...tengo.Object) (tengo.Object, error) {
	return func(args ...tengo.Object) (tengo.Object, error) {
		var objs []tengo.Object
		for _, obj := range fn() {
			objs = append(objs, makeGraphQLObject(obj))
		}

		return &tengo.ImmutableArray{
			Value: objs,
		}, nil
	}
}

func (o *GraphQLObject) genValue(args ...tengo.Object) (tengo.Object, error) {
	val := o.Value.GenValue()
	return tengo.FromInterface(val)
}

func (o *GraphQLObject) setValue(args ...tengo.Object) (tengo.Object, error) {
	val := tengo.ToInterface(args[0])
	o.Value.SetValue(val)
	return nil, nil
}

func (o *GraphQLObject) getField(args ...tengo.Object) (tengo.Object, error) {
	path, err := interop.TStrToGoStr(args[0], "path")
	if err != nil {
		return nil, err
	}

	obj := o.Value.GetField(path)
	if obj == nil {
		return nil, nil
	}

	return makeGraphQLObject(obj), nil
}

func (o *GraphQLObject) getArg(args ...tengo.Object) (tengo.Object, error) {
	name, err := interop.TStrToGoStr(args[0], "name")
	if err != nil {
		return nil, err
	}

	obj := o.Value.GetArg(name)
	if obj == nil {
		return nil, nil
	}

	return makeGraphQLObject(obj), nil
}

func (o *GraphQLObject) genArgs(args ...tengo.Object) (tengo.Object, error) {
	generatedArgs := o.Value.GenArgs()
	return tengo.FromInterface(generatedArgs)
}

func (o *GraphQLObject) genArg(args ...tengo.Object) (tengo.Object, error) {
	name, err := interop.TStrToGoStr(args[0], "name")
	if err != nil {
		return nil, err
	}

	arg := o.Value.GenArg(name)
	return tengo.FromInterface(arg)
}

func (o *GraphQLObject) toGraphQL(args ...tengo.Object) (tengo.Object, error) {
	output, err := o.Value.ToGraphQL()
	if err != nil {
		return interop.GoErrToTErr(err), nil
	}

	return interop.GoStrToTStr(output), nil
}

func makeGraphQLObject(obj *graphql.Object) *GraphQLObject {
	gqlObj := &GraphQLObject{
		Value: obj,
	}

	objectMap := map[string]tengo.Object{
		"name": &tengo.String{
			Value: obj.Name,
		},
		"description": &tengo.String{
			Value: obj.Description,
		},
		"type": &tengo.String{
			Value: obj.Type.String(),
		},
		"gen_value": &tengo.UserFunction{
			Name:  "gen_value",
			Value: gqlObj.genValue,
		},
		"set_value": &tengo.UserFunction{
			Name:  "set_value",
			Value: interop.NewCallable(gqlObj.setValue, interop.WithExactArgs(1)),
		},
		"fields": &tengo.UserFunction{
			Name: "fields",
			Value: gqlObj.funcAROs(func() []*graphql.Object {
				return obj.Fields
			}),
		},
		"get_field": &tengo.UserFunction{
			Name:  "get_field",
			Value: interop.NewCallable(gqlObj.getField, interop.WithExactArgs(1)),
		},
		"args": &tengo.UserFunction{
			Name: "args",
			Value: gqlObj.funcAROs(func() []*graphql.Object {
				return obj.Args
			}),
		},
		"get_arg": &tengo.UserFunction{
			Name:  "get_arg",
			Value: interop.NewCallable(gqlObj.getArg, interop.WithExactArgs(1)),
		},
		"gen_args": &tengo.UserFunction{
			Name:  "gen_args",
			Value: gqlObj.genArgs,
		},
		"gen_arg": &tengo.UserFunction{
			Name:  "gen_arg",
			Value: interop.NewCallable(gqlObj.genArg, interop.WithExactArgs(1)),
		},
		"to_graphql": &tengo.UserFunction{
			Name:  "to_graphql",
			Value: gqlObj.toGraphQL,
		},
	}

	gqlObj.objectMap = objectMap
	return gqlObj
}
