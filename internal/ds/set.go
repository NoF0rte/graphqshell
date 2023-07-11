package ds

import (
	"sync"

	"github.com/emirpasic/gods/sets/hashset"
)

type ThreadSafeSet struct {
	mu  *sync.Mutex
	set *hashset.Set
}

func (s *ThreadSafeSet) Add(items ...interface{}) {
	s.mu.Lock()
	s.set.Add(items...)
	s.mu.Unlock()
}

func (s *ThreadSafeSet) Contains(items ...interface{}) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set.Contains(items...)
}

func (s *ThreadSafeSet) StringValues() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var values []string
	for _, v := range s.set.Values() {
		values = append(values, v.(string))
	}
	return values
}

func (s *ThreadSafeSet) Values() []interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set.Values()
}

func NewThreadSafeSet(values ...interface{}) *ThreadSafeSet {
	return &ThreadSafeSet{
		mu:  &sync.Mutex{},
		set: hashset.New(values...),
	}
}
