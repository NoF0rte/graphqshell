package tengomod

import (
	"fmt"
	"strings"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/tengo/v2"
)

type GraphQLObjArray struct {
	tengo.ObjectImpl
	Value []*graphql.Object
}

func (o *GraphQLObjArray) String() string {
	var items []string
	for _, item := range o.Value {
		output, err := execTemplate(fieldSigTemplate, item)
		if err != nil {
			return fmt.Sprintf("error ocurred during template execution: %v", err)
		}
		items = append(items, output)
	}

	return fmt.Sprintf("[%s]", strings.Join(items, ", "))
}

func (o *GraphQLObjArray) IsFalsy() bool {
	return len(o.Value) == 0
}

func (o *GraphQLObjArray) Copy() tengo.Object {
	return &GraphQLObjArray{
		Value: append([]*graphql.Object{}, o.Value...),
	}
}

func (o *GraphQLObjArray) TypeName() string {
	return "graphql-obj-array"
}

func (o *GraphQLObjArray) IndexGet(index tengo.Object) (tengo.Object, error) {
	intIdx, ok := index.(*tengo.Int)
	if ok {
		if intIdx.Value >= 0 && intIdx.Value < int64(len(o.Value)) {
			return makeGraphQLObject(o.Value[intIdx.Value]), nil
		}

		return nil, tengo.ErrIndexOutOfBounds
	}

	strIdx, ok := index.(*tengo.String)
	if ok {
		for _, obj := range o.Value {
			if strIdx.Value == obj.Name {
				return makeGraphQLObject(obj), nil
			}
		}

		return tengo.UndefinedValue, nil
	}

	return nil, tengo.ErrInvalidIndexType
}

func (o *GraphQLObjArray) CanIterate() bool {
	return true
}

func (o *GraphQLObjArray) Iterate() tengo.Iterator {
	var items []tengo.Object
	for _, item := range o.Value {
		items = append(items, makeGraphQLObject(item))
	}

	array := &tengo.Array{
		Value: items,
	}
	return array.Iterate()
}
