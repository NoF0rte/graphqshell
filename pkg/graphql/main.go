package graphql

import (
	"encoding/json"
	"fmt"
	"math/rand"
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
	typeFloat    string = "Float"
)

var (
	typeMap      map[string]FullType            = make(map[string]FullType)
	objCache     map[string]*Object             = make(map[string]*Object)
	valCache     map[string]interface{}         = make(map[string]interface{})
	deferResolve map[string][]func(interface{}) = make(map[string][]func(interface{}))
	resolveStack *arraystack.Stack              = arraystack.New()
)

func getOrResolveObj(ref *TypeRef) *Object {
	if ref.IsScalar() {
		return ref.Resolve()
	}

	fqName := ref.String()
	obj, ok := objCache[fqName]
	if !ok {
		obj = ref.Resolve()
		objCache[fqName] = obj
	}

	return obj
}

func getOrGenValue(obj *Object) interface{} {
	if obj.Type.IsScalar() {
		return obj.GenValue()
	}

	val, ok := valCache[obj.Name]
	if !ok {
		val = obj.GenValue()
		valCache[obj.Name] = val
	}

	return val
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

func (f Field) Resolve() *Object {
	var obj *Object
	found := getOrResolveObj(f.Type)
	if found != nil {
		obj = found.Copy()
		obj.Name = f.Name
	}

	for _, arg := range f.Args {
		obj.Args = append(obj.Args, arg.Resolve())
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

func (v InputValue) Resolve() *Object {
	var obj *Object
	found := getOrResolveObj(v.Type)
	if found != nil {
		obj = found.Copy()
		obj.Name = v.Name
	}

	return obj
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

func (t TypeRef) IsScalar() bool {
	if t.OfType == nil {
		return t.Kind == kindScalar
	}

	return t.OfType.IsScalar()
}

func (t TypeRef) Resolve() *Object {
	switch t.Kind {
	case kindNonNull:
		// check stack?
		return getOrResolveObj(t.OfType)
	case kindList:
		// check stack?
		var value []*Object
		obj := getOrResolveObj(t.OfType)
		if obj == nil {
			return nil
		}

		value = append(value, obj)
		return &Object{
			Name:  t.String(),
			Type:  t,
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
				field.Value = getOrResolveObj(inputField.Type)
			}

			fields = append(fields, &field)
		}

		return &Object{
			Name:   t.Name,
			Type:   t,
			Fields: fields,
		}
	case kindEnum:
		enumType, ok := typeMap[t.Name]
		if !ok {
			panic(fmt.Errorf("unknown enum type %s", t.Name))
		}

		return &Object{
			Name:  t.String(),
			Type:  t,
			Value: enumType.EnumValues[0],
		}
	case kindScalar:
		return &Object{
			Name: t.String(),
			Type: t,
			Value: func(name string) interface{} {
				randInt := rand.Intn(500)
				switch t.Name {
				case typeBool:
					return randInt%2 == 0
				case typeInt:
					return randInt
				case typeString:
					return fmt.Sprintf("%s string", name)
				case typeID:
					return uuid.New().String()
				case typeURI:
					return fmt.Sprintf("https://example.com/%s", name)
				case typeDateTime:
					return time.Now()
				case typeHTML:
					return fmt.Sprintf("<html><body><h1>%s</h1></body></html>", name)
				case typeFloat:
					return rand.Float64() * float64(randInt)
				default: // Make configurable
					fmt.Printf("[!] No default value for scalar %s\n", t.Name)
					return fmt.Sprintf("unknown %s", name)
				}
			},
		}
	case kindObject:
		objType, ok := typeMap[t.Name]
		if !ok {
			panic(fmt.Errorf("unknown input object type %s", t.Name))
		}

		resolveStack.Push(t.Name)

		obj := &Object{
			Name:   t.Name,
			Type:   t,
			Fields: make([]*Object, 0),
		}

		for _, f := range objType.Fields {
			rootTypeName := f.Type.RootName()
			if isResolving(rootTypeName) {
				fmt.Printf("[+] [%s] Deferring field %s: %s\n", t.Name, f.Name, rootTypeName)
				deferResolve[rootTypeName] = append(deferResolve[rootTypeName], func(o interface{}) {
					copied := o.(*Object).Copy()
					copied.Name = f.Name
					obj.Fields = append(obj.Fields, copied)
				})
				continue
			}

			fmt.Printf("[+] [%s] Creating field %s: %s\n", t.Name, f.Name, rootTypeName)
			fieldObj := f.Resolve()
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
	Type   TypeRef
	Args   []*Object
	Fields []*Object
	Value  interface{}
}

func (o *Object) Copy() *Object {
	copied := &Object{
		Name:   o.Name,
		Type:   o.Type,
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
		objRootType := o.Type.RootName()
		resolveStack.Push(objRootType)

		value := make(map[string]interface{})
		for _, field := range o.Fields {
			fieldRootType := field.Type.RootName()
			if isResolving(fieldRootType) {
				value[field.Name] = nil
				// fmt.Printf("[+] [%s] Deferring field %s: %s\n", o.Name, field.Name, field.Type)
				// deferResolve[fieldRootType] = append(deferResolve[fieldRootType], func(v interface{}) {
				// 	value[field.Name] = v
				// })
				continue
			}

			fmt.Printf("[+] [%s] Creating field %s: %s\n", o.Name, field.Name, field.Type)
			value[field.Name] = getOrGenValue(field)
		}

		resolveStack.Pop()

		deferred := deferResolve[objRootType]
		for _, fun := range deferred {
			fun(value)
		}

		delete(deferResolve, objRootType)

		return value
	}

	switch v := o.Value.(type) {
	case *Object:
		return getOrGenValue(v)
	case []*Object:
		var values []interface{}
		for _, obj := range v {
			values = append(values, getOrGenValue(obj))
		}
		return values
	case EnumValue:
		return v.Name
	case func(string) interface{}:
		return v(o.Name)
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
		obj := field.Resolve()
		if obj != nil {
			data, err := json.MarshalIndent(obj.GenValue(), "", "  ")
			if err != nil {
				panic(err)
			}
			fmt.Println(string(data))
		}
	}

	return nil, nil, nil
}
