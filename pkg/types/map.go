package types

import "sync"

type Map[TKey comparable, TValue any] struct {
	_ noCopy
	mu sync.RWMutex
	m  map[TKey]TValue
}

func (m *Map[TKey, TValue]) init() {
	if m.m == nil {
		m.m = make(map[TKey]TValue)
	}
}

func (m *Map[TKey, TValue]) Load(key TKey) (value TValue, ok bool) {
	m.mu.RLock()
	value, ok = m.m[key]
	m.mu.RUnlock()
	return
}

func (m *Map[TKey, TValue]) Store(key TKey, value TValue) {
	m.mu.Lock()
	m.init()
	m.m[key] = value
	m.mu.Unlock()
}

func (m *Map[TKey, TValue]) LoadOrStore(key TKey, value TValue) (actual TValue, loaded bool) {
	m.mu.Lock()
	m.init()
	if existing, ok := m.m[key]; ok {
		m.mu.Unlock()
		return existing, true
	}
	m.m[key] = value
	m.mu.Unlock()
	return value, false
}

func (m *Map[TKey, TValue]) LoadAndDelete(key TKey) (value TValue, loaded bool) {
	m.mu.Lock()
	value, loaded = m.m[key]
	if loaded {
		delete(m.m, key)
	}
	m.mu.Unlock()
	return
}

func (m *Map[TKey, TValue]) Delete(key TKey) {
	m.mu.Lock()
	delete(m.m, key)
	m.mu.Unlock()
}

func (m *Map[TKey, TValue]) Swap(key TKey, value TValue) (previous TValue, loaded bool) {
	m.mu.Lock()
	m.init()
	previous, loaded = m.m[key]
	m.m[key] = value
	m.mu.Unlock()
	return
}

func (m *Map[TKey, TValue]) CompareAndSwap(key TKey, old, new TValue) (swapped bool) {
	m.mu.Lock()
	if existing, ok := m.m[key]; ok && any(existing) == any(old) {
		m.m[key] = new
		swapped = true
	}
	m.mu.Unlock()
	return
}

func (m *Map[TKey, TValue]) CompareAndDelete(key TKey, old TValue) (deleted bool) {
	m.mu.Lock()
	if existing, ok := m.m[key]; ok && any(existing) == any(old) {
		delete(m.m, key)
		deleted = true
	}
	m.mu.Unlock()
	return
}

// Range iteruje po snapshocie — kopiuje pary K/V pod RLock, zwalnia blokadę,
// iteruje po kopii. Callback może bezpiecznie wołać Store/Delete.
func (m *Map[TKey, TValue]) Range(f func(key TKey, value TValue) bool) {
	m.mu.RLock()
	keys := make([]TKey, 0, len(m.m))
	vals := make([]TValue, 0, len(m.m))
	for k, v := range m.m {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	m.mu.RUnlock()
	for i, k := range keys {
		if !f(k, vals[i]) {
			break
		}
	}
}

func (m *Map[TKey, TValue]) Len() int {
	m.mu.RLock()
	n := len(m.m)
	m.mu.RUnlock()
	return n
}

func (m *Map[TKey, TValue]) Keys() []TKey {
	m.mu.RLock()
	keys := make([]TKey, 0, len(m.m))
	for k := range m.m {
		keys = append(keys, k)
	}
	m.mu.RUnlock()
	return keys
}

func (m *Map[TKey, TValue]) Values() []TValue {
	m.mu.RLock()
	vals := make([]TValue, 0, len(m.m))
	for _, v := range m.m {
		vals = append(vals, v)
	}
	m.mu.RUnlock()
	return vals
}

func (m *Map[TKey, TValue]) Clear() {
	m.mu.Lock()
	clear(m.m)
	m.mu.Unlock()
}
