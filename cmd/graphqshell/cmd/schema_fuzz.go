package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/NoF0rte/graphqshell/internal/fuzz"
	"github.com/NoF0rte/graphqshell/internal/graphql"
	"github.com/analog-substance/fileutil"
	"github.com/spf13/cobra"
)

// schemaFuzzCmd represents the fuzz command
var schemaFuzzCmd = &cobra.Command{
	Use:   "fuzz",
	Short: "Iteratively build up the schema for a GraphQL endpoint",
	Run: func(cmd *cobra.Command, args []string) {
		u, _ := cmd.Flags().GetString("url")
		wordlist, _ := cmd.Flags().GetString("wordlist")
		output, _ := cmd.Flags().GetString("output")
		cookies, _ := cmd.Flags().GetString("cookies")
		// method, _ := cmd.Flags().GetString("method")
		proxy, _ := cmd.Flags().GetString("proxy")
		headers, _ := cmd.Flags().GetStringSlice("headers")
		threads, _ := cmd.Flags().GetInt("threads")
		fuzzTargets, _ := cmd.Flags().GetStringSlice("fuzz")

		var clientOpts []graphql.ClientOption
		if cookies != "" {
			clientOpts = append(clientOpts, graphql.WithCookies(cookies))
		}

		headerMap := make(map[string]string)
		for _, header := range headers {
			key, value, _ := strings.Cut(header, ":")
			headerMap[key] = value
		}

		if len(headerMap) > 0 {
			clientOpts = append(clientOpts, graphql.WithHeaders(headerMap))
		}

		if proxy != "" {
			clientOpts = append(clientOpts, graphql.WithProxy(proxy))
		}

		client := graphql.NewClient(u, clientOpts...)

		graphql.ValueCaching = false

		lines, err := fileutil.ReadLines(wordlist)
		if err != nil {
			fmt.Printf("[!] Error: %v\n", err)
			return
		}

		fuzzOpts := []fuzz.FuzzOption{
			fuzz.WithThreads(threads),
			fuzz.WithTargets(fuzzTargets),
		}

		results, _ := fuzz.Start(client, lines, fuzzOpts...)

		// Could potentially be good to go through every object again to fuzz fields/args with just the found words
		resp := graphql.ToIntrospection(&graphql.RootQuery{
			Name:    results.Query.Name,
			Queries: results.Query.Fields,
		}, &graphql.RootMutation{
			Name:      results.Mutation.Name,
			Mutations: results.Mutation.Fields,
		}, results.TypeMap)

		bytes, err := json.MarshalIndent(&resp, "", "  ")
		if err != nil {
			fmt.Printf("[!] Error: %v\n", err)
			return
		}

		err = fileutil.WriteString(output, string(bytes))
		if err != nil {
			fmt.Printf("[!] Error: %v\n", err)
			return
		}

		fmt.Println("[+] Done")
	},
}

func init() {
	schemaCmd.AddCommand(schemaFuzzCmd)

	schemaFuzzCmd.Flags().StringP("url", "u", "", "The GraphQL endpoint URL")
	schemaFuzzCmd.MarkFlagRequired("url")
	schemaFuzzCmd.Flags().StringP("wordlist", "w", "", "The fuzzing wordlist")
	schemaFuzzCmd.MarkFlagRequired("wordlist")
	schemaFuzzCmd.Flags().StringP("output", "o", "", "Path to the resulting schema JSON file")
	schemaFuzzCmd.MarkFlagRequired("output")
	schemaFuzzCmd.Flags().StringP("cookies", "c", "", "The cookies needed for the request")
	schemaFuzzCmd.Flags().StringSliceP("fuzz", "f", []string{"query", "mutation"}, "The specific queries/mutations/fields/args to fuzz. Use 'query.field' to specify a query to fuzz")
	schemaFuzzCmd.Flags().StringSliceP("headers", "H", []string{}, "Any extra headers needed")
	// schemaFuzzCmd.Flags().StringP("method", "m", http.MethodGet, "The request method")
	schemaFuzzCmd.Flags().String("proxy", "", "The proxy to use")
	// schemaFuzzCmd.Flags().Int("delay", 0, "How long the request should ")
	schemaFuzzCmd.Flags().IntP("threads", "t", 1, "Number of threads")
}
