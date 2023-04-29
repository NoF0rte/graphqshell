package main

import (
	"encoding/json"
	"fmt"
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

	query, mutation, err := graphql.ParseIntrospection(response)
	if err != nil {
		panic(err)
	}

	fmt.Println(query.Name)
	userQuery := query.Get("user")
	if userQuery != nil {
		val := userQuery.GenValue()
		data, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			panic(err)
		}

		fmt.Println(string(data))
	}

	fmt.Println(mutation.Name)
}
