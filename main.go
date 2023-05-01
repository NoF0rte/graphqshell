package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
)

func main() {
	bytes, err := os.ReadFile("github_introspection.json")
	if err != nil {
		panic(err)
	}

	var response graphql.IntrospectionResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		panic(err)
	}

	query, mutation, err := graphql.ParseIntrospection(response)
	if err != nil {
		panic(err)
	}

	queryValues := make(map[string]interface{})
	for _, q := range query.Queries {
		queryValues[q.Name] = q.GenValue()
	}

	data, err := json.MarshalIndent(queryValues, "", "  ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(data))

	fmt.Println(mutation.Name)
}
