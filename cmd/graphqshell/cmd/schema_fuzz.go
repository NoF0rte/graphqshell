package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
	"github.com/analog-substance/fileutil"
	"github.com/emirpasic/gods/queues/priorityqueue"
	"github.com/emirpasic/gods/stacks/arraystack"
	"github.com/emirpasic/gods/utils"
	"github.com/spf13/cobra"
)

type jobType string

const (
	fieldJob        jobType = "FIELD"
	argJob          jobType = "ARG"
	argFieldJob     jobType = "ARG_FIELD"
	argTypeJob      jobType = "ARG_TYPE"
	argFieldTypeJob jobType = "ARG_FIELD_TYPE"
	fieldTypeJob    jobType = "FIELD_TYPE"
)

type Location string

const (
	locField    Location = "FIELD"
	locArg      Location = "ARG"
	locArgField Location = "ARG_FIELD"
)

type Job struct {
	Priority int
	Type     jobType
	Object   *graphql.Object
}

var (
	objCache     map[string]*graphql.Object     = make(map[string]*graphql.Object)
	jobQueue     *priorityqueue.Queue           = priorityqueue.NewWith(byPriority)
	fuzzMutex    *sync.Mutex                    = &sync.Mutex{}
	deferResolve map[string][]func(interface{}) = make(map[string][]func(interface{}))
	resolveStack *arraystack.Stack              = arraystack.New()
	ignoreFields []string                       = []string{
		"__type",
		"__schema",
	}

	knownScalarTypes []string = []string{
		"Float",
		"String",
		"Int",
		"Boolean",
		"ID",
	}

	rootQuery    *graphql.Object
	rootMutation *graphql.Object
)

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

func byPriority(a, b interface{}) int {
	priorityA := a.(*Job).Priority
	priorityB := b.(*Job).Priority
	return -utils.IntComparator(priorityA, priorityB) // "-" descending order
}

func shouldIgnoreField(name string) bool {
	for _, f := range ignoreFields {
		if f == name {
			return true
		}
	}

	return false
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
	// fuzzStack.Push(job)
	jobQueue.Enqueue(job)
	// fuzzQueue.Enqueue(job)
	fuzzMutex.Unlock()
}

func pop() (*Job, bool) {
	fuzzMutex.Lock()
	defer fuzzMutex.Unlock()

	v, ok := jobQueue.Dequeue()
	if v == nil {
		return nil, false
	}

	return v.(*Job), ok
}

func getRootObj(o *graphql.Object) *graphql.Object {
	if (o.Parent == rootQuery || o.Parent == rootMutation || o.Parent == nil) && o.Caller == nil {
		return o
	}

	if o.Parent == nil {
		caller := *o.Caller
		caller.Args = []*graphql.Object{o}
		return getRootObj(&caller)
	}

	parent := *o.Parent
	parent.Fields = []*graphql.Object{o}

	return getRootObj(&parent)
}

func getMinFields(o *graphql.Object) *graphql.Object {
	for _, f := range o.Fields {
		if f.Type.IsScalar() {
			this := *o
			this.Fields = []*graphql.Object{f}
			return &this
		}
	}

	if len(o.Fields) == 0 {
		return nil
	}

	for _, f := range o.Fields {
		simple := getMinFields(f)
		if simple != nil {
			this := *o
			this.Fields = []*graphql.Object{simple}
			return &this
		}
	}

	return nil
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

func isKnownScalar(t string) bool {
	typeRef := graphql.TypeRefFromString(t, "")
	rootType := typeRef.RootName()
	for _, known := range knownScalarTypes {
		if rootType == known {
			return true
		}
	}

	return false
}

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
		requiredArgRe := func(name string) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf(`Field "%s" argument "(%s)" of type "([^"]+)" is required`, regexp.QuoteMeta(name), graphqlNameRe))
		}
		fieldNotDefinedRe := func(name string) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf(`Field "%s" is not defined by %s`, regexp.QuoteMeta(name), graphqlNameRe))
		}
		expectedTypeRe := func(name string) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf("Expected type ([^,]+), found %s", regexp.QuoteMeta(name)))
		}
		expectingTypeRe := func(variable *graphql.Variable) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf(`Variable "\$%s" of type "%s" used in position expecting type "([^"]+)"`, regexp.QuoteMeta(variable.Name), regexp.QuoteMeta(variable.Type.String())))
		}

		didYouMeanRe := regexp.MustCompile(`Did you mean (.*)\?`)
		// graphqlRe := regexp.MustCompile(fmt.Sprintf(`"(%s)(?: .*)?"`, graphqlNameRe))
		graphqlRe := regexp.MustCompile(graphqlNameRe)
		orRe := regexp.MustCompile(" or ")

		rootQuery = &graphql.Object{
			Name:     "Query",
			Template: "query {{.Body}}",
		}
		push(&Job{
			Priority: 100,
			Type:     fieldTypeJob,
			Object:   rootQuery,
		})

		rootMutation = &graphql.Object{
			Name:     "Mutation",
			Template: "mutation {{.Body}}",
		}
		push(&Job{
			Priority: 100,
			Type:     fieldTypeJob,
			Object:   rootMutation,
		})

		fieldWorker := func(o *graphql.Object, loc Location, words chan string, results chan Result, wg *sync.WaitGroup) {
			defer wg.Done()

			obj := *o

			fuzzField := &graphql.Object{}
			obj.Fields = []*graphql.Object{fuzzField}

			re := queryFieldRe
			if loc == locArgField {
				fuzzField.Type = graphql.TypeRef{
					Kind: graphql.KindEnum,
				}
				fuzzField.SetValue("graphqshell_arg_field")

				re = fieldNotDefinedRe
			}

			for word := range words {
				fuzzField.Name = word

				resp, err := client.PostJSON(getRootObj(&obj))
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					continue
				}

				handled := false
				found := false
				for _, e := range resp.Result.Errors {
					if !re(word).MatchString(e.Message) {
						continue
					}

					handled = true

					didYouMeanMatches := didYouMeanRe.FindStringSubmatch(e.Message)
					if len(didYouMeanMatches) == 0 {
						continue
					}

					suggestions := didYouMeanMatches[1]
					suggestions = orRe.ReplaceAllString(suggestions, " ")
					matches := graphqlRe.FindAllString(suggestions, -1)
					for _, field := range matches {
						found = true

						if shouldIgnoreField(field) {
							continue
						}

						results <- &FuzzResult{
							Text:     field,
							Location: loc,
							obj:      o,
						}
					}
				}

				// Can happen when the word is an exact match
				if !handled && !found {
					results <- &FuzzResult{
						Text:     word,
						Location: loc,
						obj:      o,
					}
				}
			}
		}
		fuzzFields := func(o *graphql.Object, loc Location) chan Result {
			c := make(chan Result)

			go func() {
				wg := &sync.WaitGroup{}
				words := getWords()
				for i := 0; i < threads; i++ {
					wg.Add(1)
					go fieldWorker(o, loc, words, c, wg)
				}

				wg.Wait()
				close(c)
			}()

			return c
		}

		// shallowCopy := func(o *graphql.Object) *graphql.Object {
		// 	args := make([]*graphql.Object, len(o.Args))
		// 	copy(args, o.Args)

		// 	fields := make([]*graphql.Object, len(o.Fields))
		// 	copy(fields, o.Fields)

		// 	possibleValues := make([]*graphql.Object, len(o.PossibleValues))
		// 	copy(possibleValues, o.PossibleValues)

		// 	return &graphql.Object{
		// 		Name:           o.Name,
		// 		Description:    o.Description,
		// 		Type:           o.Type,
		// 		Args:           args,
		// 		Fields:         fields,
		// 		PossibleValues: possibleValues,
		// 		Parent:         o.Parent,
		// 		Caller:         o.Caller,
		// 		Template:       o.Template,
		// 	}
		// }

		argsWorker := func(o *graphql.Object, words chan string, results chan Result, wg *sync.WaitGroup) {
			defer wg.Done()

			obj := *o

			obj.Fields = []*graphql.Object{
				{Name: "graphqlshell_field"},
			}

			fuzzArg := &graphql.Object{}
			obj.Args = []*graphql.Object{fuzzArg}

			for word := range words {
				fuzzArg.Name = word

				resp, err := client.PostJSON(getRootObj(&obj))
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					continue
				}

				handled := false
				found := false
				unknownArgRe := regexp.MustCompile(fmt.Sprintf(`Unknown argument "%s"`, word))
				for _, e := range resp.Result.Errors {
					if !unknownArgRe.MatchString(e.Message) {
						continue
					}

					handled = true
					didYouMeanMatches := didYouMeanRe.FindStringSubmatch(e.Message)
					if len(didYouMeanMatches) == 0 {
						continue
					}

					suggestions := didYouMeanMatches[1]
					suggestions = orRe.ReplaceAllString(suggestions, " ")
					if len(didYouMeanMatches) == 0 {
						continue
					}

					matches := graphqlRe.FindAllString(suggestions, -1)
					for _, arg := range matches {
						found = true
						results <- &FuzzResult{
							Text:     arg,
							Location: locArg,
							obj:      o,
						}
					}
				}

				// Can happen when the word is an exact match
				if !handled && !found {
					results <- &FuzzResult{
						Text:     word,
						Location: locArg,
						obj:      o,
					}
				}
			}
		}
		fuzzArgs := func(o *graphql.Object) chan Result {
			c := make(chan Result)

			go func() {
				obj := *o
				obj.Fields = append(obj.Fields, &graphql.Object{
					Name: "graphqshell_field",
				})

				resp, err := client.PostJSON(&obj)
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
				} else {
					for _, e := range resp.Result.Errors {
						matches := requiredArgRe(obj.Name).FindAllStringSubmatch(e.Message, -1)
						if len(matches) == 0 {
							continue
						}

						c <- &FuzzResult{
							Text:     matches[0][1],
							Location: locArg,
							obj:      o,
						}
					}
				}

				wg := &sync.WaitGroup{}
				words := getWords()
				for i := 0; i < threads; i++ {
					wg.Add(1)
					go argsWorker(o, words, c, wg)
				}

				wg.Wait()
				close(c)
			}()

			return c
		}

		determineFieldType := func(o *graphql.Object) chan Result {
			c := make(chan Result)

			obj := *o
			go func() {
				defer close(c)

				name := "graphqlshell_field"
				obj.Fields = append(obj.Fields, &graphql.Object{
					Name: name,
				})

				resp, err := client.PostJSON(getRootObj(&obj))
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					return
				}

				result := &TypeResult{
					Location: locField,
					Kind:     graphql.KindObject,
					obj:      o,
				}

				for _, e := range resp.Result.Errors {
					matches := queryFieldRe(name).FindAllStringSubmatch(e.Message, -1)
					if len(matches) == 0 {
						matches = noSubfieldsRe(obj.Name).FindAllStringSubmatch(e.Message, -1)

						if len(matches) == 0 {
							continue
						}

						result.Kind = graphql.KindScalar
						result.Type = matches[0][1]
						break
					}

					if o.Name == rootQuery.Name || o.Name == rootMutation.Name {
						result.Type = matches[0][1]
						break
					}
				}

				if result.Type == "" {
					obj.Fields = []*graphql.Object{}

					rootObj := getRootObj(&obj)
					resp, err = client.PostJSON(rootObj)
					if err != nil {
						fmt.Printf("[!] Error posting: %v\n", err)
						return
					}

					for _, e := range resp.Result.Errors {
						matches := fieldOfTypeRe(obj.Name).FindAllStringSubmatch(e.Message, -1)
						if len(matches) == 0 {
							continue
						}

						result.Type = matches[0][1]
						break
					}

					if result.Type == "" {
						return
					}
				}

				c <- result
			}()

			return c
		}
		determineArgType := func(o *graphql.Object) chan Result {
			c := make(chan Result)

			obj := *o
			go func() {
				defer close(c)

				caller := getMinFields(obj.Caller)
				if caller == nil {
					push(&Job{
						Priority: 20,
						Type:     argTypeJob,
						Object:   o,
					})
					return
				}

				obj.Caller = caller

				name := "graphqshell_arg"

				variable := &graphql.Variable{
					Name:  "var",
					Value: name,
					Type: graphql.TypeRef{
						Name: name,
					},
				}

				obj.SetValue(variable)

				// obj.Fields = append(obj.Fields, &graphql.Object{
				// 	Name: name,
				// })

				resp, err := client.PostJSON(getRootObj(&obj), variable)
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					return
				}

				result := &TypeResult{
					Location: locArg,
					Kind:     graphql.KindObject,
					obj:      o,
				}

				for _, e := range resp.Result.Errors {
					matches := expectingTypeRe(variable).FindStringSubmatch(e.Message)
					if len(matches) > 0 {
						result.Type = matches[1]
						break
					}

					matches = expectedTypeRe(name).FindStringSubmatch(e.Message)
					if len(matches) == 0 {
						fmt.Println("Type not found")
						continue
					}

					result.Type = matches[1]
					break
				}

				if isKnownScalar(result.Type) {
					result.Kind = graphql.KindScalar
				}

				if result.Type == "" {
					obj.Fields = []*graphql.Object{}

					rootObj := getRootObj(&obj)
					resp, err = client.PostJSON(rootObj)
					if err != nil {
						fmt.Printf("[!] Error posting: %v\n", err)
						return
					}

					for _, e := range resp.Result.Errors {
						matches := fieldOfTypeRe(obj.Name).FindAllStringSubmatch(e.Message, -1)
						if len(matches) == 0 {
							continue
						}

						result.Type = matches[0][1]
						break
					}

					if result.Type == "" {
						return
					}
				}

				c <- result
			}()

			return c
		}
		determineArgFieldType := func(o *graphql.Object) chan Result {
			c := make(chan Result)

			// obj := *o
			go func() {
				defer close(c)
			}()

			return c
		}

		for {
			job, ok := pop()
			if !ok || job == nil {
				break
			}

			fmt.Printf("[%s] %s\n", job.Type, job.Object.Name)

			var results chan Result
			switch job.Type {
			case fieldJob:
				resolveStack.Push(job.Object.Type.RootName())
				results = fuzzFields(job.Object, locField)
			case fieldTypeJob:
				results = determineFieldType(job.Object)
			case argJob:
				results = fuzzArgs(job.Object)
			case argTypeJob:
				results = determineArgType(job.Object)
			case argFieldJob:
				resolveStack.Push(job.Object.Type.RootName())
				results = fuzzFields(job.Object, locArgField)
			case argFieldTypeJob:
				results = determineArgFieldType(job.Object)
			default:
				panic(fmt.Sprintf("Unknown job type: %s", job.Type))
			}

			for result := range results {
				obj := result.Object()

				switch r := result.(type) {
				case *FuzzResult:
					fuzzed := &graphql.Object{
						Name:   r.Text,
						Parent: obj,
					}

					if r.Location == locArg {
						if obj.AddArg(fuzzed) {
							fuzzed.Parent = nil
							fuzzed.Caller = obj

							fmt.Printf("[%s] Found arg: %s.%s(%s)\n", job.Type, obj.Parent.Name, obj.Name, fuzzed.Name)

							push(&Job{
								Priority: 50,
								Type:     argTypeJob,
								Object:   fuzzed,
							})
						}
					} else if obj.AddField(fuzzed) {
						fmt.Printf("[%s] Found %s: %s.%s\n", job.Type, r.Location, obj.Name, fuzzed.Name)

						if r.Location == locField {
							push(&Job{
								Priority: 100,
								Type:     fieldTypeJob,
								Object:   fuzzed,
							})
							// push(&Job{
							// 	Priority: 55,
							// 	Type:     argJob,
							// 	Object:   fuzzed,
							// })
						} else {
							push(&Job{
								Priority: 75,
								Type:     argFieldTypeJob,
								Object:   fuzzed,
							})
						}
					}
				case *TypeResult:
					if obj.Name == rootQuery.Name || obj.Name == rootMutation.Name {
						obj.Name = r.Type
					} else {
						parent := obj.Parent
						if parent == nil {
							parent = obj.Caller
						}

						fmt.Printf("[%s] Found %s type: %s.%s %s\n", job.Type, r.Location, parent.Name, obj.Name, r.Type)

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

					switch r.Location {
					case locField:
						push(&Job{
							Priority: 25,
							Type:     fieldJob,
							Object:   obj,
						})
					case locArg, locArgField:
						push(&Job{
							Priority: 30,
							Type:     argFieldJob,
							Object:   obj,
						})
					}
				}
			}

			if job.Type == fieldJob || job.Type == argFieldJob {
				resolveStack.Pop()

				rootName := job.Object.Type.RootName()
				defered := deferResolve[rootName]
				for _, fn := range defered {
					fn(job.Object)
				}

				delete(deferResolve, rootName)

				if job.Type == fieldJob && job.Object.Parent != nil {
					push(&Job{
						Priority: 55,
						Type:     argJob,
						Object:   job.Object,
					})
				}

				objCache[rootName] = job.Object

				if job.Object.Name == rootQuery.Name {
					for _, f := range rootQuery.Fields {
						f.Template = graphql.QueryTemplate
						// f.Parent = nil
					}
				}

				if job.Object.Name == rootMutation.Name {
					for _, f := range rootMutation.Fields {
						f.Template = graphql.MutationTemplate
						// f.Parent = nil
					}
				}
			}
		}

		resp := graphql.ToIntrospection(&graphql.RootQuery{
			Name:    rootQuery.Name,
			Queries: rootQuery.Fields,
		}, &graphql.RootMutation{
			Name:      rootMutation.Name,
			Mutations: rootMutation.Fields,
		})

		bytes, err := json.Marshal(&resp)
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
	schemaFuzzCmd.Flags().StringSliceP("headers", "H", []string{}, "Any extra headers needed")
	// schemaFuzzCmd.Flags().StringP("method", "m", http.MethodGet, "The request method")
	schemaFuzzCmd.Flags().String("proxy", "", "The proxy to use")
	// schemaFuzzCmd.Flags().Int("delay", 0, "How long the request should ")
	schemaFuzzCmd.Flags().IntP("threads", "t", 1, "Number of threads")
}
