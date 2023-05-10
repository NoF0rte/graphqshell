package tengomod

import (
	"github.com/analog-substance/tengo/v2"
	"github.com/analog-substance/tengo/v2/stdlib"
	"github.com/analog-substance/tengomod"
)

func GetModuleMap() *tengo.ModuleMap {
	moduleMap := stdlib.GetModuleMap(stdlib.AllModuleNames()...)
	moduleMap.AddMap(tengomod.GetModuleMap())
	moduleMap.AddBuiltinModule("graphql", graphqlModule())

	return moduleMap
}
