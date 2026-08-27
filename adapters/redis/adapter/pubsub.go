package adapter

import (
	"context"
	"errors"
	"sync"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

const redisPubSubRetryDelay = time.Second

type redisMessageHandler func(payload []byte, channel string)

type sharedRedisPubSub struct {
	pubSub *redisPubSub
	refs   int
}

type redisPubSubCacheKey struct {
	server *socket.Server
	client *redis.RedisClient
}

type redisPubSubCache struct {
	mu     sync.Mutex
	groups map[redisPubSubCacheKey]*sharedRedisPubSub
}

func (c *redisPubSubCache) acquire(server *socket.Server, client *redis.RedisClient) *redisPubSub {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.groups == nil {
		c.groups = make(map[redisPubSubCacheKey]*sharedRedisPubSub)
	}
	key := redisPubSubCacheKey{server: server, client: client}
	shared := c.groups[key]
	if shared == nil {
		shared = &sharedRedisPubSub{pubSub: newRedisPubSub(client.Context(), client.Sub(), func(err error) {
			client.Emit("error", err)
		})}
		c.groups[key] = shared
	}
	shared.refs++
	return shared.pubSub
}

func (c *redisPubSubCache) release(server *socket.Server, client *redis.RedisClient, pubSub *redisPubSub) {
	c.mu.Lock()
	key := redisPubSubCacheKey{server: server, client: client}
	shared := c.groups[key]
	if shared == nil || shared.pubSub != pubSub {
		c.mu.Unlock()
		return
	}
	shared.refs--
	if shared.refs != 0 {
		c.mu.Unlock()
		return
	}
	delete(c.groups, key)
	if len(c.groups) == 0 {
		c.groups = nil
	}
	c.mu.Unlock()
	pubSub.Close()
}

var redisPubSubs redisPubSubCache

func acquireRedisPubSub(server *socket.Server, client *redis.RedisClient) *redisPubSub {
	return redisPubSubs.acquire(server, client)
}

func releaseRedisPubSub(server *socket.Server, client *redis.RedisClient, pubSub *redisPubSub) {
	redisPubSubs.release(server, client, pubSub)
}

type redisPubSubRoute struct {
	first  *redisSubscription
	others map[*redisSubscription]struct{}
}

type redisPubSubRoutes struct {
	handlers           map[string]redisPubSubRoute
	dirty              map[string]struct{}
	active             map[string]struct{} // protected by redisPubSub.mu
	subscribeScratch   []string            // owned by run
	unsubscribeScratch []string            // owned by run
}

func newRedisPubSubRoutes() redisPubSubRoutes {
	return redisPubSubRoutes{
		handlers: make(map[string]redisPubSubRoute),
		dirty:    make(map[string]struct{}),
		active:   make(map[string]struct{}),
	}
}

// redisPubSub multiplexes normal Redis Pub/Sub over one subscriber connection.
// A Socket.IO server therefore keeps a constant number of connections even
// when it creates many namespaces.
type redisPubSub struct {
	ctx     context.Context
	cancel  context.CancelFunc
	client  rds.UniversalClient
	pubSub  *rds.PubSub
	onError func(error)

	mu       sync.RWMutex
	channels redisPubSubRoutes
	patterns redisPubSubRoutes
	closed   bool
	wake     chan struct{}
	barriers chan chan struct{}
	close    sync.Once
}

func newRedisPubSub(parent context.Context, client rds.UniversalClient, onError func(error)) *redisPubSub {
	ctx, cancel := context.WithCancel(parent)
	p := &redisPubSub{
		ctx:      ctx,
		cancel:   cancel,
		client:   client,
		onError:  onError,
		channels: newRedisPubSubRoutes(),
		patterns: newRedisPubSubRoutes(),
		wake:     make(chan struct{}, 1),
		barriers: make(chan chan struct{}),
	}
	p.pubSub = client.Subscribe(ctx)
	go p.receive(p.pubSub)
	go p.run()
	return p
}

func (p *redisPubSub) receive(pubSub *rds.PubSub) {
	recovering := false
	for {
		value, err := pubSub.Receive(p.ctx)
		if err == nil {
			switch value := value.(type) {
			case *rds.Message:
				p.dispatchMessage(value)
			case *rds.Subscription:
				p.mu.RLock()
				recovered := value.Count == len(p.channels.handlers)+len(p.patterns.handlers)
				p.mu.RUnlock()
				if recovered {
					recovering = false
				}
			}
			continue
		}
		if p.ctx.Err() != nil || errors.Is(err, rds.ErrClosed) {
			return
		}
		var redisErr rds.Error
		serverError := errors.As(err, &redisErr)
		p.report(err)

		// go-redis reconnects network failures on the existing PubSub. A rejected
		// subscription command leaves its local channel set out of sync with
		// Redis, so replace the connection once and rebuild the desired state.
		// Further errors in the same recovery cycle retry on that replacement;
		// otherwise a persistent NOPERM would rebuild once per second.
		if !serverError || recovering {
			timer := time.NewTimer(redisPubSubRetryDelay)
			select {
			case <-timer.C:
			case <-p.ctx.Done():
				timer.Stop()
				return
			}
		}
		if !serverError {
			continue
		}

		p.mu.Lock()
		if p.closed || p.pubSub != pubSub {
			p.mu.Unlock()
			return
		}
		replacement := pubSub
		if !recovering {
			replacement = p.client.Subscribe(p.ctx)
			p.pubSub = replacement
		}
		for _, routes := range []*redisPubSubRoutes{&p.channels, &p.patterns} {
			clear(routes.active)
			for key := range routes.handlers {
				routes.dirty[key] = struct{}{}
			}
		}
		desired := len(p.channels.handlers) + len(p.patterns.handlers)
		p.mu.Unlock()
		rebuilt := replacement != pubSub
		if rebuilt {
			_ = pubSub.Close()
			pubSub = replacement
			recovering = desired != 0
		}
		p.signal()
	}
}

func (p *redisPubSub) newSubscription(handler redisMessageHandler) *redisSubscription {
	return &redisSubscription{
		pubSub:   p,
		handler:  handler,
		channels: make(map[string]struct{}),
		patterns: make(map[string]struct{}),
	}
}

func (p *redisPubSub) add(routes *redisPubSubRoutes, key string, subscription *redisSubscription) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	if route, exists := routes.handlers[key]; exists {
		if route.first != subscription {
			if route.others == nil {
				route.others = make(map[*redisSubscription]struct{})
			}
			route.others[subscription] = struct{}{}
			routes.handlers[key] = route
		}
		return true
	}
	routes.handlers[key] = redisPubSubRoute{first: subscription}
	routes.dirty[key] = struct{}{}
	return true
}

func (p *redisPubSub) remove(routes *redisPubSubRoutes, key string, subscription *redisSubscription) {
	p.mu.Lock()
	route, exists := routes.handlers[key]
	if !exists {
		p.mu.Unlock()
		return
	}
	if route.first == subscription {
		for replacement := range route.others {
			route.first = replacement
			delete(route.others, replacement)
			if len(route.others) == 0 {
				route.others = nil
			}
			routes.handlers[key] = route
			p.mu.Unlock()
			return
		}
		delete(routes.handlers, key)
		routes.dirty[key] = struct{}{}
	} else if _, exists := route.others[subscription]; exists {
		delete(route.others, subscription)
		if len(route.others) == 0 {
			route.others = nil
		}
		routes.handlers[key] = route
		p.mu.Unlock()
		return
	} else {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
}

func (p *redisPubSub) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *redisPubSub) takeDirty(routes *redisPubSubRoutes) ([]string, []string) {
	p.mu.Lock()
	subscribe := routes.subscribeScratch[:0]
	unsubscribe := routes.unsubscribeScratch[:0]
	for key := range routes.dirty {
		if _, desired := routes.handlers[key]; desired {
			subscribe = append(subscribe, key)
		} else if _, active := routes.active[key]; active {
			unsubscribe = append(unsubscribe, key)
		}
		delete(routes.dirty, key)
	}
	p.mu.Unlock()
	return subscribe, unsubscribe
}

func (p *redisPubSub) markDirty(routes *redisPubSubRoutes, keys []string) {
	p.mu.Lock()
	if !p.closed {
		for _, key := range keys {
			routes.dirty[key] = struct{}{}
		}
	}
	p.mu.Unlock()
}

func (p *redisPubSub) flush(ctx context.Context) error {
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

func (p *redisPubSub) run() {
	var retry <-chan time.Time
	scheduleRetry := func() {
		if retry == nil {
			retry = time.After(redisPubSubRetryDelay)
		}
	}
	reconcile := func() {
		channelsFailed := p.reconcile(&p.channels, false)
		patternsFailed := p.reconcile(&p.patterns, true)
		if channelsFailed || patternsFailed {
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
		case <-retry:
			retry = nil
			reconcile()
		}
	}
}

func (p *redisPubSub) reconcile(routes *redisPubSubRoutes, pattern bool) bool {
	subscribe, unsubscribe := p.takeDirty(routes)
	failed := p.reconcileBatch(routes, pattern, true, subscribe)
	if p.reconcileBatch(routes, pattern, false, unsubscribe) {
		failed = true
	}
	clear(subscribe)
	clear(unsubscribe)
	routes.subscribeScratch = subscribe[:0]
	routes.unsubscribeScratch = unsubscribe[:0]
	return failed
}

func (p *redisPubSub) reconcileBatch(routes *redisPubSubRoutes, pattern, subscribe bool, keys []string) bool {
	if len(keys) == 0 {
		return false
	}

	p.mu.RLock()
	pubSub := p.pubSub
	p.mu.RUnlock()
	var err error
	switch {
	case pattern && subscribe:
		err = pubSub.PSubscribe(p.ctx, keys...)
	case pattern:
		err = pubSub.PUnsubscribe(p.ctx, keys...)
	case subscribe:
		err = pubSub.Subscribe(p.ctx, keys...)
	default:
		err = pubSub.Unsubscribe(p.ctx, keys...)
	}

	if subscribe {
		// go-redis records every channel even when the write fails, so a later
		// removal must still send UNSUBSCRIBE.
		p.mu.Lock()
		if p.pubSub == pubSub {
			for _, key := range keys {
				routes.active[key] = struct{}{}
			}
		}
		p.mu.Unlock()
	}
	if err != nil {
		p.markDirty(routes, keys)
		p.report(err)
		return true
	}
	if !subscribe {
		p.mu.Lock()
		for _, key := range keys {
			delete(routes.active, key)
		}
		p.mu.Unlock()
	}
	return false
}

func (p *redisPubSub) dispatchMessage(message *rds.Message) {
	if message.Pattern == "" {
		p.dispatch(&p.channels, message.Channel, message.Channel, []byte(message.Payload))
	} else {
		p.dispatch(&p.patterns, message.Pattern, message.Channel, []byte(message.Payload))
	}
}

func (p *redisPubSub) dispatch(routes *redisPubSubRoutes, key, channel string, payload []byte) {
	p.mu.RLock()
	route, exists := routes.handlers[key]
	if !exists {
		p.mu.RUnlock()
		return
	}
	if len(route.others) == 0 {
		p.mu.RUnlock()
		route.first.handler(payload, channel)
		return
	}
	handlers := make([]redisMessageHandler, 0, len(route.others)+1)
	handlers = append(handlers, route.first.handler)
	for subscription := range route.others {
		handlers = append(handlers, subscription.handler)
	}
	p.mu.RUnlock()
	for _, handler := range handlers {
		handler(payload, channel)
	}
}

func (p *redisPubSub) report(err error) {
	if err != nil && p.ctx.Err() == nil && !errors.Is(err, rds.ErrClosed) && p.onError != nil {
		p.onError(err)
	}
}

func (p *redisPubSub) Close() {
	p.close.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.channels.handlers = nil
		p.patterns.handlers = nil
		pubSub := p.pubSub
		p.mu.Unlock()
		p.cancel()
		if pubSub != nil {
			_ = pubSub.Close()
		}
	})
}

type redisSubscription struct {
	pubSub   *redisPubSub
	handler  redisMessageHandler
	mu       sync.Mutex
	channels map[string]struct{}
	patterns map[string]struct{}
	closed   bool
}

func (s *redisSubscription) Subscribe(channels ...string) {
	s.add(false, channels)
}

func (s *redisSubscription) PSubscribe(patterns ...string) {
	s.add(true, patterns)
}

func (s *redisSubscription) add(pattern bool, keys []string) {
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

func (s *redisSubscription) Close() {
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
