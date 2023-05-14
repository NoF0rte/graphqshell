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
	client    *graphql.Client
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

// Call takes an arbitrary number of arguments and returns a return value
// and/or an error.
func (o *GraphQLObject) Call(args ...tengo.Object) (tengo.Object, error) {
	return makeGraphQLClient(o.client).postJSON(o)
}

// CanCall returns whether the Object can be Called.
func (o *GraphQLObject) CanCall() bool {
	return true
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

func (o *GraphQLObject) getFields() tengo.Object {
	return &GraphQLObjArray{
		Value: o.Value.Fields,
	}
}

func (o *GraphQLObject) getField(arg tengo.Object) (tengo.Object, error) {
	path, err := interop.TStrToGoStr(arg, "path")
	if err != nil {
		return nil, err
	}

	field := o.Value.GetField(path)
	if field == nil {
		return nil, nil
	}

	return makeGraphQLObject(field, o.client), nil
}

func (o *GraphQLObject) getArgs() tengo.Object {
	return &GraphQLObjArray{
		Value:  o.Value.Args,
		client: o.client,
	}
}

func (o *GraphQLObject) getArg(arg tengo.Object) (tengo.Object, error) {
	name, err := interop.TStrToGoStr(arg, "name")
	if err != nil {
		return nil, err
	}

	argObj := o.Value.GetArg(name)
	if argObj == nil {
		return nil, nil
	}

	return makeGraphQLObject(argObj, o.client), nil
}

func makeGraphQLObject(obj *graphql.Object, client *graphql.Client) *GraphQLObject {
	gqlObj := &GraphQLObject{
		Value:  obj,
		client: client,
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
			Name:  "fields",
			Value: getAllOrSingle(gqlObj.getFields, gqlObj.getField),
		},
		"args": &tengo.UserFunction{
			Name:  "args",
			Value: getAllOrSingle(gqlObj.getArgs, gqlObj.getArg),
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
