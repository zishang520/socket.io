package adapter

import (
	"context"
	"sync"
	"time"

	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/v3/pkg/queue"
)

const classicValkeyPubSubRetryDelay = time.Second

type classicValkeyMessageHandler func(payload []byte, channel string)

type sharedClassicValkeyPubSub struct {
	pubSub *classicValkeyPubSub
	refs   int
}

type classicValkeyPubSubCache struct {
	mu     sync.Mutex
	groups map[*valkey.ValkeyClient]*sharedClassicValkeyPubSub
}

func (c *classicValkeyPubSubCache) acquire(client *valkey.ValkeyClient) *classicValkeyPubSub {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.groups == nil {
		c.groups = make(map[*valkey.ValkeyClient]*sharedClassicValkeyPubSub)
	}
	shared := c.groups[client]
	if shared == nil {
		shared = &sharedClassicValkeyPubSub{
			pubSub: newClassicValkeyPubSub(client.Context(), client.Sub(), func(err error) {
				client.Emit("error", err)
			}),
		}
		c.groups[client] = shared
	}
	shared.refs++
	return shared.pubSub
}

func (c *classicValkeyPubSubCache) release(client *valkey.ValkeyClient, pubSub *classicValkeyPubSub) {
	c.mu.Lock()
	shared := c.groups[client]
	if shared == nil || shared.pubSub != pubSub {
		c.mu.Unlock()
		return
	}
	shared.refs--
	if shared.refs != 0 {
		c.mu.Unlock()
		return
	}
	delete(c.groups, client)
	if len(c.groups) == 0 {
		c.groups = nil
	}
	c.mu.Unlock()
	pubSub.Close()
}

var classicValkeyPubSubs classicValkeyPubSubCache

type classicValkeyPubSubRoute struct {
	first  *classicValkeySubscription
	others map[*classicValkeySubscription]struct{}
}

type classicValkeyPubSubRoutes struct {
	handlers map[string]classicValkeyPubSubRoute
	active   map[string]struct{}
}

func newClassicValkeyPubSubRoutes() classicValkeyPubSubRoutes {
	return classicValkeyPubSubRoutes{
		handlers: make(map[string]classicValkeyPubSubRoute),
		active:   make(map[string]struct{}),
	}
}

// classicValkeyPubSub keeps SUBSCRIBE and PSUBSCRIBE on one dedicated
// valkey-go connection. Its single dispatch queue preserves the order in which
// the server delivers normal and pattern messages.
type classicValkeyPubSub struct {
	ctx     context.Context
	cancel  context.CancelFunc
	client  vk.Client
	onError func(error)

	mu       sync.RWMutex
	channels classicValkeyPubSubRoutes
	patterns classicValkeyPubSubRoutes

	messages *queue.Queue
	wake     chan struct{}
	barriers chan chan struct{}
	done     chan struct{}
	close    sync.Once
}

func newClassicValkeyPubSub(parent context.Context, client vk.Client, onError func(error)) *classicValkeyPubSub {
	ctx, cancel := context.WithCancel(parent)
	p := &classicValkeyPubSub{
		ctx:      ctx,
		cancel:   cancel,
		client:   client,
		onError:  onError,
		channels: newClassicValkeyPubSubRoutes(),
		patterns: newClassicValkeyPubSubRoutes(),
		messages: queue.New(),
		wake:     make(chan struct{}, 1),
		barriers: make(chan chan struct{}),
		done:     make(chan struct{}),
	}
	go p.run()
	return p
}

func (p *classicValkeyPubSub) newSubscription(handler classicValkeyMessageHandler) *classicValkeySubscription {
	return &classicValkeySubscription{
		pubSub:   p,
		handler:  handler,
		channels: make(map[string]struct{}),
		patterns: make(map[string]struct{}),
	}
}

func (p *classicValkeyPubSub) add(routes *classicValkeyPubSubRoutes, key string, subscription *classicValkeySubscription) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		return false
	}
	if route, exists := routes.handlers[key]; exists {
		if route.first != subscription {
			if route.others == nil {
				route.others = make(map[*classicValkeySubscription]struct{})
			}
			route.others[subscription] = struct{}{}
			routes.handlers[key] = route
		}
		return true
	}
	routes.handlers[key] = classicValkeyPubSubRoute{first: subscription}
	return true
}

func (p *classicValkeyPubSub) remove(routes *classicValkeyPubSubRoutes, key string, subscription *classicValkeySubscription) {
	p.mu.Lock()
	defer p.mu.Unlock()
	route, exists := routes.handlers[key]
	if !exists {
		return
	}
	if route.first != subscription {
		delete(route.others, subscription)
		if len(route.others) == 0 {
			route.others = nil
		}
		routes.handlers[key] = route
		return
	}
	for replacement := range route.others {
		route.first = replacement
		delete(route.others, replacement)
		if len(route.others) == 0 {
			route.others = nil
		}
		routes.handlers[key] = route
		return
	}
	delete(routes.handlers, key)
}

func (p *classicValkeyPubSub) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *classicValkeyPubSub) flush(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case p.barriers <- done:
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func (p *classicValkeyPubSub) run() {
	defer close(p.done)
	var (
		client     vk.DedicatedClient
		hookErrors <-chan error
		retry      <-chan time.Time
	)

	disconnect := func() {
		p.mu.Lock()
		clear(p.channels.active)
		clear(p.patterns.active)
		p.mu.Unlock()
		if client != nil {
			client.SetPubSubHooks(vk.PubSubHooks{})
			// Close also tears down valkey-go's internal RESP2 Pub/Sub connection.
			client.Close()
			client = nil
			hookErrors = nil
		}
	}
	defer disconnect()

	scheduleRetry := func() {
		if retry == nil {
			retry = time.After(classicValkeyPubSubRetryDelay)
		}
	}
	reconcile := func() {
		if client == nil {
			client, _ = p.client.Dedicate()
			hookErrors = client.SetPubSubHooks(vk.PubSubHooks{
				OnMessage:      p.dispatchMessage,
				OnSubscription: p.onSubscription,
			})
		}
		if p.reconcile(client) {
			scheduleRetry()
		}
	}

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.wake:
			if retry == nil {
				reconcile()
			}
		case done := <-p.barriers:
			if retry == nil {
				reconcile()
			}
			close(done)
		case err, ok := <-hookErrors:
			if ok {
				p.report(err)
				// The channel closes only after the old hooks can no longer run.
				for range hookErrors {
				}
			}
			disconnect()
			if p.ctx.Err() == nil {
				scheduleRetry()
			}
		case <-retry:
			retry = nil
			reconcile()
		}
	}
}

func (p *classicValkeyPubSub) changes(routes *classicValkeyPubSubRoutes) (subscribe, unsubscribe []string) {
	p.mu.RLock()
	for key := range routes.handlers {
		if _, exists := routes.active[key]; !exists {
			subscribe = append(subscribe, key)
		}
	}
	for key := range routes.active {
		if _, exists := routes.handlers[key]; !exists {
			unsubscribe = append(unsubscribe, key)
		}
	}
	p.mu.RUnlock()
	return
}

func (p *classicValkeyPubSub) reconcile(client vk.DedicatedClient) bool {
	channelSubscribe, channelUnsubscribe := p.changes(&p.channels)
	patternSubscribe, patternUnsubscribe := p.changes(&p.patterns)

	if p.reconcileBatch(client, true, true, patternSubscribe) {
		return true
	}
	if p.reconcileBatch(client, false, true, channelSubscribe) {
		return true
	}
	if p.reconcileBatch(client, false, false, channelUnsubscribe) {
		return true
	}
	return p.reconcileBatch(client, true, false, patternUnsubscribe)
}

func (p *classicValkeyPubSub) reconcileBatch(client vk.DedicatedClient, pattern, subscribe bool, keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	var command vk.Completed
	switch {
	case pattern && subscribe:
		command = client.B().Psubscribe().Pattern(keys...).Build()
	case pattern:
		command = client.B().Punsubscribe().Pattern(keys...).Build()
	case subscribe:
		command = client.B().Subscribe().Channel(keys...).Build()
	default:
		command = client.B().Unsubscribe().Channel(keys...).Build()
	}
	if err := client.Do(p.ctx, command).Error(); err != nil {
		p.report(err)
		return true
	}
	return false
}

func (p *classicValkeyPubSub) onSubscription(subscription vk.PubSubSubscription) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var routes *classicValkeyPubSubRoutes
	switch subscription.Kind {
	case "subscribe", "unsubscribe":
		routes = &p.channels
	case "psubscribe", "punsubscribe":
		routes = &p.patterns
	default:
		return
	}
	if subscription.Kind == "subscribe" || subscription.Kind == "psubscribe" {
		routes.active[subscription.Channel] = struct{}{}
	} else {
		delete(routes.active, subscription.Channel)
	}
}

func (p *classicValkeyPubSub) dispatchMessage(message vk.PubSubMessage) {
	routes := &p.channels
	key := message.Channel
	if message.Pattern != "" {
		routes = &p.patterns
		key = message.Pattern
	}

	p.mu.RLock()
	route, exists := routes.handlers[key]
	if !exists {
		p.mu.RUnlock()
		return
	}
	if len(route.others) == 0 {
		handler := route.first.handler
		p.mu.RUnlock()
		p.messages.Enqueue(func() {
			handler([]byte(message.Message), message.Channel)
		})
		return
	}
	handlers := make([]classicValkeyMessageHandler, 0, len(route.others)+1)
	handlers = append(handlers, route.first.handler)
	for subscription := range route.others {
		handlers = append(handlers, subscription.handler)
	}
	p.mu.RUnlock()
	p.messages.Enqueue(func() {
		payload := []byte(message.Message)
		for _, handler := range handlers {
			handler(payload, message.Channel)
		}
	})
}

func (p *classicValkeyPubSub) report(err error) {
	if err != nil && p.ctx.Err() == nil && p.onError != nil {
		go p.onError(err)
	}
}

func (p *classicValkeyPubSub) Close() {
	p.close.Do(func() {
		p.mu.Lock()
		p.cancel()
		p.channels.handlers = nil
		p.patterns.handlers = nil
		p.mu.Unlock()
		<-p.done
		p.messages.TryClose()
	})
}

type classicValkeySubscription struct {
	pubSub   *classicValkeyPubSub
	handler  classicValkeyMessageHandler
	mu       sync.Mutex
	channels map[string]struct{}
	patterns map[string]struct{}
	closed   bool
}

func (s *classicValkeySubscription) Subscribe(channels ...string) {
	s.add(false, channels)
}

func (s *classicValkeySubscription) PSubscribe(patterns ...string) {
	s.add(true, patterns)
}

func (s *classicValkeySubscription) add(pattern bool, keys []string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	owned := s.channels
	routes := &s.pubSub.channels
	if pattern {
		owned = s.patterns
		routes = &s.pubSub.patterns
	}
	for _, key := range keys {
		if _, exists := owned[key]; exists {
			continue
		}
		if s.pubSub.add(routes, key, s) {
			owned[key] = struct{}{}
		}
	}
	s.mu.Unlock()
	s.pubSub.signal()
}

func (s *classicValkeySubscription) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	channels, patterns := s.channels, s.patterns
	s.channels, s.patterns = nil, nil
	s.mu.Unlock()
	for channel := range channels {
		s.pubSub.remove(&s.pubSub.channels, channel, s)
	}
	for pattern := range patterns {
		s.pubSub.remove(&s.pubSub.patterns, pattern, s)
	}
	s.pubSub.signal()
}
