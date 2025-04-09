package utils

import (
	"sync"
)

type ParameterBag struct {
	mu         sync.RWMutex
	parameters map[string][]string
}

func NewParameterBag(parameters map[string][]string) *ParameterBag {
	if parameters == nil {
		parameters = make(map[string][]string)
	}
	return &ParameterBag{parameters: parameters}
}

// Returns the parameters.
func (p *ParameterBag) All() map[string][]string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	_tmp := make(map[string][]string, len(p.parameters))
	for k, v := range p.parameters {
		_tmp[k] = append([]string(nil), v...)
	}

	return _tmp
}

// Returns the parameter keys.
func (p *ParameterBag) Keys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	keys := make([]string, 0, len(p.parameters))
	for k := range p.parameters {
		keys = append(keys, k)
	}
	return keys
}

// Replaces the current parameters by a new set.
func (p *ParameterBag) Replace(parameters map[string][]string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.parameters = parameters
}

// Adds or replaces the current parameters with a new set.
func (p *ParameterBag) With(parameters map[string][]string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, v := range parameters {
		p.parameters[k] = append([]string(nil), v...)
	}
}

// Add adds the value to key. It appends to any existing values associated with key.
func (p *ParameterBag) Add(key string, value string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.parameters[key] = append(p.parameters[key], value)
}

// Returns a parameter by name, defaulting to the last value if present.
func (p *ParameterBag) Get(key string, _default ...string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if value, ok := p.parameters[key]; ok && len(value) > 0 {
		return value[len(value)-1], true
	}
	if len(_default) > 0 {
		return _default[0], false
	}
	return "", false
}

func (p *ParameterBag) Peek(key string, _default ...string) string {
	v, _ := p.Get(key, _default...)
	return v
}

// Returns the first value of a parameter by name.
func (p *ParameterBag) GetFirst(key string, _default ...string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if value, ok := p.parameters[key]; ok && len(value) > 0 {
		return value[0], true
	}
	if len(_default) > 0 {
		return _default[0], false
	}
	return "", false
}

// Returns the last value of a parameter by name.
func (p *ParameterBag) GetLast(key string, _default ...string) (string, bool) {
	return p.Get(key, _default...)
}

// Returns all values of a parameter by name.
func (p *ParameterBag) Gets(key string, _default ...[]string) ([]string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if v, ok := p.parameters[key]; ok {
		return append([]string(nil), v...), true
	}
	if len(_default) > 0 {
		return _default[0], false
	}
	return []string{}, false
}

// Sets a parameter by name.
func (p *ParameterBag) Set(key string, value string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.parameters[key] = []string{value}
}

// Returns true if the parameter is defined.
func (p *ParameterBag) Has(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	_, ok := p.parameters[key]
	return ok
}

// Removes a parameter.
func (p *ParameterBag) Remove(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.parameters, key)
}

// Returns the number of parameters.
func (p *ParameterBag) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.parameters)
}
