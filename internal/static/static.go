package static

import (
	_ "embed"
)

//go:embed introspection.graphql
var IntrospectionQuery string
