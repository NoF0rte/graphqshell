package fuzz

import (
	"fmt"
	"strings"
	"sync"

	"github.com/NoF0rte/graphqshell/internal/graphql"
)

type Runner interface {
	Run(o *graphql.Object) chan Result
}

type FieldRunner struct {
	Client   *graphql.Client
	Location Location
	Words    []string
	Threads  int

	results chan Result
	wg      sync.WaitGroup
	target  *graphql.Object
}

func (r *FieldRunner) Run(o *graphql.Object) chan Result {
	r.results = make(chan Result)
	r.target = o

	go func() {
		words := toChan(r.Words)
		for i := 0; i < r.Threads; i++ {
			r.wg.Add(1)

			obj := copyFromCache(o)
			if len(obj.Args) > 0 {
				fmt.Println("[!] Copied cache had args")
			}
			go r.worker(obj, words, 1)
		}

		r.wg.Wait()
		close(r.results)
	}()

	return r.results
}

func (r *FieldRunner) batchFields(createField func(word string) *graphql.Object, words chan string) []*graphql.Object {
	count := 0
	var fields []*graphql.Object
	for word := range words {
		if count == batchFieldSize {
			break
		}

		fields = append(fields, createField(word))
		count++
	}
	return fields
}

func (r *FieldRunner) worker(obj *graphql.Object, words chan string, callCount int) {
	defer func() {
		if callCount == 1 {
			r.wg.Done()
		}
	}()

	// obj := copyFromCache(o)
	// if len(obj.Args) > 0 {
	// 	fmt.Println("[!] Copied cache had args")
	// }

	matcher := queryFieldRe
	createField := func(word string) *graphql.Object {
		return &graphql.Object{
			Name: word,
		}
	}

	if r.Location == locArgField {
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

		matcher = fieldNotDefinedRe
	}

	// words := toChan(r.Words)
	rootObj := walkObject(obj, directionRoot)

	var retryFields []string
	for {
		fields := r.batchFields(createField, words)
		if len(fields) == 0 {
			break
		}

		obj.Fields = fields

		resp, err := r.Client.PostJSON(rootObj)
		if err != nil {
			fmt.Printf("[!] Error posting: %v\n", err)
			continue
		}

		if resp.RawResponse.StatusCode >= 500 {
			fmt.Printf("[!] Server error: %s\n", resp.RawResponse.Status)
			continue
		}

		for _, f := range fields {
			word := f.Name

			handled, found, shouldContinue := r.process(matcher(word), word, resp)
			if !shouldContinue {
				return
			}

			// Can happen when the word is an exact match
			if !handled && !found {
				// If the word is 1 letter, assume it is an exact match for now
				if len(word) == 1 {
					r.results <- r.makeResult(word, r.Location)
					continue
				}

				retryFields = append(retryFields, word[:len(word)-1])

				// name := word[:len(word)-1]
				// obj.Fields = []*graphql.Object{
				// 	createField(name),
				// }

				// resp2, err := r.Client.PostJSON(rootObj)
				// if err != nil {
				// 	fmt.Printf("[!] Error posting: %v\n", err)
				// 	continue
				// }

				// _, _, shouldContinue = r.process(matcher(name), name, resp2)
				// if !shouldContinue {
				// 	break
				// }
			}
		}
	}

	if len(retryFields) > 0 {
		r.worker(obj, toChan(retryFields), callCount+1)
	}
}

func (r *FieldRunner) process(matchers Regexes, word string, resp *graphql.Response) (handled bool, found bool, shouldContinue bool) {
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

		if !matchers.MatchString(e.Message) {
			continue
		}

		handled = true

		didYouMeanMatches := didYouMeanRe.FindStringSubmatch(e.Message)
		if len(didYouMeanMatches) == 0 {
			continue
		}

		loc := r.Location
		if strings.Contains(e.Message, "inline fragment") {
			if loc != locField {
				fmt.Println("[!] Found inline fragment on non object field")
			}

			loc = locInterface
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

			r.results <- r.makeResult(field, loc)
		}

		break
	}
	return
}

func (r *FieldRunner) makeResult(text string, loc Location) *FuzzResult {
	return &FuzzResult{
		Text:     text,
		Location: loc,
		obj:      r.target,
	}
}

type EnumRunner struct {
	Client   *graphql.Client
	Location Location
	Words    []string
	Threads  int

	results chan Result
	wg      sync.WaitGroup
	target  *graphql.Object
}

func (r *EnumRunner) Run(o *graphql.Object) chan Result {
	r.results = make(chan Result)
	r.target = o

	go func() {
		words := toChan(r.Words)
		for i := 0; i < r.Threads; i++ {
			r.wg.Add(1)

			obj := copyFromCache(o)
			if len(obj.Args) > 0 {
				fmt.Println("[!] Copied cache had args")
			}
			go r.worker(obj, words, 1)
		}

		r.wg.Wait()
		close(r.results)
	}()

	return r.results
}

func (r *EnumRunner) worker(obj *graphql.Object, words chan string, callCount int) {
	defer func() {
		if callCount == 1 {
			r.wg.Done()
		}
	}()

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
	if r.Location == locArgField {
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

	// words := toChan(r.Words)

	var retryValues []string
	rootName := obj.Type.RootName()
	for word := range words {
		obj.SetValue(word)

		resp, err := r.Client.PostJSON(rootObj)
		if err != nil {
			fmt.Printf("[!] Error posting: %v\n", err)
			continue
		}

		handled, found := r.process(rootName, word, resp)

		// Can happen when the word is an exact match
		if !handled && !found {
			if len(word) == 1 {
				r.results <- r.makeResult(word)
				continue
			}

			retryValues = append(retryValues, word[:len(word)-1])

			// value := word[:len(word)-1]
			// obj.SetValue(value)

			// resp2, err := r.Client.PostJSON(rootObj)
			// if err != nil {
			// 	fmt.Printf("[!] Error posting: %v\n", err)
			// 	continue
			// }
			// process(value, resp2)
		}

		if len(retryValues) > 0 {
			r.worker(obj, toChan(retryValues), callCount+1)
		}
	}
}

func (r *EnumRunner) process(rootName string, word string, resp *graphql.Response) (handled bool, found bool) {
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
			r.results <- r.makeResult(match)
		}
	}
	return
}

func (r *EnumRunner) makeResult(text string) *FuzzResult {
	return &FuzzResult{
		Text:     text,
		Location: locEnum,
		obj:      r.target,
	}
}

type ArgRunner struct {
	Client  *graphql.Client
	Words   []string
	Threads int

	results chan Result
	wg      sync.WaitGroup
	target  *graphql.Object
}

func (r *ArgRunner) Run(o *graphql.Object) chan Result {
	r.results = make(chan Result)
	r.target = o

	go func() {
		words := toChan(r.Words)
		for i := 0; i < r.Threads; i++ {
			r.wg.Add(1)

			obj := copyFromCache(o)
			obj.Fields = []*graphql.Object{
				{Name: "graphqlshell_field"},
			}

			go r.worker(obj, words, 1)
		}

		r.wg.Wait()
		close(r.results)
	}()

	return r.results
}

func (r *ArgRunner) batchArgs(words chan string) []*graphql.Object {
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
	return args
}

func (r *ArgRunner) worker(obj *graphql.Object, words chan string, callCount int) {
	defer func() {
		if callCount == 1 {
			r.wg.Done()
		}
	}()

	// words := toChan(r.Words)

	var retryArgs []string
	for {
		args := r.batchArgs(words)
		if len(args) == 0 {
			break
		}

		rootObj := walkObject(obj, directionRoot)

		obj.Args = args

		resp, err := r.Client.PostJSON(rootObj)
		if err != nil {
			fmt.Printf("[!] Error posting: %v\n", err)
			continue
		}

		// TODO: Make more efficient by not doing 2 loops
		for _, a := range args {
			word := a.Name

			handled, found := r.process(a.Name, resp)

			// Can happen when the word is an exact match
			// Let's do some checks to make sure it isn't a false positive
			if !handled && !found {
				if len(word) == 1 {
					r.results <- r.makeResult(word)
					continue
				}

				retryArgs = append(retryArgs, word[:len(word)-1])
				// obj.Args = []*graphql.Object{
				// 	{
				// 		Name: name,
				// 	},
				// }

				// resp2, err := client.PostJSON(rootObj)
				// if err != nil {
				// 	fmt.Printf("[!] Error posting: %v\n", err)
				// 	continue
				// }

				// // This should make it so if it was an exact match before
				// // we should get the "did you mean" error
				// process(name, resp2)
			}
		}
	}

	if len(retryArgs) > 0 {
		r.worker(obj, toChan(retryArgs), callCount+1)
	}
}

func (r *ArgRunner) process(word string, resp *graphql.Response) (handled bool, found bool) {
	matcher := unknownArgRe(word)
	for _, e := range resp.Result.Errors {
		if !matcher.MatchString(e.Message) {

			// Exact match
			if exactArgMatchRe(word).MatchString(e.Message) {
				found = true
				handled = true
				r.results <- r.makeResult(word)
				break
			}

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
			r.results <- r.makeResult(arg)
		}

		// Might want to break after we get here since there shouldn't be
		// any more matches for this word
	}

	return
}

func (r *ArgRunner) makeResult(text string) *FuzzResult {
	return &FuzzResult{
		Text:     text,
		Location: locArg,
		obj:      r.target,
	}
}

type FieldTypeRunner struct {
	Client *graphql.Client
}

func (r *FieldTypeRunner) Run(o *graphql.Object) chan Result {
	c := make(chan Result)

	obj := copyFromCache(o)
	if len(obj.Args) > 0 {
		fmt.Println("[!] Copied cache had args")
	}
	go func() {
		defer close(c)

		// Having issues with the args being copied from cache when they shouldn't be
		// Don't know whether to copy the args or not
		rootObj := walkObject(obj, directionRoot)

		name := "graphqlshell_field"
		obj.Fields = append(obj.Fields, &graphql.Object{
			Name: name,
		})

		resp, err := r.Client.PostJSON(rootObj)
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
			matches := queryFieldRe(name).FindAllStringSubmatch(e.Message)
			if len(matches) == 0 {
				matches = noSubfieldsRe(obj.Name).FindAllStringSubmatch(e.Message)

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

			resp, err = r.Client.PostJSON(rootObj)
			if err != nil {
				fmt.Printf("[!] Error posting: %v\n", err)
				return
			}

			for _, e := range resp.Result.Errors {
				matches := fieldOfTypeRe(obj.Name).FindAllStringSubmatch(e.Message)
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

type ArgTypeRunner struct {
	Client   *graphql.Client
	Location Location
}

func (r *ArgTypeRunner) Run(o *graphql.Object) chan Result {
	c := make(chan Result)

	obj := copyFromCache(o)
	if len(obj.Args) > 0 {
		fmt.Println("[!] Copied cache had args")
	}
	go func() {
		defer close(c)

		result := &TypeResult{
			Location: r.Location,
			Kind:     graphql.KindObject,
			obj:      o,
		}

		caller := walkObject(obj, directionCaller)
		caller = getMinFields(caller)
		if caller == nil {
			newPriority := currentJob.Priority - 10
			path := logger.Path(obj)
			// path := objPath(obj, "")
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
		if r.Location == locArgField {
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

			resp, err := r.Client.PostJSON(rootObj, variable)
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

			resp, err := r.Client.PostJSON(rootObj, variable)
			if err != nil {
				fmt.Printf("[!] Error posting: %v\n", err)
				return
			}

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

type RequiredInputRunner struct {
	Client   *graphql.Client
	Location Location

	results chan Result
}

func (r *RequiredInputRunner) Run(o *graphql.Object) chan Result {
	r.results = make(chan Result)

	obj := copyFromCache(o)
	if len(obj.Args) > 0 {
		fmt.Println("[!] Copied cache had args")
	}
	go func() {
		defer close(r.results)

		var rootObj *graphql.Object

		if r.Location == locArg {
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

		resp, err := r.Client.PostJSON(rootObj)
		if err != nil {
			fmt.Printf("[!] Error posting: %v\n", err)
		}

		r.process(obj, resp)
	}()

	return r.results
}

func (r *RequiredInputRunner) process(o *graphql.Object, resp *graphql.Response) {
	for _, e := range resp.Result.Errors {
		matches := requiredArgRe(o.Name).FindStringSubmatch(e.Message)
		if len(matches) > 0 {
			r.results <- &RequiredResult{
				Text:     matches[1],
				Type:     matches[2],
				Location: locArg,
				obj:      o,
			}
			continue
		}

		matches = requiredArgFieldRe(o.Type.RootName()).FindStringSubmatch(e.Message)
		if len(matches) == 0 {
			continue
		}

		r.results <- &RequiredResult{
			Text:     matches[1],
			Type:     matches[2],
			Location: locArgField,
			obj:      o,
		}
	}
}

type NopRunner struct {
}

func (r *NopRunner) Run(o *graphql.Object) chan Result {
	c := make(chan Result)
	go func() {
		close(c)
	}()
	return c
}
