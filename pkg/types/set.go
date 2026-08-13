package types

import (
	"encoding/json"
	"maps"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
)

type Set[KType comparable] struct {
	mu    sync.RWMutex
	cache map[KType]Void
}

// NewSet creates a new Set and initializes it with the provided keys.
func NewSet[KType comparable](keys ...KType) *Set[KType] {
	s := &Set[KType]{cache: make(map[KType]Void, len(keys))}
	for _, key := range keys {
		s.cache[key] = NULL
	}
	return s
}

// Add adds the provided keys and reports whether the set changed.
func (s *Set[KType]) Add(keys ...KType) bool {
	if len(keys) == 0 {
		return false
	}

	s.mu.Lock()
	if s.cache == nil {
		s.cache = make(map[KType]Void, len(keys))
	}
	length := len(s.cache)
	for _, key := range keys {
		s.cache[key] = NULL
	}
	changed := len(s.cache) != length
	s.mu.Unlock()
	return changed
}

// Delete removes the provided keys and reports whether the set changed.
func (s *Set[KType]) Delete(keys ...KType) bool {
	if len(keys) == 0 {
		return false
	}

	s.mu.Lock()
	length := len(s.cache)
	for _, key := range keys {
		delete(s.cache, key)
	}
	changed := len(s.cache) != length
	s.mu.Unlock()
	return changed
}

// Clear removes all items from the set, reusing the underlying map memory.
func (s *Set[KType]) Clear() bool {
	s.mu.Lock()
	changed := len(s.cache) > 0
	clear(s.cache)
	s.mu.Unlock()
	return changed
}

// Has checks if the set contains the provided key.
func (s *Set[KType]) Has(key KType) bool {
	s.mu.RLock()
	_, exists := s.cache[key]
	s.mu.RUnlock()
	return exists
}

// Len returns the number of items in the set.
func (s *Set[KType]) Len() int {
	s.mu.RLock()
	length := len(s.cache)
	s.mu.RUnlock()
	return length
}

// All returns a copy of the set's internal map.
func (s *Set[KType]) All() map[KType]Void {
	s.mu.RLock()
	cache := maps.Clone(s.cache)
	s.mu.RUnlock()
	return cache
}

// Keys returns a slice containing all keys in the set.
func (s *Set[KType]) Keys() []KType {
	s.mu.RLock()
	if len(s.cache) == 0 {
		s.mu.RUnlock()
		return nil
	}
	keys := make([]KType, len(s.cache))
	i := 0
	for key := range s.cache {
		keys[i] = key
		i++
	}
	s.mu.RUnlock()
	return keys
}

// populate replaces the set contents from a slice of keys.
func (s *Set[KType]) populate(keys []KType) {
	cache := make(map[KType]Void, len(keys))
	for _, key := range keys {
		cache[key] = NULL
	}

	s.mu.Lock()
	s.cache = cache
	s.mu.Unlock()
}

// MarshalJSON implements the json.Marshaler interface.
func (s *Set[KType]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Keys())
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (s *Set[KType]) UnmarshalJSON(data []byte) error {
	var keys []KType
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	s.populate(keys)
	return nil
}

// MarshalMsgpack implements the msgpack.Marshaler interface.
func (s *Set[KType]) MarshalMsgpack() ([]byte, error) {
	return msgpack.Marshal(s.Keys())
}

// UnmarshalMsgpack implements the msgpack.Unmarshaler interface.
func (s *Set[KType]) UnmarshalMsgpack(data []byte) error {
	var keys []KType
	if err := msgpack.Unmarshal(data, &keys); err != nil {
		return err
	}

	s.populate(keys)
	return nil
}
