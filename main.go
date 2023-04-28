package main

import (
	"encoding/json"
	"os"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
)

func main() {
	bytes, err := os.ReadFile("yelp_introspection.json")
	if err != nil {
		panic(err)
	}

	var response graphql.IntrospectionResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		panic(err)
	}

	graphql.ParseIntrospection(response)
}
