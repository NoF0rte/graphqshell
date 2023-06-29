package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/fileutil"
	"github.com/emirpasic/gods/stacks/arraystack"
	"github.com/spf13/cobra"
)

type resultType int

const (
	fieldResult resultType = iota
	typeResult
)

type jobType int

const (
	fieldJob jobType = iota
	argJob
	argFieldJob
	typeJob
)

type Job struct {
	Type   jobType
	Object *graphql.Object
}

var (
	objCache  map[string]*graphql.Object = make(map[string]*graphql.Object)
	fuzzStack *arraystack.Stack          = arraystack.New()
	fuzzMutex *sync.Mutex                = &sync.Mutex{}
)

type Result struct {
	Type    resultType
	Word    string
	Matched string
	Object  *graphql.Object
}

func push(job Job) {
	fuzzMutex.Lock()
	fuzzStack.Push(job)
	fuzzMutex.Unlock()
}

func pop() (Job, bool) {
	fuzzMutex.Lock()
	defer fuzzMutex.Unlock()

	v, ok := fuzzStack.Pop()
	return v.(Job), ok
}

// func getOrResolveObj(ref *TypeRef) *graphql.Object {
// 	if ref.IsScalar() {
// 		return ref.Resolve()
// 	}

// 	fqName := ref.String()
// 	obj, ok := objCache[fqName]
// 	if !ok {
// 		obj = ref.Resolve()
// 		objCache[fqName] = obj
// 	}

// 	return obj
// }

// schemaFuzzCmd represents the fuzz command
var schemaFuzzCmd = &cobra.Command{
	Use:   "fuzz",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		u, _ := cmd.Flags().GetString("url")
		wordlist, _ := cmd.Flags().GetString("wordlist")
		cookies, _ := cmd.Flags().GetString("cookies")
		// method, _ := cmd.Flags().GetString("method")
		proxy, _ := cmd.Flags().GetString("proxy")
		headers, _ := cmd.Flags().GetStringSlice("headers")
		// delay, _ := cmd.Flags().GetInt("delay")
		// threads, _ := cmd.Flags().GetInt("threads")

		var opts []graphql.ClientOption
		if cookies != "" {
			opts = append(opts, graphql.WithCookies(cookies))
		}

		headerMap := make(map[string]string)
		for _, header := range headers {
			key, value, _ := strings.Cut(header, ":")
			headerMap[key] = value
		}

		if len(headerMap) > 0 {
			opts = append(opts, graphql.WithHeaders(headerMap))
		}

		if proxy != "" {
			opts = append(opts, graphql.WithProxy(proxy))
		}

		client := graphql.NewClient(u, opts...)

		lines, err := fileutil.ReadLines(wordlist)
		if err != nil {
			fmt.Printf("[!] Error: %v\n", err)
			return
		}

		getWords := func() chan string {
			c := make(chan string)
			go func() {
				for _, word := range lines {
					c <- word
				}
				close(c)
			}()

			return c
		}

		didYouMeanRe := regexp.MustCompile(`Did you mean.*\?`)
		graphqlRe := regexp.MustCompile(`"([_A-Za-z][_0-9A-Za-z]*)"`)

		fuzzFields := func(o *graphql.Object) chan *Result {
			c := make(chan *Result)

			go func() {
				fuzzField := &graphql.Object{}
				o.Fields = append(o.Fields, fuzzField)

				for word := range getWords() {
					fuzzField.Name = word

					resp, err := client.PostJSON(o)
					if err != nil {
						fmt.Printf("[!] Error posting: %v\n", err)
						continue
					}

					for _, e := range resp.Result.Errors {
						didYouMeanMatches := didYouMeanRe.FindStringSubmatch(e.Message)
						if len(didYouMeanMatches) == 0 {
							continue
						}

						allMatches := graphqlRe.FindAllStringSubmatch(didYouMeanMatches[0], -1)
						for _, matches := range allMatches {
							c <- &Result{
								Word:    word,
								Type:    fieldResult,
								Matched: matches[1],
								Object:  o,
							}
						}
					}
				}

				close(c)
			}()

			return c
		}

		rootMutation := &graphql.Object{
			Name:     "Mutation",
			Template: "mutation {{.Body}}",
		}
		push(Job{
			Type:   fieldJob,
			Object: rootMutation,
		})

		rootQuery := &graphql.Object{
			Name:     "Query",
			Template: "query {{.Body}}",
		}
		push(Job{
			Type:   fieldJob,
			Object: rootQuery,
		})

		for {
			job, ok := pop()
			if !ok {
				break
			}

			var results chan *Result
			switch job.Type {
			case fieldJob:
				results = fuzzFields(job.Object)
			default:
				panic(fmt.Sprintf("Unknown job type: %d", job.Type))
			}

			// Queue objects to get fuzzed
			// 1. Fuzz fields
			// 2. Fuzz args
			// 3. Get field types and queue to be fuzzed
			// 4. Get arg types and queue to be fuzzed
			for result := range results {
				// Once type has been fuzzed, then queue to fuzz for fields
				switch result.Type {
				case fieldResult:
				case typeResult:
				}
				fmt.Println(result.Matched)
			}
		}
	},
}

func init() {
	schemaCmd.AddCommand(schemaFuzzCmd)

	schemaFuzzCmd.Flags().StringP("url", "u", "", "The GraphQL endpoint URL")
	schemaFuzzCmd.MarkFlagRequired("url")
	schemaFuzzCmd.Flags().StringP("wordlist", "w", "", "The fuzzing wordlist")
	schemaFuzzCmd.MarkFlagRequired("wordlist")
	schemaFuzzCmd.Flags().StringP("cookies", "c", "", "The cookies needed for the request")
	schemaFuzzCmd.Flags().StringSliceP("headers", "H", []string{}, "Any extra headers needed")
	// schemaFuzzCmd.Flags().StringP("method", "m", http.MethodGet, "The request method")
	schemaFuzzCmd.Flags().String("proxy", "", "The proxy to use")
	// schemaFuzzCmd.Flags().Int("delay", 0, "How long the request should ")
	schemaFuzzCmd.Flags().IntP("threads", "t", 1, "Number of threads")
}
