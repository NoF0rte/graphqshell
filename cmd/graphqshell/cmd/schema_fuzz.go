package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/NoF0rte/graphqshell/internal/ds"
	"github.com/NoF0rte/graphqshell/internal/graphql"
	"github.com/analog-substance/fileutil"
	"github.com/emirpasic/gods/queues/priorityqueue"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/emirpasic/gods/stacks/arraystack"
	"github.com/emirpasic/gods/utils"
	"github.com/spf13/cobra"
)

type jobType string

const (
	jobField             jobType = "FIELD"
	jobEnum              jobType = "ENUM"
	jobArg               jobType = "ARG"
	jobArgField          jobType = "ARG_FIELD"
	jobArgType           jobType = "ARG_TYPE"
	jobArgFieldType      jobType = "ARG_FIELD_TYPE"
	jobFieldType         jobType = "FIELD_TYPE"
	jobRequiredArgs      jobType = "REQUIRED_ARGS"
	jobRequiredArgFields jobType = "REQUIRED_ARG_FIELDS"
)

type Location string

const (
	locField     Location = "FIELD"
	locArg       Location = "ARG"
	locArgField  Location = "ARG_FIELD"
	locEnum      Location = "ENUM"
	locInterface Location = "INTERFACE"
)

type Job struct {
	Priority int
	Type     jobType
	Object   *graphql.Object
	Previous *Job
}

func (j *Job) isAnyType(types ...jobType) bool {
	for _, t := range types {
		if j.Type == t {
			return true
		}
	}
	return false
}

var (
	cacheMutex   *sync.Mutex                    = &sync.Mutex{}
	objCache     map[string]*graphql.Object     = make(map[string]*graphql.Object)
	jobQueue     *priorityqueue.Queue           = priorityqueue.NewWith(byPriority)
	fuzzMutex    *sync.Mutex                    = &sync.Mutex{}
	deferResolve map[string][]func(interface{}) = make(map[string][]func(interface{}))
	resolveStack *arraystack.Stack              = arraystack.New()

	ignoreFields []string = []string{
		"__type",
		"__schema",
	}

	foundWordsSet *ds.ThreadSafeSet = ds.NewThreadSafeSet()
	knownScalars  *ds.ThreadSafeSet = ds.NewThreadSafeSet(
		"Float",
		"String",
		"Int",
		"Boolean",
		"ID",
	)
	knownEnums *ds.ThreadSafeSet = ds.NewThreadSafeSet()

	rootQuery    *graphql.Object
	rootMutation *graphql.Object
	currentJob   *Job
)

const (
	batchFieldSize int = 64
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

type RequiredResult struct {
	Text     string
	Type     string
	Location Location

	obj *graphql.Object
}

func (r *RequiredResult) Object() *graphql.Object {
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

func isQueued(t jobType, rootName string) bool {
	for _, j := range jobQueue.Values() {
		job := j.(*Job)
		if job.Type == t && job.Object.Type.RootName() == rootName {
			return true
		}
	}

	return false
}

func push(job *Job) {
	fuzzMutex.Lock()
	jobQueue.Enqueue(job)
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

func isRoot(o *graphql.Object) bool {
	return (o.Parent == rootQuery || o.Parent == rootMutation || o.Parent == nil) && o.Caller == nil
}

type direction int

const (
	directionRoot direction = iota
	directionCaller
)

func walkObject(o *graphql.Object, dir direction) *graphql.Object {
	if dir == directionRoot {
		if isRoot(o) {
			return o
		}

		if o.Parent == nil {
			caller := copyFromCache(o.Caller)

			args := []*graphql.Object{o}
			for _, arg := range caller.Args {
				if arg.Name != o.Name && arg.Type.IsRequired() {
					args = append(args, copyFromCache(arg))
				}
			}

			caller.Args = args
			return walkObject(caller, dir)
		}

		parent := copyFromCache(o.Parent)
		if parent.GetPossibleValue(o.Name) != nil {
			parent.PossibleValues = []*graphql.Object{o}
		} else {
			parent.Fields = []*graphql.Object{o}
		}

		return walkObject(parent, dir)
	}

	if o.Caller != nil {
		caller := copyFromCache(o.Caller)

		args := []*graphql.Object{o}
		for _, a := range caller.Args {
			if a.Name != o.Name && a.Type.IsRequired() {
				args = append(args, copyFromCache(a))
			}
		}

		caller.Args = args
		caller.SetValue(nil)
		return caller
	}

	parent := copyFromCache(o.Parent)

	fields := []*graphql.Object{o}
	for _, f := range parent.Fields {
		if f.Name != o.Name && f.Type.IsRequired() {
			fields = append(fields, copyFromCache(f))
		}
	}

	parent.Fields = fields
	parent.SetValue(nil)

	return walkObject(parent, dir)
}

func updateCache(t string, o *graphql.Object) {
	// cacheMutex.Lock()
	// defer cacheMutex.Unlock()

	objCache[t] = o
}

func getCached(t string) (*graphql.Object, bool) {
	// cacheMutex.Lock()
	// defer cacheMutex.Unlock()

	cached, ok := objCache[t]
	return cached, ok
}

func tryGetCached(o *graphql.Object) *graphql.Object {
	// cacheMutex.Lock()
	// defer cacheMutex.Unlock()

	cached := objCache[o.Type.RootName()]
	if cached != nil {
		return cached
	}
	return o
}

func shallowMergeUpdate(o1 *graphql.Object, o2 *graphql.Object) {
	o1.Args = o2.Args
	o1.Fields = o2.Fields
	o1.PossibleValues = o2.PossibleValues
}

func shallowMergeNew(o1 *graphql.Object, o2 *graphql.Object) *graphql.Object {
	merged := &graphql.Object{
		Name:     o1.Name,
		Type:     o1.Type,
		Parent:   o1.Parent,
		Caller:   o1.Caller,
		Template: o1.Template,
	}
	shallowMergeUpdate(merged, o2)
	return merged
}

func copyFromCache(o *graphql.Object) *graphql.Object {
	cached := tryGetCached(o)
	return shallowMergeNew(o, cached)
}

// func getRootObj(o *graphql.Object) *graphql.Object {
// 	if (o.Parent == rootQuery || o.Parent == rootMutation || o.Parent == nil) && o.Caller == nil {
// 		return o
// 	}

// 	if o.Parent == nil {
// 		caller := *getCached(o.Caller)

// 		args := []*graphql.Object{o}
// 		for _, arg := range caller.Args {
// 			if arg.Name != o.Name && arg.Type.IsRequired() {
// 				args = append(args, getCached(arg))
// 			}
// 		}

// 		caller.Args = args
// 		return getRootObj(&caller)
// 	}

// 	parent := *o.Parent
// 	if parent.GetPossibleValue(o.Name) != nil {
// 		parent.PossibleValues = []*graphql.Object{o}
// 	} else {
// 		parent.Fields = []*graphql.Object{o}
// 	}

// 	return getRootObj(&parent)
// }

// func getCallerObj(o *graphql.Object) *graphql.Object {
// 	if o.Caller != nil {
// 		caller := *o.Caller

// 		args := []*graphql.Object{o}
// 		for _, a := range caller.Args {
// 			if a.Name != o.Name && a.Type.IsRequired() {
// 				args = append(args, a)
// 			}
// 		}
// 		caller.Args = args
// 		caller.SetValue(nil)
// 		return &caller
// 	}

// 	parent := *o.Parent

// 	fields := []*graphql.Object{o}
// 	for _, f := range parent.Fields {
// 		if f.Name != o.Name && f.Type.IsRequired() {
// 			fields = append(fields, f)
// 		}
// 	}
// 	parent.Fields = fields
// 	parent.SetValue(nil)

// 	return getCallerObj(&parent)
// }

func getMinFields(o *graphql.Object) *graphql.Object {
	for _, f := range o.Fields {
		if f.Type.IsScalar() {
			this := *o
			// this := copyFromCache(o)
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

// func assignNewParent(items []*graphql.Object, parent *graphql.Object) []*graphql.Object {
// 	var newItems []*graphql.Object
// 	for _, item := range items {
// 		i := *item
// 		i.Parent = parent

// 		newItems = append(newItems, &i)
// 	}

// 	return newItems
// }

// Could be bad since we are shallow copying the item which means if the original item gets updated with it's type
// this item won't get the update, right?
// func assignNewCaller(items []*graphql.Object, caller *graphql.Object) []*graphql.Object {
// 	var newItems []*graphql.Object
// 	for _, item := range items {
// 		i := *item
// 		i.Caller = caller

// 		newItems = append(newItems, &i)
// 	}

// 	return newItems
// }

func isKnownScalar(t string) bool {
	typeRef := graphql.TypeRefFromString(t, "")
	rootType := typeRef.RootName()
	return knownScalars.Contains(rootType)
}

func isInferredScalar(t string) bool {
	rootName := graphql.TypeRefFromString(t, "").RootName()
	re := regexp.MustCompile(`Int(?:[A-Z]|\b)|[Ii]nteger|[Ss]tring|[Dd]ate[Tt]ime|[Dd]ate|[Tt]ime|URL|URI`)
	return re.MatchString(rootName)
}

func isKnownOrInferredScalar(t string) bool {
	return isKnownScalar(t) || isInferredScalar(t)
}

func isKnownEnum(t string) bool {
	typeRef := graphql.TypeRefFromString(t, "")
	rootType := typeRef.RootName()
	return knownEnums.Contains(rootType)
}

func isInferredEnum(t string) bool {
	rootName := graphql.TypeRefFromString(t, "").RootName()
	re := regexp.MustCompile(`[Ee]num`)
	return re.MatchString(rootName)
}

func isKnownOrInferredEnum(t string) bool {
	return isKnownEnum(t) || isInferredEnum(t)
}

func objPath(o *graphql.Object, input string) string {
	if input == "" {
		input = o.Name
	} else {
		input = fmt.Sprintf("%s.%s", o.Name, input)
	}

	if o.Parent == nil && o.Caller == nil {
		return input
	}

	if o.Parent == nil {
		return objPath(o.Caller, input)
	}

	return objPath(o.Parent, input)
}

func addFoundWord(word string) {
	foundWordsSet.Add(word)
	if len(word) > 1 {
		foundWordsSet.Add(word[:len(word)-1])
	}
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
		threads, _ := cmd.Flags().GetInt("threads")
		fuzzTargets, _ := cmd.Flags().GetStringSlice("fuzz")

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

		graphql.ValueCaching = false

		lines, err := fileutil.ReadLines(wordlist)
		if err != nil {
			fmt.Printf("[!] Error: %v\n", err)
			return
		}

		getWords := func(foundWords []interface{}) chan string {
			c := make(chan string)

			set := hashset.New()
			for _, line := range lines {
				set.Add(line)
			}
			for _, found := range foundWords {
				set.Add(found)
			}
			combined := set.Values()

			go func() {
				for _, word := range combined {
					c <- word.(string)
				}

				close(c)
			}()

			return c
		}

		graphqlNameRe := "[_A-Za-z][_0-9A-Za-z]*"

		nonEnumValueRe := regexp.MustCompile(`Enum "[^"]+" cannot represent non-enum value: (.*)\.`)
		didYouMeanRe := regexp.MustCompile(`Did you mean(?: the enum value| to use an inline fragment on)? (.*)\?$`)
		graphqlRe := regexp.MustCompile(graphqlNameRe)
		orRe := regexp.MustCompile(" or ")

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
		requiredArgFieldRe := func(t string) *regexp.Regexp {
			return regexp.MustCompile(fmt.Sprintf(`Field "?%s\.(%s)"? of required type "?(.*?)"? was not provided`, regexp.QuoteMeta(t), graphqlNameRe))
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
		enumNotExistsRe := func(name string, t string) *regexp.Regexp {
			if t == "" {
				regexp.MustCompile(fmt.Sprintf(`Value "%s" does not exist in "[^"]+" enum`, regexp.QuoteMeta(name)))
			}
			return regexp.MustCompile(fmt.Sprintf(`Value "%s" does not exist in "%s" enum`, regexp.QuoteMeta(name), regexp.QuoteMeta(t)))
		}

		rootQuery = &graphql.Object{
			Name:     "Query",
			Template: "query {{.Body}}",
		}
		rootMutation = &graphql.Object{
			Name:     "Mutation",
			Template: "mutation {{.Body}}",
		}

		for _, target := range fuzzTargets {
			root, field, _ := strings.Cut(target, ".")
			obj := &graphql.Object{
				Name: field,
			}

			isQuery := strings.EqualFold(root, "query")
			isMutation := strings.EqualFold(root, "mutation")

			job := &Job{
				Priority: 100,
				Type:     jobFieldType,
				Object:   obj,
			}

			if field == "" && isQuery {
				job.Object = rootQuery
			} else if field == "" && isMutation {
				job.Object = rootMutation
			} else if isQuery {
				obj.Template = graphql.QueryTemplate
				obj.Parent = rootQuery
				rootQuery.AddField(obj)
			} else {
				obj.Template = graphql.MutationTemplate
				obj.Parent = rootMutation
				rootMutation.AddField(obj)
			}

			push(job)
		}

		fieldWorker := func(o *graphql.Object, loc Location, words chan string, results chan Result, wg *sync.WaitGroup) {
			defer wg.Done()

			obj := copyFromCache(o)

			re := queryFieldRe
			createField := func(word string) *graphql.Object {
				return &graphql.Object{
					Name: word,
				}
			}

			if loc == locArgField {
				obj.SetValue(nil)

				createField = func(word string) *graphql.Object {
					f := &graphql.Object{
						Name: word,
						Type: graphql.TypeRef{
							Kind: graphql.KindEnum,
						},
					}
					f.SetValue("graphqshell_arg_field")
					return f
				}

				re = fieldNotDefinedRe
			}

			for {
				count := 0
				var fields []*graphql.Object
				for word := range words {
					if count == batchFieldSize {
						break
					}

					fields = append(fields, createField(word))
					count++
				}

				if count == 0 {
					break
				}

				rootObj := walkObject(obj, directionRoot)

				obj.Fields = fields

				resp, err := client.PostJSON(rootObj)
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					continue
				}

				if resp.RawResponse.StatusCode >= 500 {
					fmt.Printf("[!] Server error: %s\n", resp.RawResponse.Status)
					continue
				}

				process := func(word string, resp *graphql.Response) (handled bool, found bool, shouldContinue bool) {
					shouldContinue = true

					for _, e := range resp.Result.Errors {
						// If this matches, then it is an enum and doesn't have fields
						if enumNotExistsRe(word, "").MatchString(e.Message) {
							shouldContinue = false
							return
						}

						// If this matches, then it is an enum and doesn't have fields
						matches := nonEnumValueRe.FindStringSubmatch(e.Message)
						if len(matches) > 0 && strings.Contains(matches[1], fmt.Sprintf("%s: graphqshell_arg_field", word)) {
							shouldContinue = false
							return
						}

						if !re(word).MatchString(e.Message) {
							continue
						}

						handled = true

						didYouMeanMatches := didYouMeanRe.FindStringSubmatch(e.Message)
						if len(didYouMeanMatches) == 0 {
							continue
						}

						fuzzLoc := loc
						if strings.Contains(e.Message, "inline fragment") {
							if loc != locField {
								fmt.Println("[!] Found inline fragment on non object field")
							}

							fuzzLoc = locInterface
						}

						suggestions := didYouMeanMatches[1]
						suggestions = orRe.ReplaceAllString(suggestions, " ")
						matches = graphqlRe.FindAllString(suggestions, -1)
						if len(matches) == 0 {
							continue
						}

						for _, field := range matches {
							found = true

							if shouldIgnoreField(field) {
								continue
							}

							results <- &FuzzResult{
								Text:     field,
								Location: fuzzLoc,
								obj:      o,
							}

							addFoundWord(field)
						}

						break
					}
					return
				}

				for _, f := range fields {
					word := f.Name

					handled, found, shouldContinue := process(word, resp)
					if !shouldContinue {
						break
					}

					// Can happen when the word is an exact match
					if !handled && !found {
						// If the word is 1 letter, assume it is an exact match for now
						if len(word) == 1 {
							results <- &FuzzResult{
								Text:     word,
								Location: loc,
								obj:      o,
							}
							continue
						}

						name := word[:len(word)-1]
						obj.Fields = []*graphql.Object{
							createField(name),
						}

						resp2, err := client.PostJSON(rootObj)
						if err != nil {
							fmt.Printf("[!] Error posting: %v\n", err)
							continue
						}

						_, _, shouldContinue = process(name, resp2)
						if !shouldContinue {
							break
						}
					}
				}
			}
		}
		fuzzFields := func(o *graphql.Object, loc Location) chan Result {
			c := make(chan Result)

			foundWords := foundWordsSet.Values()
			go func() {
				wg := &sync.WaitGroup{}
				words := getWords(foundWords)
				for i := 0; i < threads; i++ {
					wg.Add(1)
					go fieldWorker(o, loc, words, c, wg)
				}

				wg.Wait()
				close(c)
			}()

			return c
		}

		enumWorker := func(o *graphql.Object, loc Location, words chan string, results chan Result, wg *sync.WaitGroup) {
			defer wg.Done()

			obj := copyFromCache(o)

			caller := walkObject(obj, directionCaller)
			caller = getMinFields(caller)
			if caller == nil {
				return
				// push(&Job{
				// 	Priority: 20,
				// 	Type:     argTypeJob,
				// 	Object:   o,
				// })
				// return
			}

			var rootObj *graphql.Object
			if loc == locArgField {
				// caller := walkObject(obj, directionCaller)
				// caller = getMinFields(caller)

				// rootObj = walkObject(caller, directionRoot)
				// if isRoot(caller) {
				// 	rootObj = shallowMergeNew(caller, caller) // Create new object
				// }
			} else {
				// caller := getMinFields(obj.Caller)
				// if caller == nil {
				// 	return
				// 	// push(&Job{
				// 	// 	Priority: 20,
				// 	// 	Type:     argTypeJob,
				// 	// 	Object:   o,
				// 	// })
				// 	// return
				// }

				obj.Caller = caller

				// rootObj = walkObject(caller, directionRoot)
				// if isRoot(caller) {
				// 	rootObj = shallowMergeNew(caller, caller) // Create new object
				// }
			}

			rootObj = walkObject(caller, directionRoot)

			rootName := obj.Type.RootName()
			handled := false
			found := false

			process := func(word string, resp *graphql.Response) {
				for _, e := range resp.Result.Errors {
					if !enumNotExistsRe(word, rootName).MatchString(e.Message) && !expectedTypeRe(word).MatchString(e.Message) {
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
					for _, match := range matches {
						found = true
						results <- &FuzzResult{
							Text:     match,
							Location: locEnum,
							obj:      o,
						}
					}
				}
			}

			for word := range words {
				obj.SetValue(word)

				resp, err := client.PostJSON(rootObj)
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					continue
				}

				process(word, resp)

				// Can happen when the word is an exact match
				if !handled && !found {
					if len(word) == 1 {
						results <- &FuzzResult{
							Text:     word,
							Location: locEnum,
							obj:      o,
						}
						continue
					}

					value := word[:len(word)-1]
					obj.SetValue(value)

					resp2, err := client.PostJSON(rootObj)
					if err != nil {
						fmt.Printf("[!] Error posting: %v\n", err)
						continue
					}
					process(value, resp2)
				}
			}
		}
		fuzzEnumValues := func(o *graphql.Object, loc Location) chan Result {
			c := make(chan Result)

			go func() {
				wg := &sync.WaitGroup{}
				words := getWords(foundWordsSet.Values())
				for i := 0; i < threads; i++ {
					wg.Add(1)
					go enumWorker(o, loc, words, c, wg)
				}

				wg.Wait()
				close(c)
			}()

			return c
		}

		argsWorker := func(o *graphql.Object, words chan string, results chan Result, wg *sync.WaitGroup) {
			defer wg.Done()

			obj := copyFromCache(o)

			obj.Fields = []*graphql.Object{
				{Name: "graphqlshell_field"},
			}

			for {
				count := 0
				var args []*graphql.Object
				for word := range words {
					if count == batchFieldSize {
						break
					}

					args = append(args, &graphql.Object{
						Name: word,
					})
					count++
				}

				if count == 0 {
					break
				}

				rootObj := walkObject(obj, directionRoot)

				obj.Args = args

				process := func(word string, resp *graphql.Response) (handled bool, found bool) {
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

							addFoundWord(arg)
						}
					}

					return
				}

				resp, err := client.PostJSON(rootObj)
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
					continue
				}

				// TODO: Make more efficient by not doing 2 loops
				for _, a := range args {
					word := a.Name

					handled, found := process(a.Name, resp)

					// Can happen when the word is an exact match
					// Let's do some checks to make sure it isn't a false positive
					if !handled && !found {
						if len(word) == 1 {
							results <- &FuzzResult{
								Text:     word,
								Location: locArg,
								obj:      o,
							}
							continue
						}

						name := word[:len(word)-1]
						obj.Args = []*graphql.Object{
							{
								Name: name,
							},
						}

						resp2, err := client.PostJSON(rootObj)
						if err != nil {
							fmt.Printf("[!] Error posting: %v\n", err)
							continue
						}

						// This should make it so if it was an exact match before
						// we should get the "did you mean" error
						process(name, resp2)
					}
				}
			}
		}
		fuzzArgs := func(o *graphql.Object) chan Result {
			c := make(chan Result)

			foundWords := foundWordsSet.Values()
			go func() {
				obj := copyFromCache(o)
				obj.Fields = append(obj.Fields, &graphql.Object{
					Name: "graphqshell_field",
				})

				wg := &sync.WaitGroup{}
				words := getWords(foundWords)
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

			obj := copyFromCache(o)
			go func() {
				defer close(c)

				rootObj := walkObject(obj, directionRoot)

				name := "graphqlshell_field"
				obj.Fields = append(obj.Fields, &graphql.Object{
					Name: name,
				})

				resp, err := client.PostJSON(rootObj)
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

				if isKnownScalar(result.Type) || isInferredScalar(result.Type) {
					result.Kind = graphql.KindScalar
				} else if isKnownEnum(result.Type) || isInferredEnum(result.Type) {
					result.Kind = graphql.KindEnum
				}

				c <- result
			}()

			return c
		}
		determineArgType := func(o *graphql.Object, loc Location) chan Result {
			c := make(chan Result)

			obj := copyFromCache(o)
			go func() {
				defer close(c)

				result := &TypeResult{
					Location: loc,
					Kind:     graphql.KindObject,
					obj:      o,
				}

				caller := walkObject(obj, directionCaller)
				caller = getMinFields(caller)
				if caller == nil {
					newPriority := currentJob.Priority - 10
					path := objPath(obj, "")
					if newPriority < 0 {
						fmt.Printf("[!] Stopping getting arg type for %s\n", path)
						return
					}

					fmt.Printf("[!] Skipping arg type job for %s until caller has scalar fields\n", path)
					push(&Job{
						Priority: newPriority,
						Type:     currentJob.Type,
						Object:   o,
					})
					return
				}

				var rootObj *graphql.Object
				if loc == locArgField {
					// caller = getMinFields(caller)
					// if caller == nil { // Should probably push a job that will fast track the scalar fields
					// 	newPriority := currentJob.Priority - 10
					// 	path := objPath(obj, "")
					// 	if newPriority < 0 {
					// 		fmt.Printf("[!] Stopping getting arg type for %s\n", path)
					// 		return
					// 	}

					// 	fmt.Printf("[!] Skipping arg field type job for %s until caller has scalar fields\n", path)
					// 	push(&Job{
					// 		Priority: newPriority,
					// 		Type:     currentJob.Type,
					// 		Object:   o,
					// 	})
					// 	return
					// }

					// rootObj = walkObject(caller, directionRoot)
					// if isRoot(caller) {
					// 	rootObj = shallowMergeNew(caller, caller) // Create new object
					// }
				} else {
					// caller = getMinFields(caller)
					// if caller == nil {
					// 	newPriority := currentJob.Priority - 10
					// 	path := objPath(obj, "")
					// 	if newPriority < 0 {
					// 		fmt.Printf("[!] Stopping getting arg type for %s\n", path)
					// 		return
					// 	}

					// 	fmt.Printf("[!] Skipping arg type job for %s until caller has scalar fields\n", path)
					// 	push(&Job{
					// 		Priority: newPriority,
					// 		Type:     currentJob.Type,
					// 		Object:   o,
					// 	})
					// 	return
					// }

					obj.Caller = caller

					// if isRoot(caller) {
					// 	rootObj = shallowMergeNew(caller, caller) // Create new object
					// }
				}

				rootObj = walkObject(caller, directionRoot)

				name := "graphqshell_arg"

				// If the type is partially filled out, then the arg is probably a required arg
				if obj.Type.RootName() != "" {
					result.Type = obj.Type.String()
				} else {
					variable := &graphql.Variable{
						Name:  obj.Name,
						Value: name,
						Type:  *graphql.TypeRefFromString("[Boolean!]!", graphql.KindScalar), // [Boolean!]! will probably rarely ever be the correct type so this ensures errors
					}
					obj.SetValue(variable)

					resp, err := client.PostJSON(rootObj, variable)
					if err != nil {
						fmt.Printf("[!] Error posting: %v\n", err)
						return
					}

					for _, e := range resp.Result.Errors {
						matches := expectingTypeRe(variable).FindStringSubmatch(e.Message)
						if len(matches) > 0 {
							result.Type = matches[1]
							break
						}

						matches = expectedTypeRe(name).FindStringSubmatch(e.Message)
						if len(matches) == 0 {
							fmt.Printf("Type not found: %s\n", e.Message)
							continue
						}

						result.Type = matches[1]
						break
					}
				}

				if isKnownOrInferredScalar(result.Type) {
					result.Kind = graphql.KindScalar
				} else if isKnownOrInferredEnum(result.Type) {
					result.Kind = graphql.KindEnum
				} else if len(obj.Fields) == 0 {
					typeRef := graphql.TypeRefFromString(result.Type, "")

					variable := &graphql.Variable{
						Name:  obj.Name,
						Value: map[string]interface{}{},
						Type:  *typeRef,
					}
					obj.SetValue(variable)

					resp, err := client.PostJSON(rootObj, variable)
					if err != nil {
						fmt.Printf("[!] Error posting: %v\n", err)
						return
					}

					nonScalarRe := regexp.MustCompile(`(non-?string)|(non-?integer)|(must be a string)`)
					enumRe := regexp.MustCompile(`\b[Ee]nums?\b`)
					for _, e := range resp.Result.Errors {
						if enumRe.MatchString(e.Message) {
							result.Kind = graphql.KindEnum
							break
						}
						if nonScalarRe.MatchString(e.Message) {
							result.Kind = graphql.KindScalar
							break
						}
					}
				}

				if result.Type != "" {
					c <- result
				}
			}()

			return c
		}
		determineRequiredInputs := func(o *graphql.Object, loc Location) chan Result {
			c := make(chan Result)

			obj := copyFromCache(o)
			go func() {
				defer close(c)

				var rootObj *graphql.Object

				if loc == locArg {
					obj.Args = []*graphql.Object{}
					obj.Fields = []*graphql.Object{}
					obj.PossibleValues = []*graphql.Object{}
					rootObj = walkObject(obj, directionRoot)
				} else {
					caller := walkObject(obj, directionCaller)
					caller.Fields = []*graphql.Object{
						{
							Name: "graphqshell_field",
						},
					}
					caller.PossibleValues = []*graphql.Object{}

					rootObj = walkObject(caller, directionRoot)
					if isRoot(caller) {
						rootObj = shallowMergeNew(caller, caller) // Create new object
					}
				}

				resp, err := client.PostJSON(rootObj)
				if err != nil {
					fmt.Printf("[!] Error posting: %v\n", err)
				}

				for _, e := range resp.Result.Errors {
					matches := requiredArgRe(obj.Name).FindStringSubmatch(e.Message)
					if len(matches) > 0 {
						c <- &RequiredResult{
							Text:     matches[1],
							Type:     matches[2],
							Location: locArg,
							obj:      o,
						}
						continue
					}

					matches = requiredArgFieldRe(obj.Type.RootName()).FindStringSubmatch(e.Message)
					if len(matches) == 0 {
						continue
					}

					c <- &RequiredResult{
						Text:     matches[1],
						Type:     matches[2],
						Location: locArgField,
						obj:      o,
					}
				}
			}()

			return c
		}

		var ok bool
		for {
			currentJob, ok = pop()
			if !ok || currentJob == nil {
				break
			}

			fmt.Printf("[%s] %s %s\n", currentJob.Type, objPath(currentJob.Object, ""), currentJob.Object.Type.String())

			// Given the fact that we are gradually building up the different types
			// it might be better to defer everything to the end?

			cached, isCached := objCache[currentJob.Object.Type.RootName()]
			if isCached && currentJob.isAnyType(jobArgField, jobField, jobEnum) {
				// Maybe add to deferResolve?
				shallowMergeUpdate(currentJob.Object, cached)
				continue
			}

			hadResults := false
			var results chan Result
			switch currentJob.Type {
			case jobField:
				resolveStack.Push(currentJob.Object.Type.RootName())
				results = fuzzFields(currentJob.Object, locField)
			case jobFieldType:
				results = determineFieldType(currentJob.Object)
			case jobArg:
				results = fuzzArgs(currentJob.Object)
			case jobArgType:
				results = determineArgType(currentJob.Object, locArg)
			case jobArgField:
				resolveStack.Push(currentJob.Object.Type.RootName())
				results = fuzzFields(currentJob.Object, locArgField)
			case jobArgFieldType:
				results = determineArgType(currentJob.Object, locArgField)
			case jobRequiredArgs:
				results = determineRequiredInputs(currentJob.Object, locArg)
			case jobRequiredArgFields:
				results = determineRequiredInputs(currentJob.Object, locArgField)
			case jobEnum:
				resolveStack.Push(currentJob.Object.Type.RootName())
				if currentJob.Object.Parent != nil {
					results = fuzzEnumValues(currentJob.Object, locArgField)
				} else {
					results = fuzzEnumValues(currentJob.Object, locArg)
				}
			}

			for result := range results {
				hadResults = true

				obj := result.Object()

				switch r := result.(type) {
				case *FuzzResult:
					fuzzed := &graphql.Object{
						Name:   r.Text,
						Parent: obj,
					}

					switch r.Location {
					case locArg:
						if obj.AddArg(fuzzed) {
							fuzzed.Parent = nil
							fuzzed.Caller = obj

							fmt.Printf("[%s] Found: %s(%s)\n", currentJob.Type, objPath(obj, ""), fuzzed.Name)

							push(&Job{
								Priority: 50,
								Type:     jobArgType,
								Object:   fuzzed,
							})
						}
					case locEnum:
						if obj.AddPossibleValue(fuzzed) {
							fuzzed.Parent = nil
							fuzzed.Caller = nil

							fmt.Printf("[%s] Found: %s => %s\n", currentJob.Type, objPath(obj, ""), fuzzed.Name)
						}
					case locInterface:
						if obj.Type.RootKind() != graphql.KindInterface {
							obj.Type.SetRootKind(graphql.KindInterface)
						}

						// check the obj cache for the type

						if obj.AddPossibleValue(fuzzed) {
							fuzzed.Type = *graphql.TypeRefFromString(fuzzed.Name, graphql.KindObject)
							push(&Job{
								Priority: currentJob.Priority,
								Type:     jobField,
								Object:   fuzzed,
							})
						}
					default:
						if obj.AddField(fuzzed) {
							fmt.Printf("[%s] Found: %s.%s\n", currentJob.Type, objPath(obj, ""), fuzzed.Name)

							if r.Location == locField {
								push(&Job{
									Priority: 100,
									Type:     jobFieldType,
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
									Type:     jobArgFieldType,
									Object:   fuzzed,
								})
							}
						}
					}
				case *TypeResult:
					nextJob := &Job{
						Object:   obj,
						Previous: currentJob,
					}

					if obj.Name == rootQuery.Name || obj.Name == rootMutation.Name {
						obj.Name = r.Type

						nextJob.Priority = 25
						nextJob.Type = jobField
					} else {
						fmt.Printf("[%s] Found: %s %s\n", currentJob.Type, objPath(obj, ""), r.Type)

						ref := graphql.TypeRefFromString(r.Type, r.Kind)
						obj.Type = *ref

						rootName := ref.RootName()
						if rootName != "" {
							addFoundWord(rootName)
						}

						// Probably should change this so that the workers set the right kind
						if r.Kind == graphql.KindObject && isInferredScalar(r.Type) {
							r.Kind = graphql.KindScalar
							obj.Type.SetRootKind(r.Kind)
							// obj.Type = *graphql.TypeRefFromString(r.Type, r.Kind)
						} else if r.Kind == graphql.KindObject && isInferredEnum(r.Type) {
							r.Kind = graphql.KindEnum
							obj.Type.SetRootKind(r.Kind)
						}

						if (r.Kind == graphql.KindScalar || r.Kind == graphql.KindEnum) &&
							(r.Location == locArg || r.Location == locArgField) {

							if r.Kind == graphql.KindScalar {
								knownScalars.Add(rootName)
							} else {
								knownEnums.Add(rootName)
							}
						}

						if r.Kind == graphql.KindEnum && r.Location != locField {
							obj.SetValue("graphqshell_enum")

							nextJob.Priority = 20
							nextJob.Type = jobEnum
						} else {
							switch r.Location {
							case locField:
								nextJob.Priority = 25
								nextJob.Type = jobField
							case locArg, locArgField:
								nextJob.Priority = 25
								nextJob.Type = jobArgField
							}
						}

						// If we know the type is resolving, the job is queued,
						// or that it is an enum but on a normal field, defer the resolution
						if (r.Kind == graphql.KindEnum && r.Location == locField) || isResolving(rootName) || isQueued(nextJob.Type, rootName) {
							deferResolve[rootName] = append(deferResolve[rootName], func(i interface{}) {
								shallowMergeUpdate(obj, i.(*graphql.Object))
							})
							continue
						}

						o, cached := getCached(rootName)
						if cached {
							objKind := obj.Type.RootKind()
							resolvedKind := o.Type.RootKind()
							if resolvedKind != objKind && objKind == graphql.KindEnum {
								fmt.Println("blah")
							}

							shallowMergeUpdate(obj, o)
							continue
						}

						if r.Kind == graphql.KindScalar {
							continue
						}

						// // If we know it is an enum but it is a normal field, we need to make sure
						// // that we check to see if an enum job is queued for that type so we can update
						// // the values later. Should find a better way to do this
						// if r.Kind == graphql.KindEnum && r.Location == locField {
						// 	nextJob.Priority = 25
						// 	nextJob.Type = jobField
						// }

						// if r.Kind == graphql.KindEnum && r.Location != locField {

						// 	push(nextJob)
						// 	// push(&Job{
						// 	// 	Priority: 20,
						// 	// 	Type:     jobEnum,
						// 	// 	Object:   obj,
						// 	// })
						// 	continue
						// }
					}

					push(nextJob)

					// switch r.Location {
					// case locField:
					// 	push(nextJob)
					// 	// push(&Job{
					// 	// 	Priority: 25,
					// 	// 	Type:     jobField,
					// 	// 	Object:   obj,
					// 	// })
					// case locArg, locArgField:
					// 	push(nextJob)
					// 	// push(&Job{
					// 	// 	Priority: 30,
					// 	// 	Type:     jobArgField,
					// 	// 	Object:   obj,
					// 	// })
					// }
				case *RequiredResult:
					ref := *graphql.TypeRefFromString(r.Type, "")
					if isKnownOrInferredScalar(r.Type) {
						ref.SetRootKind(graphql.KindScalar)
						knownScalars.Add(ref.RootName())
					} else if isKnownOrInferredEnum(r.Type) {
						ref.SetRootKind(graphql.KindEnum)
						knownEnums.Add(ref.RootName())
					}

					fuzzed := &graphql.Object{
						Name:   r.Text,
						Type:   ref,
						Parent: obj,
					}

					rootName := fuzzed.Type.RootName()
					addFoundWord(rootName)

					if isKnownScalar(r.Type) {
						fuzzed.SetValue(nil)
						fuzzed.SetValue(fuzzed.GenValue())
					} else {
						fuzzed.SetValue(map[string]interface{}{})
					}

					if r.Location == locArg {
						fmt.Printf("[%s] Found: %s(%s %s)\n", currentJob.Type, objPath(obj, ""), fuzzed.Name, r.Type)

						fuzzed.Parent = nil
						fuzzed.Caller = obj

						obj.AddArg(fuzzed)

						if isKnownScalar(r.Type) {
							continue
						}

						if !isKnownEnum(r.Type) {
							if isResolving(rootName) {
								deferResolve[rootName] = append(deferResolve[rootName], func(i interface{}) {
									shallowMergeUpdate(obj, i.(*graphql.Object))
								})
								continue
							}

							o, cached := getCached(rootName)
							if cached {
								objKind := fuzzed.Type.RootKind()
								resolvedKind := o.Type.RootKind()
								if resolvedKind != objKind && objKind == graphql.KindEnum {
									fmt.Println("blah")
								}

								shallowMergeUpdate(fuzzed, o)
								continue
							}

							push(&Job{
								Priority: 110,
								Type:     jobRequiredArgFields,
								Object:   fuzzed,
							})
						}

						push(&Job{
							Priority: 50,
							Type:     jobArgType,
							Object:   fuzzed,
						})
					} else {
						fmt.Printf("[%s] Found: %s.%s %s\n", currentJob.Type, objPath(obj, ""), fuzzed.Name, r.Type)

						obj.AddField(fuzzed)
						obj.SetValue(nil)

						if isKnownScalar(r.Type) {
							continue
						}

						if isResolving(rootName) {
							deferResolve[rootName] = append(deferResolve[rootName], func(i interface{}) {
								shallowMergeUpdate(fuzzed, i.(*graphql.Object))
							})
							continue
						}

						o, resolved := objCache[rootName]
						if resolved {
							objKind := fuzzed.Type.RootKind()
							resolvedKind := o.Type.RootKind()
							if resolvedKind != objKind && objKind == graphql.KindEnum {
								fmt.Println("blah")
							}

							shallowMergeUpdate(fuzzed, o)
							continue
						}

						if isKnownEnum(r.Type) {
							continue
						}

						push(&Job{
							Priority: 110,
							Type:     jobRequiredArgFields,
							Object:   fuzzed,
						})
						push(&Job{
							Priority: 50,
							Type:     jobArgFieldType,
							Object:   fuzzed,
						})
					}
				}
			}

			if currentJob.isAnyType(jobField, jobArgField, jobEnum) {
				resolveStack.Pop()

				rootName := currentJob.Object.Type.RootName()
				defered := deferResolve[rootName]
				for _, fn := range defered {
					fn(currentJob.Object)
				}

				delete(deferResolve, rootName)

				if currentJob.Type == jobField && currentJob.Object.Parent != nil && currentJob.Object.Parent.Type.RootKind() != graphql.KindInterface {
					push(&Job{
						Priority: 55,
						Type:     jobArg,
						Object:   currentJob.Object,
					})
					push(&Job{
						Priority: 120,
						Type:     jobRequiredArgs,
						Object:   currentJob.Object,
					})
				}

				// If there were no fields found, it could be the type isn't an object
				// Fuzz for enum values
				if currentJob.Type == jobArgField && !hadResults {
					push(&Job{
						Priority: 10,
						Type:     jobEnum,
						Object:   currentJob.Object,
						Previous: currentJob,
					})
				}

				// If no results on an enum fuzz and the previous job was a field fuzz
				// that means we couldn't find any fields or enum values. Maybe safe to say
				// that the object is a Scalar
				if currentJob.Type == jobEnum && !hadResults && currentJob.Previous != nil && currentJob.Previous.Type == jobArgField {
					currentJob.Object.Type.SetRootKind(graphql.KindScalar)
					knownScalars.Add(currentJob.Object.Type.RootName())
				}

				if rootName != "" {
					updateCache(rootName, currentJob.Object)
				}

				if currentJob.Object.Name == rootQuery.Name {
					for _, f := range rootQuery.Fields {
						f.Template = graphql.QueryTemplate
					}
				}

				if currentJob.Object.Name == rootMutation.Name {
					for _, f := range rootMutation.Fields {
						f.Template = graphql.MutationTemplate
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
		}, objCache)

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
