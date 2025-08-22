package types

import (
	"encoding/json"
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
		if _, exists := s.cache[key]; !exists {
			s.cache[key] = NULL
		}
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

// Clear removes all items from the set.
func (s *Set[KType]) Clear() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache = map[KType]Void{}
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

	_tmp := make(map[KType]Void, len(s.cache))
	for k := range s.cache {
		_tmp[k] = NULL
	}

	return _tmp
}

// Keys returns a slice containing all keys in the set.
func (s *Set[KType]) Keys() []KType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]KType, 0, len(s.cache))
	for k := range s.cache {
		list = append(list, k)
	}

	return list
}

// MarshalJSON implements the json.Marshaler interface.
func (s *Set[KType]) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]KType, 0, len(s.cache))

	for key := range s.cache {
		keys = append(keys, key)
	}

	return json.Marshal(keys)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (s *Set[KType]) UnmarshalJSON(data []byte) error {
	var keys []KType

	// Unmarshal the JSON data into a slice of keys
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	// Clear the current set and populate with the new keys
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[KType]Void, len(keys))
	for _, key := range keys {
		s.cache[key] = NULL
	}

	return nil
}

// MarshalMsgpack implements the msgpack.Marshaler interface.
func (s *Set[KType]) MarshalMsgpack() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]KType, 0, len(s.cache))

	for key := range s.cache {
		keys = append(keys, key)
	}

	return msgpack.Marshal(keys)
}

// UnmarshalMsgpack implements the msgpack.Unmarshaler interface.
func (s *Set[KType]) UnmarshalMsgpack(data []byte) error {
	var keys []KType

	// Unmarshal the MessagePack data into a slice of keys
	if err := msgpack.Unmarshal(data, &keys); err != nil {
		return err
	}

	// Clear the current set and populate with the new keys
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[KType]Void, len(keys))
	for _, key := range keys {
		s.cache[key] = NULL
	}

	return nil
}
