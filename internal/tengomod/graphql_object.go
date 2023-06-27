package tengomod

import (
	"bytes"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengomod/interop"
	"github.com/analog-substance/tengomod/types"
)

type GraphQLObject struct {
	types.PropObject
	Value  *graphql.Object
	client *graphql.Client
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

// Call takes an arbitrary number of arguments and returns a return value
// and/or an error.
func (o *GraphQLObject) Call(args ...tengo.Object) (tengo.Object, error) {
	client := o.client
	if client == nil {
		client = defaultClient
	}

	return makeGraphQLClient(client).postJSON(interop.ArgMap{
		"obj": o,
	})
}

// CanCall returns whether the Object can be Called.
func (o *GraphQLObject) CanCall() bool {
	return true
}

func (o *GraphQLObject) genValue(args ...tengo.Object) (tengo.Object, error) {
	val := o.Value.GenValue()
	return tengo.FromInterface(val)
}

func (o *GraphQLObject) setValue(args interop.ArgMap) (tengo.Object, error) {
	arg, _ := args.GetObject("val")
	val := tengo.ToInterface(arg)
	o.Value.SetValue(val)
	return nil, nil
}

func (o *GraphQLObject) genArgs(args ...tengo.Object) (tengo.Object, error) {
	generatedArgs := o.Value.GenArgs()
	return tengo.FromInterface(generatedArgs)
}

func (o *GraphQLObject) genArg(args interop.ArgMap) (tengo.Object, error) {
	name, _ := args.GetString("name")
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
		"gen_value": &tengo.UserFunction{
			Name:  "gen_value",
			Value: gqlObj.genValue,
		},
		"set_value": &interop.AdvFunction{
			Name:    "set_value",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.ObjectArg("val")},
			Value:   gqlObj.setValue,
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
		"gen_arg": &interop.AdvFunction{
			Name:    "gen_arg",
			NumArgs: interop.ExactArgs(1),
			Args:    []interop.AdvArg{interop.StrArg("name")},
			Value:   gqlObj.genArg,
		},
		"to_graphql": &tengo.UserFunction{
			Name:  "to_graphql",
			Value: gqlObj.toGraphQL,
		},
	}

	properties := map[string]types.Property{
		"name": types.StaticProperty(interop.GoStrToTStr(obj.Name)),
		"type": types.StaticProperty(interop.GoStrToTStr(obj.Type.String())),
		"description": {
			Get: func() tengo.Object {
				return interop.GoStrToTStr(obj.Description)
			},
			Set: func(o tengo.Object) error {
				desc, err := interop.TStrToGoStr(o, "description")
				if err != nil {
					return err
				}

				obj.Description = desc
				return nil
			},
		},
	}

	for i := range obj.Fields {
		field := obj.Fields[i]
		properties[field.Name] = types.Property{
			Get: func() tengo.Object {
				return makeGraphQLObject(field, client)
			},
		}
	}

	gqlObj.PropObject = types.PropObject{
		ObjectMap:  objectMap,
		Properties: properties,
	}
	return gqlObj
}
