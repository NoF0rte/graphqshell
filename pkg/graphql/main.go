package graphql

import (
	"fmt"
	"time"

	"github.com/emirpasic/gods/stacks/arraystack"
	"github.com/google/uuid"
)

const (
	kindNonNull     string = "NON_NULL"
	kindScalar      string = "SCALAR"
	kindObject      string = "OBJECT"
	kindInterface   string = "INTERFACE"
	kindInputObject string = "INPUT_OBJECT"
	kindEnum        string = "ENUM"
	kindList        string = "LIST"

	typeInt      string = "Int"
	typeString   string = "String"
	typeBool     string = "Boolean"
	typeID       string = "ID"
	typeURI      string = "URI"
	typeDateTime string = "DateTime"
	typeHTML     string = "HTML"
)

var (
	typeMap      map[string]FullType        = make(map[string]FullType)
	objCache     map[string]*Object         = make(map[string]*Object)
	deferResolve map[string][]func(*Object) = make(map[string][]func(*Object))
	resolveStack *arraystack.Stack          = arraystack.New()
)

func getOrCreateObj(ref *TypeRef) *Object {
	fqName := ref.String()
	obj, ok := objCache[fqName]
	if !ok {
		obj = ref.Create()
		objCache[fqName] = obj
	}

	return obj
}

func isResolving(name string) bool {
	for _, value := range resolveStack.Values() {
		if name == value.(string) {
			return true
		}
	}

	return false
}

type IntrospectionResponse struct {
	Data struct {
		Schema Schema `json:"__schema"`
	} `json:"data"`
}

type Schema struct {
	MutationType struct {
		Name string `json:"name"`
	} `json:"mutationType"`
	QueryType struct {
		Name string `json:"name"`
	} `json:"queryType"`
	Types []FullType `json:"types"`
}

type FullType struct {
	Description   string       `json:"description"`
	EnumValues    []EnumValue  `json:"enumValues"`
	Fields        []Field      `json:"fields"`
	InputFields   []InputValue `json:"inputFields"`
	Interfaces    []TypeRef    `json:"interfaces"`
	Kind          string       `json:"kind"`
	Name          string       `json:"name"`
	PossibleTypes []TypeRef    `json:"possibleTypes"`
}

type Field struct {
	Args              []InputValue `json:"args"`
	DeprecationReason string       `json:"deprecationReason"`
	Description       string       `json:"description"`
	IsDeprecated      bool         `json:"isDeprecated"`
	Name              string       `json:"name"`
	Type              *TypeRef     `json:"type"`
}

func (f Field) Create() *Object {
	var obj *Object
	found := getOrCreateObj(f.Type)
	if found != nil {
		obj = found.Copy()
		obj.Name = f.Name
	}

	return obj
}

type EnumValue struct {
	DeprecationReason interface{} `json:"deprecationReason"`
	Description       string      `json:"description"`
	IsDeprecated      bool        `json:"isDeprecated"`
	Name              string      `json:"name"`
}

type InputValue struct {
	DefaultValue interface{} `json:"defaultValue"`
	Description  string      `json:"description"`
	Name         string      `json:"name"`
	Type         *TypeRef    `json:"type"`
}

type TypeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *TypeRef `json:"ofType"`
}

func (t TypeRef) IsRequired() bool {
	if t.Kind == "NON_NULL" {
		return true
	}

	if t.OfType != nil {
		return t.IsRequired()
	}

	return false
}

func (t TypeRef) String() string {
	if t.OfType == nil {
		return t.Name
	}

	ofType := t.OfType.String()
	switch t.Kind {
	case "NON_NULL":
		return fmt.Sprintf("%s!", ofType)
	case "LIST":
		return fmt.Sprintf("[%s]", ofType)
	default:
		return fmt.Sprintf("%s - %s", t.Kind, ofType)
	}
}

func (t TypeRef) RootName() string {
	if t.OfType == nil {
		return t.Name
	}

	return t.OfType.RootName()
}

func (t TypeRef) Create() *Object {
	switch t.Kind {
	case kindNonNull:
		// check stack?
		return getOrCreateObj(t.OfType)
	case kindList:
		// check stack?
		var value []*Object
		obj := getOrCreateObj(t.OfType)
		if obj == nil {
			return nil
		}

		value = append(value, obj)
		return &Object{
			Name:  t.String(),
			Value: value,
		}
	case kindInputObject:
		var fields []*Object

		objType, ok := typeMap[t.Name]
		if !ok {
			panic(fmt.Errorf("unknown input object type %s", t.Name))
		}

		for _, inputField := range objType.InputFields {
			field := Object{
				Name:  inputField.Name,
				Value: inputField.DefaultValue,
			}

			if field.Value == nil {
				// check stack
				field.Value = getOrCreateObj(inputField.Type)
			}

			fields = append(fields, &field)
		}

		return &Object{
			Name:   t.Name,
			Fields: fields,
		}
	case kindEnum:
		enumType, ok := typeMap[t.Name]
		if !ok {
			panic(fmt.Errorf("unknown enum type %s", t.Name))
		}

		return &Object{
			Name:  t.String(),
			Value: enumType.EnumValues[0],
		}
	case kindScalar:
		obj := Object{
			Name: t.String(),
		}

		switch t.Name {
		case typeBool:
			obj.Value = false
		case typeInt:
			obj.Value = 1
		case typeString:
			obj.Value = "default string"
		case typeID:
			obj.Value = uuid.New().String()
		case typeURI:
			obj.Value = "https://example.com"
		case typeDateTime:
			obj.Value = time.Now()
		case typeHTML:
			obj.Value = "<html><body><h1>Example</h1></body></html>"
		default: // Make configurable
			fmt.Printf("[!] No default value for scalar %s\n", t.Name)
		}

		return &obj
	case kindObject:
		objType, ok := typeMap[t.Name]
		if !ok {
			panic(fmt.Errorf("unknown input object type %s", t.Name))
		}

		resolveStack.Push(t.Name)

		obj := &Object{
			Name:   t.Name,
			Fields: make([]*Object, 0),
		}

		for _, f := range objType.Fields {
			rootTypeName := f.Type.RootName()
			if isResolving(rootTypeName) {
				fmt.Printf("[+] [%s] Deferring field %s: %s\n", t.Name, f.Name, rootTypeName)
				deferResolve[rootTypeName] = append(deferResolve[rootTypeName], func(o *Object) {
					obj.Fields = append(obj.Fields, o)
				})
				continue
			}

			fmt.Printf("[+] [%s] Creating field %s: %s\n", t.Name, f.Name, rootTypeName)
			fieldObj := f.Create()
			if fieldObj != nil {
				obj.Fields = append(obj.Fields, fieldObj)
			}
		}

		resolveStack.Pop()

		defered := deferResolve[t.Name]
		for _, fun := range defered {
			fun(obj)
		}

		delete(deferResolve, t.Name)

		return obj
	default:
		fmt.Printf("[!] Either unknown or ignored kind %s\n", t.Kind)
		return nil
	}
}

type RootQuery struct {
	Name   string
	Fields []Object
}

type Query struct {
	Name        string
	Description string
	Args        []InputValue
	ReturnType  TypeRef
}

type Object struct {
	Name   string
	Args   []*Object
	Fields []*Object
	Value  interface{}
}

func (o *Object) Copy() *Object {
	copied := &Object{
		Name:   o.Name,
		Args:   make([]*Object, len(o.Args)),
		Fields: make([]*Object, len(o.Fields)),
		Value:  o.Value,
	}
	copy(copied.Args, o.Args)
	copy(copied.Fields, o.Fields)

	return copied
}

func (o *Object) GenValue() interface{} {
	if o.Value == nil {
		value := make(map[string]interface{})
		for _, field := range o.Fields {
			value[field.Name] = field.GenValue()
		}

		return value
	}

	switch v := o.Value.(type) {
	case *Object:
		return v.GenValue()
	case []*Object:
		var values []interface{}
		for _, obj := range v {
			values = append(values, obj.GenValue())
		}
		return values
	case EnumValue:
		return v.Name
	default:
		return v
	}
}

type RootMutation struct {
}

func ParseIntrospection(response IntrospectionResponse) (*RootQuery, *RootMutation, error) {
	schema := response.Data.Schema
	// root := &RootQuery{
	// 	Name: schema.QueryType.Name,
	// }

	types := schema.Types
	for _, t := range types {
		typeMap[t.Name] = t

		// if t.Name == "Query" {
		// 	for _, field := range t.Fields {
		// 		fmt.Println(field.Type.String())
		// 	}
		// }
	}

	queryType := typeMap["Query"]
	for _, field := range queryType.Fields {
		fmt.Println(field.Name)
		obj := field.Create()
		if obj != nil {
			fmt.Println(obj.GenValue())
			break
		}
	}

	return nil, nil, nil
}
