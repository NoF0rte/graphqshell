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
	objCache     map[string]*graphql.Object     = make(map[string]*graphql.Object)
	fuzzStack    *arraystack.Stack              = arraystack.New()
	fuzzMutex    *sync.Mutex                    = &sync.Mutex{}
	deferResolve map[string][]func(interface{}) = make(map[string][]func(interface{}))
	resolveStack *arraystack.Stack              = arraystack.New()
)

type Result interface {
	Object() *graphql.Object
}

type FieldResult struct {
	Word  string
	Field string

	obj *graphql.Object
}

func (r *FieldResult) Object() *graphql.Object {
	return r.obj
}

type TypeResult struct {
	Word string
	Type string
	Kind string

	obj *graphql.Object
}

func (r *TypeResult) Object() *graphql.Object {
	return r.obj
}

func isResolving(name string) bool {
	for _, value := range resolveStack.Values() {
		if name == value.(string) {
			return true
		}
	}

	return false
}

func push(job *Job) {
	fuzzMutex.Lock()
	fuzzStack.Push(job)
	fuzzMutex.Unlock()
}

func pop() (*Job, bool) {
	fuzzMutex.Lock()
	defer fuzzMutex.Unlock()

	v, ok := fuzzStack.Pop()
	if v == nil {
		return nil, false
	}

	return v.(*Job), ok
}

func getRootObj(o *graphql.Object) *graphql.Object {
	if o.Parent == nil {
		return o
	}

	parent := *o.Parent
	parent.Fields = []*graphql.Object{o}

	return getRootObj(&parent)
}

func assignNewParent(items []*graphql.Object, parent *graphql.Object) []*graphql.Object {
	var newItems []*graphql.Object
	for _, item := range items {
		i := *item
		i.Parent = parent

		newItems = append(newItems, &i)
	}

	return newItems
}

// schemaFuzzCmd represents the fuzz command
var schemaFuzzCmd = &cobra.Command{
	Use:   "fuzz",
	Short: "Iteratively build up the schema for a GraphQL endpoint",
	Run: func(cmd *cobra.Command, args []string) {
		u, _ := cmd.Flags().GetString("url")
		wordlist, _ := cmd.Flags().GetString("wordlist")
		cookies, _ := cmd.Flags().GetString("cookies")
		// method, _ := cmd.Flags().GetString("method")
		proxy, _ := cmd.Flags().GetString("proxy")
		headers, _ := cmd.Flags().GetStringSlice("headers")
		// delay, _ := cmd.Flags().GetInt("delay")
		threads, _ := cmd.Flags().GetInt("threads")

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

		graphqlNameRe := "[_A-Za-z][_0-9A-Za-z]*"

		queryFieldRe := func(name string) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf(`Cannot query field "%s" on type "(%s)"`, regexp.QuoteMeta(name), graphqlNameRe))
		}
		noSubfieldsRe := func(name string) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf(`Field "%s".*"([^"]+)" has no subfields`, regexp.QuoteMeta(name)))
		}
		fieldOfTypeRe := func(name string) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf(`Field \"%s\" of type \"([^"]+)\"`, regexp.QuoteMeta(name)))
		}

		didYouMeanRe := regexp.MustCompile(`Did you mean.*\?`)
		graphqlRe := regexp.MustCompile(fmt.Sprintf(`"(%s)(?: .*)?"`, graphqlNameRe))

		rootMutation := &graphql.Object{
			Name:     "Mutation",
			Template: "mutation {{.Body}}",
		}
		push(&Job{
			Type:   typeJob,
			Object: rootMutation,
		})

		rootQuery := &graphql.Object{
			Name:     "Query",
			Template: "query {{.Body}}",
		}
		push(&Job{
			Type:   typeJob,
			Object: rootQuery,
		})

		fieldWorker := func(o *graphql.Object, words chan string, results chan Result, wg *sync.WaitGroup) {
			defer wg.Done()

			obj := *o

			fuzzField := &graphql.Object{}
			obj.Fields = []*graphql.Object{fuzzField}

			for word := range words {
				fuzzField.Name = word

				resp, err := client.PostJSON(getRootObj(&obj))
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
						results <- &FieldResult{
							Word:  word,
							Field: matches[1],
							obj:   o,
						}
					}
				}
			}
		}

		fuzzFields := func(o *graphql.Object) chan Result {
			c := make(chan Result)

			go func() {
				wg := &sync.WaitGroup{}
				words := getWords()
				for i := 0; i < threads; i++ {
					wg.Add(1)
					go fieldWorker(o, words, c, wg)
				}

				wg.Wait()
				close(c)
			}()

			return c
		}
		determineType := func(o *graphql.Object) chan Result {
			c := make(chan Result)

			obj := *o
			go func() {
				defer close(c)

				name := "graphqlshell"
				obj.Fields = append(obj.Fields, &graphql.Object{
					Name: name,
				})

				resp, err := client.PostJSON(getRootObj(&obj))
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					return
				}

				for _, e := range resp.Result.Errors {
					result := &TypeResult{
						obj:  o,
						Word: name,
						Kind: graphql.KindObject,
					}

					matches := queryFieldRe(name).FindAllStringSubmatch(e.Message, -1)
					if len(matches) == 0 {
						matches = noSubfieldsRe(obj.Name).FindAllStringSubmatch(e.Message, -1)

						if len(matches) == 0 {
							continue
						}

						result.Kind = graphql.KindScalar
						result.Type = matches[0][1]
					} else if o.Name != rootQuery.Name && o.Name != rootMutation.Name {
						obj.Fields = []*graphql.Object{}

						resp, err = client.PostJSON(getRootObj(&obj))
						if err != nil {
							fmt.Printf("[!] Error posting: %v\n", err)
							return
						}

						for _, e2 := range resp.Result.Errors {
							matches2 := fieldOfTypeRe(obj.Name).FindAllStringSubmatch(e2.Message, -1)
							if len(matches2) == 0 {
								continue
							}

							result.Type = matches2[0][1]
						}
					} else {
						result.Type = matches[0][1]
					}

					c <- result
				}
			}()

			return c
		}

		for {
			job, ok := pop()
			if !ok || job == nil {
				break
			}

			var results chan Result
			switch job.Type {
			case fieldJob:
				resolveStack.Push(job.Object.Type.RootName())
				results = fuzzFields(job.Object)
			case typeJob:
				results = determineType(job.Object)
			default:
				panic(fmt.Sprintf("Unknown job type: %d", job.Type))
			}

			// Queue objects to get fuzzed
			// 1. Fuzz fields
			// 2. Fuzz args
			// 3. Get field types and queue to be fuzzed
			// 4. Get arg types and queue to be fuzzed
			for result := range results {

				obj := result.Object()
				switch r := result.(type) {
				case *FieldResult:
					field := &graphql.Object{
						Name:   r.Field,
						Parent: obj,
					}
					if obj.AddField(field) {
						push(&Job{
							Type:   typeJob,
							Object: field,
						})
						push(&Job{
							Type:   argJob,
							Object: field,
						})
					}
				case *TypeResult:
					if obj.Name == rootQuery.Name || obj.Name == rootMutation.Name {
						obj.Name = r.Type
					} else {
						ref := graphql.TypeRefFromString(r.Type, r.Kind)
						obj.Type = *ref

						rootName := ref.RootName()
						if isResolving(rootName) {
							deferResolve[rootName] = append(deferResolve[rootName], func(i interface{}) {
								o := i.(*graphql.Object)
								obj.Fields = o.Fields
							})
							continue
						}

						o, resolved := objCache[rootName]
						if resolved {
							obj.Fields = assignNewParent(o.Fields, obj)
							obj.Args = assignNewParent(o.Args, obj)
							obj.PossibleValues = assignNewParent(o.PossibleValues, obj)

							continue
						}

						if r.Kind == graphql.KindScalar {
							continue
						}
					}

					push(&Job{
						Type:   fieldJob,
						Object: obj,
					})
					// push args
				}
			}

			if job.Type != typeJob {
				resolveStack.Pop()

				rootName := job.Object.Type.RootName()
				defered := deferResolve[rootName]
				for _, fn := range defered {
					fn(job.Object)
				}

				delete(deferResolve, rootName)

				objCache[rootName] = job.Object
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
