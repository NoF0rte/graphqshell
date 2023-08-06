package fuzz

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/NoF0rte/graphqshell/internal/ds"
	"github.com/NoF0rte/graphqshell/internal/graphql"
	"github.com/emirpasic/gods/queues/priorityqueue"
	"github.com/emirpasic/gods/stacks/arraystack"
	"github.com/emirpasic/gods/utils"
)

type Location string

const (
	locField     Location = "FIELD"
	locArg       Location = "ARG"
	locArgField  Location = "ARG_FIELD"
	locEnum      Location = "ENUM"
	locInterface Location = "INTERFACE"
)

type direction int

const (
	directionRoot direction = iota
	directionCaller
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

func (j *Job) Runner(client *graphql.Client, threads int, words []string) Runner {
	rootName := j.Object.Type.RootName()
	cached, isCached := getCached(rootName)
	if isCached && j.isAnyType(jobArgField, jobField, jobEnum) {
		updateFields(j.Object, cached)
		j.Object.Args = []*graphql.Object{}

		resolveStack.Push(rootName)
		return &NopRunner{}
	}

	switch j.Type {
	case jobField:
		resolveStack.Push(rootName)
		return &FieldRunner{
			Client:   client,
			Location: locField,
			Threads:  threads,
			Words:    words,
		}
	case jobFieldType:
		return &FieldTypeRunner{
			Client: client,
		}
	case jobArg:
		return &ArgRunner{
			Client:  client,
			Words:   words,
			Threads: threads,
		}
	case jobArgType:
		return &ArgTypeRunner{
			Client:   client,
			Location: locArg,
		}
	case jobArgField:
		resolveStack.Push(rootName)
		return &FieldRunner{
			Client:   client,
			Location: locArgField,
			Words:    words,
			Threads:  threads,
		}
	case jobArgFieldType:
		return &ArgTypeRunner{
			Client:   client,
			Location: locArgField,
		}
	case jobRequiredArgs:
		return &RequiredInputRunner{
			Client:   client,
			Location: locArg,
		}
	case jobRequiredArgFields:
		return &RequiredInputRunner{
			Client:   client,
			Location: locArgField,
		}
	case jobEnum:
		resolveStack.Push(rootName)
		if j.Object.Parent != nil {
			return &EnumRunner{
				Client:   client,
				Location: locArgField,
				Words:    words,
				Threads:  threads,
			}
		}

		return &EnumRunner{
			Client:   client,
			Location: locArg,
			Words:    words,
			Threads:  threads,
		}
	default:
		return nil
	}
}

type Results struct {
	Query      *graphql.Object
	Mutation   *graphql.Object
	FoundWords []string
	New        bool
	TypeMap    map[string]*graphql.Object
}

type FuzzOptions struct {
	targets []string
	force   bool
	schema  map[string]*graphql.Object
	threads int
}

type FuzzOption func(o *FuzzOptions)

var (
	logger       *Logger = &Logger{}
	currentJob   *Job
	jobQueue     *priorityqueue.Queue           = priorityqueue.NewWith(byPriority)
	fuzzMutex    *sync.Mutex                    = &sync.Mutex{}
	deferResolve map[string][]func(interface{}) = make(map[string][]func(interface{}))
	resolveStack *arraystack.Stack              = arraystack.New()
	typeMap      map[string]*graphql.Object     = make(map[string]*graphql.Object)

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

	nonEnumValueRe = regexp.MustCompile(`Enum "[^"]+" cannot represent non-enum value: (.*)\.`)
	didYouMeanRe   = regexp.MustCompile(`Did you mean(?: the enum value| to use an inline fragment on)? (.*)\?$`)
	graphqlRe      = regexp.MustCompile(graphqlNameRe)
	orRe           = regexp.MustCompile(" or ")
	nonScalarRe    = regexp.MustCompile(`(non-?string)|(non-?integer)|(must be a string)`)
	enumRe         = regexp.MustCompile(`\b[Ee]nums?\b`)

	queryFieldRe = func(name string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`Cannot query field "%s" on type "(%s)"`, regexp.QuoteMeta(name), graphqlNameRe))
	}
	noSubfieldsRe = func(name string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`Field "%s".*"([^"]+)" has no subfields`, regexp.QuoteMeta(name)))
	}
	fieldOfTypeRe = func(name string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`Field \"%s\" of type \"([^"]+)\"`, regexp.QuoteMeta(name)))
	}
	requiredArgRe = func(name string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`Field "%s" argument "(%s)" of type "([^"]+)" is required`, regexp.QuoteMeta(name), graphqlNameRe))
	}
	requiredArgFieldRe = func(t string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`Field "?%s\.(%s)"? of required type "?(.*?)"? was not provided`, regexp.QuoteMeta(t), graphqlNameRe))
	}
	fieldNotDefinedRe = func(name string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`Field "%s" is not defined by %s`, regexp.QuoteMeta(name), graphqlNameRe))
	}
	expectedTypeRe = func(name string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf("Expected type ([^,]+), found %s", regexp.QuoteMeta(name)))
	}
	expectingTypeRe = func(variable *graphql.Variable) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`Variable "\$%s" of type "%s" used in position expecting type "([^"]+)"`, regexp.QuoteMeta(variable.Name), regexp.QuoteMeta(variable.Type.String())))
	}
	enumNotExistsRe = func(name string, t string) *regexp.Regexp {
		if t == "" {
			regexp.MustCompile(fmt.Sprintf(`Value "%s" does not exist in "[^"]+" enum`, regexp.QuoteMeta(name)))
		}
		return regexp.MustCompile(fmt.Sprintf(`Value "%s" does not exist in "%s" enum`, regexp.QuoteMeta(name), regexp.QuoteMeta(t)))
	}
)

const (
	batchFieldSize int = 64

	graphqlNameRe string = "[_A-Za-z][_0-9A-Za-z]*"
)

func isRoot(o *graphql.Object) bool {
	return (o.Parent == rootQuery || o.Parent == rootMutation || o.Parent == nil) && o.Caller == nil
}

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

func getCached(t string) (*graphql.Object, bool) {
	cached, ok := typeMap[t]
	return cached, ok
}

func tryGetCached(o *graphql.Object) *graphql.Object {
	cached := typeMap[o.Type.RootName()]
	if cached != nil {
		return cached
	}
	return o
}

func updateFields(o1 *graphql.Object, o2 *graphql.Object) {
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
		Args:     o1.Args,
	}
	updateFields(merged, o2)
	return merged
}

func copyFromCache(o *graphql.Object) *graphql.Object {
	cached := tryGetCached(o)
	return shallowMergeNew(o, cached)
}

func shouldIgnoreField(name string) bool {
	for _, f := range ignoreFields {
		if f == name {
			return true
		}
	}

	return false
}

func toChan(items []string) chan string {
	c := make(chan string)
	go func() {
		for _, item := range items {
			c <- item
		}
		close(c)
	}()
	return c
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

func byPriority(a, b interface{}) int {
	priorityA := a.(*Job).Priority
	priorityB := b.(*Job).Priority
	return -utils.IntComparator(priorityA, priorityB) // "-" descending order
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

func combineWords(words []string) []string {
	set := ds.NewThreadSafeSet()
	for _, line := range words {
		set.Add(line)
	}
	for _, found := range foundWordsSet.Values() {
		set.Add(found)
	}
	return set.StringValues()
}

func addFoundWord(word string) {
	foundWordsSet.Add(word)
	if len(word) > 1 {
		foundWordsSet.Add(word[:len(word)-1])
	}
}

func pushArgJobs(o *graphql.Object) {
	push(&Job{
		Priority: 55,
		Type:     jobArg,
		Object:   o,
	})
	push(&Job{
		Priority: 120,
		Type:     jobRequiredArgs,
		Object:   o,
	})
}

func populate(schema map[string]*graphql.Object) {
	if schema == nil {
		schema = make(map[string]*graphql.Object)
	}

	typeMap = schema

	for key, value := range schema {
		if strings.EqualFold(key, "query") {
			rootQuery = value
		} else if strings.EqualFold(key, "mutation") {
			rootMutation = value
		}

		if rootQuery != nil && rootMutation != nil {
			break
		}
	}

	if rootQuery == nil {
		rootQuery = &graphql.Object{
			Name:     "Query",
			Template: "query {{.Body}}",
		}
	}

	if rootMutation == nil {
		rootMutation = &graphql.Object{
			Name:     "Mutation",
			Template: "mutation {{.Body}}",
		}
	}
}

func WithTargets(targets []string) FuzzOption {
	return func(o *FuzzOptions) {
		o.targets = targets
	}
}

func WithForce() FuzzOption {
	return func(o *FuzzOptions) {
		o.force = true
	}
}

func WithSchema(schema map[string]*graphql.Object) FuzzOption {
	return func(o *FuzzOptions) {
		o.schema = schema
	}
}

func WithThreads(threads int) FuzzOption {
	return func(o *FuzzOptions) {
		o.threads = threads
	}
}

func Start(client *graphql.Client, words []string, opts ...FuzzOption) (*Results, error) {
	options := &FuzzOptions{}
	for _, o := range opts {
		o(options)
	}

	populate(options.schema)

	for _, target := range options.targets {
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

	logger = &Logger{}

	var ok bool
	for {
		currentJob, ok = pop()
		if !ok || currentJob == nil {
			break
		}
		logger.job = currentJob

		hadResults := false
		runner := currentJob.Runner(client, options.threads, combineWords(words))
		for result := range runner.Run(currentJob.Object) {
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

						logger.Found(fuzzed)
						// fmt.Printf("[%s] Found: %s(%s)\n", currentJob.Type, objPath(obj, ""), fuzzed.Name)

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

						logger.Enum(obj, fuzzed)
						//fmt.Printf("[%s] Found: %s => %s\n", currentJob.Type, objPath(obj, ""), fuzzed.Name)
					}
				case locInterface:
					if obj.Type.RootKind() != graphql.KindInterface {
						obj.Type.SetRootKind(graphql.KindInterface)
					}

					// check the obj cache for the type

					if obj.AddPossibleValue(fuzzed) {
						fuzzed.Type = *graphql.TypeRefFromString(fuzzed.Name, graphql.KindObject)

						o, cached := getCached(fuzzed.Name)
						if cached {
							objKind := obj.Type.RootKind()
							resolvedKind := o.Type.RootKind()
							if resolvedKind != objKind && objKind == graphql.KindEnum {
								fmt.Println("blah")
							}

							updateFields(obj, o)
							continue
						}

						if isResolving(fuzzed.Name) || isQueued(jobField, fuzzed.Name) {
							deferResolve[fuzzed.Name] = append(deferResolve[fuzzed.Name], func(i interface{}) {
								updateFields(obj, i.(*graphql.Object))
							})
							continue
						}

						push(&Job{
							Priority: currentJob.Priority,
							Type:     jobField,
							Object:   fuzzed,
						})
					}
				default:
					if obj.AddField(fuzzed) {
						logger.Found(fuzzed)
						//fmt.Printf("[%s] Found: %s.%s\n", currentJob.Type, objPath(obj, ""), fuzzed.Name)

						if r.Location == locField {
							push(&Job{
								Priority: 100,
								Type:     jobFieldType,
								Object:   fuzzed,
							})
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
					logger.FoundWithType(obj, r.Type)
					//fmt.Printf("[%s] Found: %s %s\n", currentJob.Type, objPath(obj, ""), r.Type)

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

					o, cached := getCached(rootName)
					if cached {
						objKind := obj.Type.RootKind()
						resolvedKind := o.Type.RootKind()
						if resolvedKind != objKind && objKind == graphql.KindEnum {
							fmt.Println("blah")
						}

						updateFields(obj, o)
						if r.Location == locField && r.Kind != graphql.KindEnum {
							pushArgJobs(obj)
						}
						continue
					}

					// If we know the type is resolving, the job is queued,
					// or that it is an enum but on a normal field, defer the resolution
					if (r.Kind == graphql.KindEnum && r.Location == locField) || isResolving(rootName) || isQueued(nextJob.Type, rootName) {
						deferResolve[rootName] = append(deferResolve[rootName], func(i interface{}) {
							updateFields(obj, i.(*graphql.Object))

							// Even though we know the fields we still need to find the arguments
							if r.Location == locField && r.Kind != graphql.KindEnum {
								pushArgJobs(obj)
							}
						})
						continue
					}

					if r.Kind == graphql.KindScalar {
						continue
					}
				}

				push(nextJob)
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
					logger.FoundWithType(fuzzed, r.Type)
					//fmt.Printf("[%s] Found: %s(%s %s)\n", currentJob.Type, objPath(obj, ""), fuzzed.Name, r.Type)

					fuzzed.Parent = nil
					fuzzed.Caller = obj

					obj.AddArg(fuzzed)

					if isKnownScalar(r.Type) {
						continue
					}

					if !isKnownEnum(r.Type) {
						o, cached := getCached(rootName)
						if cached {
							objKind := fuzzed.Type.RootKind()
							resolvedKind := o.Type.RootKind()
							if resolvedKind != objKind && objKind == graphql.KindEnum {
								fmt.Println("blah")
							}

							updateFields(fuzzed, o)
							continue
						}

						if isResolving(rootName) {
							deferResolve[rootName] = append(deferResolve[rootName], func(i interface{}) {
								updateFields(obj, i.(*graphql.Object))
							})
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
					logger.FoundWithType(fuzzed, r.Type)
					//fmt.Printf("[%s] Found: %s.%s %s\n", currentJob.Type, objPath(obj, ""), fuzzed.Name, r.Type)

					obj.AddField(fuzzed)
					obj.SetValue(nil)

					if isKnownScalar(r.Type) {
						continue
					}

					o, resolved := getCached(rootName)
					if resolved {
						objKind := fuzzed.Type.RootKind()
						resolvedKind := o.Type.RootKind()
						if resolvedKind != objKind && objKind == graphql.KindEnum {
							fmt.Println("blah")
						}

						updateFields(fuzzed, o)
						continue
					}

					if isResolving(rootName) {
						deferResolve[rootName] = append(deferResolve[rootName], func(i interface{}) {
							updateFields(fuzzed, i.(*graphql.Object))
						})
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
				pushArgJobs(currentJob.Object)
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
				_, found := getCached(rootName)
				if !found {
					typeMap[rootName] = currentJob.Object
				} else {
					fmt.Println("[!] Obj already cached!")
				}
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

	// Add root query & mutation to type map to help populate method
	typeMap["Query"] = rootQuery
	typeMap["Mutation"] = rootMutation

	return &Results{
		Query: &graphql.Object{
			Name:   rootQuery.Name,
			Fields: rootQuery.Fields,
		},
		Mutation: &graphql.Object{
			Name:   rootMutation.Name,
			Fields: rootMutation.Fields,
		},
		TypeMap:    typeMap,
		FoundWords: foundWordsSet.StringValues(),
		New:        foundWordsSet.Size() > 0,
	}, nil
}
