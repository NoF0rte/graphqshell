package graphql

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"text/template"
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
	kindUnion       string = "UNION"

	typeInt      string = "Int"
	typeString   string = "String"
	typeBool     string = "Boolean"
	typeID       string = "ID"
	typeURI      string = "URI"
	typeDateTime string = "DateTime"
	typeHTML     string = "HTML"
	typeFloat    string = "Float"
)

const gqlTemplate string = `
{{- if (isEmpty .Fields) -}}
	{{ .Name }}
{{- else -}}

{{- if (isEmpty .Args) -}}
{{ .Name }} {
{{- else -}}
{{ .Name }}({{printArgs}}) {
{{- end}}
{{ range .Fields -}}
	{{ toGraphQL . | println }}
{{- end -}}
}

{{- end -}}`

var (
	typeMap      map[string]FullType            = make(map[string]FullType)
	scalarTypes  []string                       = make([]string, 0)
	objCache     map[string]*Object             = make(map[string]*Object)
	valCache     map[string]interface{}         = make(map[string]interface{})
	deferResolve map[string][]func(interface{}) = make(map[string][]func(interface{}))
	resolveStack *arraystack.Stack              = arraystack.New()

	Debug bool = false
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

func logf(format string, a ...interface{}) {
	if !Debug {
		return
	}

	fmt.Printf(format, a...)
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

type EnumValue struct {
	DeprecationReason interface{} `json:"deprecationReason"`
	Description       string      `json:"description"`
	IsDeprecated      bool        `json:"isDeprecated"`
	Name              string      `json:"name"`
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
	var obj Object
	found := getOrResolveObj(f.Type)
	if found != nil {
		obj = *found
		obj.Name = f.Name

		if f.Description != "" {
			obj.Description = f.Description
		}

		for _, arg := range f.Args {
			obj.Args = append(obj.Args, arg.Resolve())
		}
	}

	return &obj
}

type InputValue struct {
	DefaultValue interface{} `json:"defaultValue"`
	Description  string      `json:"description"`
	Name         string      `json:"name"`
	Type         *TypeRef    `json:"type"`
}

func (v InputValue) Resolve() *Object {
	var obj Object
	found := getOrResolveObj(v.Type)
	if found != nil {
		obj = *found
		obj.Name = v.Name

		if v.Description != "" {
			obj.Description = v.Description
		}

		if v.DefaultValue != nil {
			obj.valFactory = func(_ string) interface{} {
				return v.DefaultValue
			}
		}
	}

	return &obj
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
	objType, ok := typeMap[t.Name]
	if !ok && t.Name != "" {
		panic(fmt.Errorf("unknown type %s", t.Name))
	}

	switch t.Kind {
	case kindNonNull, kindList:
		obj := getOrResolveObj(t.OfType)
		if obj == nil {
			return nil
		}

		copied := *obj
		return &Object{
			Name:           t.String(),
			Type:           t,
			Fields:         obj.Fields,
			Args:           obj.Args,
			PossibleValues: obj.PossibleValues,
			valFactory: func(name string) interface{} {
				copied.Name = name
				if t.Kind == kindList {
					return []interface{}{
						getOrGenValue(&copied),
					}
				}

				return getOrGenValue(&copied)
			},
		}
	case kindEnum:
		return &Object{
			Name: t.String(),
			Type: t,
			valFactory: func(_ string) interface{} {
				return objType.EnumValues[rand.Intn(len(objType.EnumValues))].Name
			},
		}
	case kindScalar:
		return &Object{
			Name: t.String(),
			Type: t,
			valFactory: func(name string) interface{} {
				randInt := rand.Intn(500)
				switch {
				case strings.Contains(t.Name, typeBool):
					return randInt%2 == 0
				case strings.Contains(t.Name, typeInt):
					return randInt
				case strings.Contains(t.Name, typeString):
					return fmt.Sprintf("%s string", name)
				case strings.Contains(t.Name, typeID):
					return uuid.New().String()
				case strings.Contains(t.Name, typeURI):
					return fmt.Sprintf("https://example.com/%s", name)
				case strings.Contains(t.Name, typeDateTime):
					return time.Now()
				case strings.Contains(t.Name, typeHTML):
					return fmt.Sprintf("<html><body><h1>%s</h1></body></html>", name)
				case strings.Contains(t.Name, typeFloat):
					return rand.Float64() * float64(randInt)
				default: // Make configurable
					logf("[!] No default value for scalar %s\n", t.Name)
					return fmt.Sprintf("unknown %s", name)
				}
			},
		}
	case kindObject, kindInputObject, kindInterface, kindUnion:
		resolveStack.Push(t.Name)

		obj := &Object{
			Name:           t.Name,
			Type:           t,
			Fields:         make([]*Object, 0),
			PossibleValues: make([]*Object, 0),
		}

		for _, f := range objType.Fields {
			rootTypeName := f.Type.RootName()
			if isResolving(rootTypeName) {
				logf("[+] [%s] Deferring field %s: %s\n", t.Name, f.Name, rootTypeName)
				deferResolve[rootTypeName] = append(deferResolve[rootTypeName], func(o interface{}) {
					// copied := o.(*Object).copy()
					copied := *o.(*Object)
					copied.Name = f.Name
					obj.Fields = append(obj.Fields, &copied)
				})
				continue
			}

			logf("[+] [%s] Resolving field %s: %s\n", t.Name, f.Name, rootTypeName)
			fieldObj := f.Resolve()
			if fieldObj != nil {
				obj.Fields = append(obj.Fields, fieldObj)
			}
		}

		for _, f := range objType.InputFields {
			rootTypeName := f.Type.RootName()
			if isResolving(rootTypeName) {
				logf("[+] [%s] Deferring input field %s: %s\n", t.Name, f.Name, rootTypeName)
				deferResolve[rootTypeName] = append(deferResolve[rootTypeName], func(o interface{}) {
					// copied := o.(*Object).copy()
					copied := *o.(*Object)
					copied.Name = f.Name

					if f.DefaultValue != nil {
						copied.valFactory = func(_ string) interface{} {
							return f.DefaultValue
						}
					}

					obj.Fields = append(obj.Fields, &copied)
				})
				continue
			}

			logf("[+] [%s] Resolving input field %s: %s\n", t.Name, f.Name, rootTypeName)
			fieldObj := f.Resolve()
			if fieldObj != nil {
				obj.Fields = append(obj.Fields, fieldObj)
			}
		}

		for _, possibleType := range objType.PossibleTypes {
			rootTypeName := possibleType.RootName()
			if isResolving(rootTypeName) {
				logf("[+] [%s] Deferring possible type %s: %s\n", t.Name, possibleType.Name, rootTypeName)
				deferResolve[rootTypeName] = append(deferResolve[rootTypeName], func(o interface{}) {
					// copied := o.(*Object).copy()
					copied := *o.(*Object)
					copied.Name = t.Name
					obj.PossibleValues = append(obj.PossibleValues, &copied)
				})
				continue
			}

			logf("[+] [%s] Resolving possible type %s: %s\n", t.Name, possibleType.Name, rootTypeName)
			possibleObj := possibleType.Resolve()
			if possibleObj != nil {
				obj.PossibleValues = append(obj.PossibleValues, possibleObj)
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
		logf("[!] Either unknown or ignored kind %s\n", t.Kind)
		return nil
	}
}

type Object struct {
	Name           string
	Description    string
	Type           TypeRef
	Args           []*Object
	Fields         []*Object
	PossibleValues []*Object
	valFactory     func(string) interface{}
	valOverride    interface{}
}

func (o *Object) GenValue() interface{} {
	if o.valOverride != nil {
		return o.valOverride
	}

	if o.valFactory == nil {
		objRootType := o.Type.RootName()
		resolveStack.Push(objRootType)

		var generated interface{}
		if len(o.PossibleValues) != 0 && len(o.Fields) == 0 {
			possibleVal := o.PossibleValues[rand.Intn(len(o.PossibleValues))]
			valRootType := possibleVal.Type.RootName()
			if isResolving(valRootType) {
				logf("[!] Found cycle when generating value. Setting generated = nil\n")
			} else {
				logf("[+] [%s] Creating possible value %s: %s\n", o.Name, possibleVal.Name, possibleVal.Type)
				generated = getOrGenValue(possibleVal)
			}
		} else {
			value := make(map[string]interface{})

			for _, field := range o.Fields {
				fieldRootType := field.Type.RootName()
				if isResolving(fieldRootType) {
					logf("[!] Found cycle when generating value. Setting %s.%s = nil", o.Name, field.Name)
					value[field.Name] = nil
					// fmt.Printf("[+] [%s] Deferring field %s: %s\n", o.Name, field.Name, field.Type)
					// deferResolve[fieldRootType] = append(deferResolve[fieldRootType], func(v interface{}) {
					// 	value[field.Name] = v
					// })
					continue
				}

				logf("[+] [%s] Creating field %s: %s\n", o.Name, field.Name, field.Type)
				value[field.Name] = getOrGenValue(field)
			}

			generated = value
		}

		resolveStack.Pop()

		deferred := deferResolve[objRootType]
		for _, fun := range deferred {
			fun(generated)
		}

		delete(deferResolve, objRootType)

		return generated
	}

	return o.valFactory(o.Name)
}

func (o *Object) SetValue(val interface{}) {
	o.valOverride = val
}

func (o *Object) GetField(path string) *Object {
	if path == "" {
		return nil
	}

	name, remaining, _ := strings.Cut(path, ".")

	for _, field := range o.Fields {
		if field.Name == name {
			if remaining == "" {
				return field
			}

			return field.GetField(remaining)
		}
	}

	return nil
}

func (o *Object) GetArg(name string) *Object {
	for _, arg := range o.Args {
		if arg.Name == name {
			return arg
		}
	}

	return nil
}

func (o *Object) GenArgs() []interface{} {
	var args []interface{}
	for _, obj := range o.Args {
		args = append(args, obj.GenValue())
	}

	return args
}

func (o *Object) GenArg(name string) interface{} {
	for _, obj := range o.Args {
		if obj.Name == name {
			return obj.GenValue()
		}
	}
	return nil
}

func (o *Object) ToGraphQL() (string, error) {
	funcMap := template.FuncMap{
		"isEmpty": func(slice interface{}) bool {
			tp := reflect.TypeOf(slice).Kind()
			switch tp {
			case reflect.Slice, reflect.Array:
				l2 := reflect.ValueOf(slice)
				return l2.Len() == 0
			default:
				return false
			}
		},
		"toGraphQL": func(obj *Object) (string, error) {
			output, err := obj.ToGraphQL()
			if err != nil {
				return "", err
			}

			// Indent once
			pad := strings.Repeat("\t", 1)
			return pad + strings.Replace(output, "\n", "\n"+pad, -1), nil
		},
		"printArgs": func() (string, error) {
			var args []string
			for _, a := range o.Args {
				arg := a.ToArgStr()
				args = append(args, arg)
			}
			return strings.Join(args, ", "), nil
		},
	}

	t := template.Must(template.New("gql").Funcs(funcMap).Parse(gqlTemplate))

	buf := new(bytes.Buffer)
	err := t.Execute(buf, o)

	if err != nil {
		return "", errors.Join(fmt.Errorf("error on object %s:%v", o.Name, err))
	}

	output := buf.String()

	return output, nil
}

func toArgStr(name string, val interface{}) string {
	var str string
	switch t := val.(type) {
	case map[string]interface{}:
		var vals []string
		for key, value := range t {
			vals = append(vals, toArgStr(key, value))
		}
		str = fmt.Sprintf("{%s}", strings.Join(vals, ", "))
	default:
		bytes, _ := json.Marshal(&t)
		str = string(bytes)
	}
	return fmt.Sprintf("%s: %s", name, str)
}

func (o *Object) ToArgStr() string {
	return toArgStr(o.Name, o.GenValue())
}

type RootQuery struct {
	Name    string
	Queries []*Object
}

func (q *RootQuery) Get(name string) *Object {
	for _, query := range q.Queries {
		if query.Name == name {
			return query
		}
	}
	return nil
}

func newRootQuery(name string) *RootQuery {
	t, ok := typeMap[name]
	if !ok {
		return nil
	}

	resolveStack.Push(name)

	var queries []*Object
	for _, field := range t.Fields {
		rootTypeName := field.Type.RootName()

		// can only happen if the field is the RootQuery type
		if isResolving(rootTypeName) {
			continue
		}

		obj := field.Resolve()
		if obj != nil {
			queries = append(queries, obj)
		}
	}

	resolveStack.Pop()
	delete(deferResolve, name)

	return &RootQuery{
		Name:    name,
		Queries: queries,
	}
}

type RootMutation struct {
	Name      string
	Mutations []*Object
}

func (m *RootMutation) Get(name string) *Object {
	for _, mutation := range m.Mutations {
		if mutation.Name == name {
			return mutation
		}
	}
	return nil
}

func newRootMutation(name string) *RootMutation {
	t, ok := typeMap[name]
	if !ok {
		return nil
	}

	resolveStack.Push(name)

	var mutations []*Object
	for _, field := range t.Fields {
		rootTypeName := field.Type.RootName()

		// can only happen if the field is the RootMutation type
		if isResolving(rootTypeName) {
			continue
		}

		mutation := field.Resolve()
		if mutation != nil {
			mutations = append(mutations, mutation)
		}
	}

	resolveStack.Pop()
	delete(deferResolve, name)

	return &RootMutation{
		Name:      name,
		Mutations: mutations,
	}
}

func ParseIntrospection(response IntrospectionResponse) (*RootQuery, *RootMutation, error) {
	schema := response.Data.Schema

	types := schema.Types
	for _, t := range types {
		if t.Kind == kindScalar {
			scalarTypes = append(scalarTypes, t.Name)
		}

		typeMap[t.Name] = t
	}

	return newRootQuery(schema.QueryType.Name), newRootMutation(schema.MutationType.Name), nil
}
