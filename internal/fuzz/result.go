package fuzz

import "github.com/NoF0rte/graphqshell/internal/graphql"

type Result interface {
	Object() *graphql.Object
}

type FuzzResult struct {
	Text     string
	Location Location

	obj *graphql.Object
}

func (r *FuzzResult) Object() *graphql.Object {
	return r.obj
}

type TypeResult struct {
	Type     string
	Kind     string
	Location Location

	obj *graphql.Object
}

func (r *TypeResult) Object() *graphql.Object {
	return r.obj
}

type RequiredResult struct {
	Text     string
	Type     string
	Location Location

	obj *graphql.Object
}

func (r *RequiredResult) Object() *graphql.Object {
	return r.obj
}
