package graphql

import (
	"fmt"

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

	typeInt    string = "Int"
	typeString string = "String"
	typeBool   string = "Boolean"
	typeID     string = "ID"
	typeURI    string = "URI"
)

var typeMap map[string]FullType = make(map[string]FullType)

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
	Type              TypeRef      `json:"type"`
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
	Type         TypeRef     `json:"type"`
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

func (t TypeRef) Create() *Object {
	switch t.Kind {
	case kindNonNull:
		return t.OfType.Create()
	case kindList:
		return &Object{
			Name: t.String(),
			Value: []*Object{
				t.OfType.Create(),
			},
		}
	case kindInputObject:
		var fields []Object

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
				field.Value = inputField.Type.Create()
			}

			fields = append(fields, field)
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
		default:
			panic(fmt.Errorf("no default value for scalar %s", t.Name))
		}

		return &obj
	default:
		fmt.Printf("[!] Either unknown or ignored kind %s", t.Kind)
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
	Args   []Object
	Fields []Object
	Value  interface{}
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
		if t.Name == "Query" {
			for _, field := range t.Fields {
				fmt.Println(field.Type.String())
			}
		}
	}
	return nil, nil, nil
}
