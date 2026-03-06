package types

import (
	"encoding/json"
	"maps"
	"slices"
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

// Add adds the provided keys to the set.
func (s *Set[KType]) Add(keys ...KType) bool {
	if len(keys) == 0 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		s.cache[key] = NULL // idempotent; skip existence check
	}
	return true
}

// Delete removes the provided keys from the set.
func (s *Set[KType]) Delete(keys ...KType) bool {
	if len(keys) == 0 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		delete(s.cache, key)
	}
	return true
}

// Clear removes all items from the set, reusing the underlying map memory.
func (s *Set[KType]) Clear() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.cache)
	return true
}

// Has checks if the set contains the provided key.
func (s *Set[KType]) Has(key KType) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.cache[key]
	return exists
}

// Len returns the number of items in the set.
func (s *Set[KType]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.cache)
}

// All returns a copy of the set's internal map.
func (s *Set[KType]) All() map[KType]Void {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return maps.Clone(s.cache)
}

// Keys returns a slice containing all keys in the set.
func (s *Set[KType]) Keys() []KType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.keys()
}

// keys returns a slice of all keys without locking (caller must hold lock).
func (s *Set[KType]) keys() []KType {
	return slices.Collect(maps.Keys(s.cache))
}

// populate replaces the set contents from a slice of keys (caller must hold write lock).
func (s *Set[KType]) populate(keys []KType) {
	s.cache = make(map[KType]Void, len(keys))
	for _, key := range keys {
		s.cache[key] = NULL
	}
}

// MarshalJSON implements the json.Marshaler interface.
func (s *Set[KType]) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return json.Marshal(s.keys())
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (s *Set[KType]) UnmarshalJSON(data []byte) error {
	var keys []KType
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.populate(keys)
	return nil
}

// MarshalMsgpack implements the msgpack.Marshaler interface.
func (s *Set[KType]) MarshalMsgpack() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return msgpack.Marshal(s.keys())
}

// UnmarshalMsgpack implements the msgpack.Unmarshaler interface.
func (s *Set[KType]) UnmarshalMsgpack(data []byte) error {
	var keys []KType
	if err := msgpack.Unmarshal(data, &keys); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.populate(keys)
	return nil
}
