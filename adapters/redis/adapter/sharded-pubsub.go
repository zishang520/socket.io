package adapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

const (
	shardedPubSubRetryDelay      = time.Second
	shardedPubSubBackoffBase     = 100 * time.Millisecond
	shardedPubSubBackoffMax      = 2 * time.Second
	shardedPubSubStableAfter     = 5 * time.Second
	shardedPubSubMaxFailureCount = 7
	shardedPubSubAuditEvery      = 5 * time.Second
)

type shardedMessageHandler func(payload []byte, channel string)

type sharedShardedPubSub struct {
	pubSub *shardedPubSub
	refs   int
}

type shardedPubSubCacheKey struct {
	server *socket.Server
	client *redis.RedisClient
}

type shardedPubSubCache struct {
	mu     sync.Mutex
	groups map[shardedPubSubCacheKey]*sharedShardedPubSub
}

func (c *shardedPubSubCache) acquire(server *socket.Server, client *redis.RedisClient) *shardedPubSub {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.groups == nil {
		c.groups = make(map[shardedPubSubCacheKey]*sharedShardedPubSub)
	}
	key := shardedPubSubCacheKey{server: server, client: client}
	shared := c.groups[key]
	if shared == nil {
		shared = &sharedShardedPubSub{pubSub: newShardedPubSub(client.Context(), client.Sub(), func(err error) {
			client.Emit("error", err)
		})}
		c.groups[key] = shared
	}
	shared.refs++
	return shared.pubSub
}

func (c *shardedPubSubCache) release(server *socket.Server, client *redis.RedisClient, pubSub *shardedPubSub) {
	c.mu.Lock()
	key := shardedPubSubCacheKey{server: server, client: client}
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

var shardedRedisPubSubs shardedPubSubCache

func acquireShardedPubSub(server *socket.Server, client *redis.RedisClient) *shardedPubSub {
	return shardedRedisPubSubs.acquire(server, client)
}

func releaseShardedPubSub(server *socket.Server, client *redis.RedisClient, pubSub *shardedPubSub) {
	shardedRedisPubSubs.release(server, client, pubSub)
}

type shardedRoute struct {
	first  *shardedSubscription
	others map[*shardedSubscription]struct{}
}

type shardedPoolKey struct {
	node    *rds.Client
	channel string // only used for an opaque UniversalClient
}

type shardedPool struct {
	key      shardedPoolKey
	pubSub   *rds.PubSub
	channels map[string]struct{}
}

type shardedPubSubEvent struct {
	pool *shardedPool
	err  error
}

// shardedPubSub owns the Redis subscription state. Like ioredis'
// ClusterSubscriberGroup, it shares one connection per owner; adapters only
// register handlers and run serializes Redis I/O.
type shardedPubSub struct {
	ctx    context.Context
	cancel context.CancelFunc
	client rds.UniversalClient

	mu       sync.RWMutex
	routes   map[string]shardedRoute
	dirty    map[string]struct{}
	closed   bool
	wake     chan struct{}
	barriers chan chan struct{}
	events   chan shardedPubSubEvent
	errors   chan error
	done     chan struct{}
	close    sync.Once

	pools        map[shardedPoolKey]*shardedPool // owned by run
	channels     map[string]*shardedPool         // owned by run
	failureBurst uint8                           // owned by run
	lastFailure  time.Time                       // owned by run
	scratch      []string                        // owned by run
}

func newShardedPubSub(parent context.Context, client rds.UniversalClient, onError func(error)) *shardedPubSub {
	ctx, cancel := context.WithCancel(parent)
	s := &shardedPubSub{
		ctx:      ctx,
		cancel:   cancel,
		client:   client,
		routes:   make(map[string]shardedRoute),
		dirty:    make(map[string]struct{}),
		wake:     make(chan struct{}, 1),
		barriers: make(chan chan struct{}),
		events:   make(chan shardedPubSubEvent),
		errors:   make(chan error, 1),
		done:     make(chan struct{}),
		pools:    make(map[shardedPoolKey]*shardedPool),
		channels: make(map[string]*shardedPool),
	}
	if onError != nil {
		go s.emitErrors(onError)
	}
	go s.run()
	return s
}

func (s *shardedPubSub) newSubscription(handler shardedMessageHandler) *shardedSubscription {
	return &shardedSubscription{
		pubSub:   s,
		handler:  handler,
		channels: make(map[string]struct{}),
	}
}

func (s *shardedPubSub) add(channel string, subscription *shardedSubscription) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if route, exists := s.routes[channel]; exists {
		if route.first == subscription {
			return true
		}
		if _, exists := route.others[subscription]; !exists {
			if route.others == nil {
				route.others = make(map[*shardedSubscription]struct{})
			}
			route.others[subscription] = struct{}{}
			s.routes[channel] = route
		}
		return true
	}
	s.routes[channel] = shardedRoute{first: subscription}
	s.dirty[channel] = struct{}{}
	s.signal()
	return true
}

func (s *shardedPubSub) remove(channel string, subscription *shardedSubscription) {
	s.mu.Lock()
	route, exists := s.routes[channel]
	if !exists {
		s.mu.Unlock()
		return
	}
	if route.first == subscription {
		for replacement := range route.others {
			route.first = replacement
			delete(route.others, replacement)
			if len(route.others) == 0 {
				route.others = nil
			}
			s.routes[channel] = route
			s.mu.Unlock()
			return
		}
		delete(s.routes, channel)
		s.dirty[channel] = struct{}{}
	} else if _, exists := route.others[subscription]; exists {
		delete(route.others, subscription)
		if len(route.others) == 0 {
			route.others = nil
		}
		s.routes[channel] = route
		s.mu.Unlock()
		return
	} else {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.signal()
}

func (s *shardedPubSub) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *shardedPubSub) desired(channel string) bool {
	s.mu.RLock()
	_, desired := s.routes[channel]
	s.mu.RUnlock()
	return desired
}

func (s *shardedPubSub) dispatch(channel string, payload []byte) {
	s.mu.RLock()
	route, exists := s.routes[channel]
	if !exists {
		s.mu.RUnlock()
		return
	}
	if len(route.others) == 0 {
		s.mu.RUnlock()
		route.first.handler(payload, channel)
		return
	}
	handlers := make([]shardedMessageHandler, 0, len(route.others)+1)
	handlers = append(handlers, route.first.handler)
	for subscription := range route.others {
		handlers = append(handlers, subscription.handler)
	}
	s.mu.RUnlock()
	for _, handler := range handlers {
		handler(payload, channel)
	}
}

func (s *shardedPubSub) takeDirty() []string {
	s.mu.Lock()
	channels := s.scratch[:0]
	for channel := range s.dirty {
		channels = append(channels, channel)
		delete(s.dirty, channel)
	}
	s.mu.Unlock()
	return channels
}

func (s *shardedPubSub) markDirty(channel string) {
	s.mu.Lock()
	if !s.closed {
		s.dirty[channel] = struct{}{}
	}
	s.mu.Unlock()
}

// flush waits until run has processed the current desired state once. Redis
// failures stay pending and are retried asynchronously.
func (s *shardedPubSub) flush(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case s.barriers <- done:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *shardedPubSub) run() {
	defer close(s.done)
	var retryC <-chan time.Time
	var auditC <-chan time.Time
	var audit *time.Ticker
	switch s.client.(type) {
	case *rds.ClusterClient:
		audit = time.NewTicker(shardedPubSubAuditEvery)
	}
	if audit != nil {
		defer audit.Stop()
		auditC = audit.C
	}

	scheduleRetry := func(delay time.Duration) {
		if retryC == nil {
			retryC = time.After(delay)
		}
	}

	for {
		select {
		case <-s.ctx.Done():
			s.mu.Lock()
			s.closed = true
			s.routes = nil
			s.dirty = nil
			s.mu.Unlock()
			s.closePools()
			return
		case <-s.wake:
			if s.reconcile(retryC == nil) {
				scheduleRetry(shardedPubSubRetryDelay)
			}
		case done := <-s.barriers:
			if s.reconcile(retryC == nil) {
				scheduleRetry(shardedPubSubRetryDelay)
			}
			close(done)
		case event := <-s.events:
			if !s.isCurrent(event.pool) {
				continue
			}
			s.dropPool(event.pool)
			s.reportSubscriptionError(event.err)
			if event.err != nil {
				if delay := s.receiveBackoff(time.Now()); delay > 0 {
					scheduleRetry(delay)
					continue
				}
				// Only the first failure in a burst bypasses an existing retry.
				retryC = nil
			} else if retryC != nil {
				continue
			}
			// Rebuild immediately. This avoids adding a fixed one-second outage
			// after the first connection error; repeated failures are backed off.
			if s.restore() {
				scheduleRetry(shardedPubSubRetryDelay)
			}
		case <-retryC:
			retryC = nil
			if s.restore() {
				scheduleRetry(shardedPubSubRetryDelay)
			}
		case <-auditC:
			if retryC == nil && s.restore() {
				scheduleRetry(shardedPubSubRetryDelay)
			}
		}
	}
}

func (s *shardedPubSub) restore() bool {
	if s.auditOwners() {
		return true
	}
	return s.reconcile(true)
}

func (s *shardedPubSub) receiveBackoff(now time.Time) time.Duration {
	if s.lastFailure.IsZero() || now.Sub(s.lastFailure) >= shardedPubSubStableAfter {
		s.failureBurst = 0
	}
	s.lastFailure = now
	if s.failureBurst < shardedPubSubMaxFailureCount {
		s.failureBurst++
	}
	if s.failureBurst == 1 {
		return 0
	}

	delay := shardedPubSubBackoffBase
	for attempt := uint8(2); attempt < s.failureBurst && delay < shardedPubSubBackoffMax; attempt++ {
		delay *= 2
	}
	if delay > shardedPubSubBackoffMax {
		delay = shardedPubSubBackoffMax
	}
	if jitter := delay / 4; jitter > 0 {
		delay -= time.Duration(rand.Int64N(int64(jitter) + 1))
	}
	return delay
}

func (s *shardedPubSub) reconcile(allowSubscribe bool) bool {
	failed := false
	blocked := false
	var failedPools map[shardedPoolKey]struct{}
	channels := s.takeDirty()
	for _, channel := range channels {
		pool := s.channels[channel]
		if !s.desired(channel) {
			if pool != nil {
				if err := s.unsubscribe(pool, channel); err != nil {
					s.reportSubscriptionError(err)
					failed = true
				}
			}
			continue
		}
		if pool != nil {
			continue
		}
		if !allowSubscribe || blocked {
			s.markDirty(channel)
			continue
		}
		key, err := s.resolve(channel)
		if err != nil {
			s.markDirty(channel)
			if !isRecoverableShardedPubSubError(err) {
				s.report(fmt.Errorf("resolve %q: %w", channel, err))
			}
			failed = true
			blocked = true
			continue
		}
		if _, unavailable := failedPools[key]; unavailable {
			s.markDirty(channel)
			continue
		}
		if err := s.subscribe(key, channel); err != nil {
			s.markDirty(channel)
			if !isRecoverableShardedPubSubError(err) {
				s.report(fmt.Errorf("SSUBSCRIBE %q: %w", channel, err))
			}
			failed = true
			if isRecoverableShardedPubSubError(err) {
				if key.node == nil {
					blocked = true
				} else {
					if failedPools == nil {
						failedPools = make(map[shardedPoolKey]struct{})
					}
					failedPools[key] = struct{}{}
				}
			}
		}
	}
	clear(channels)
	s.scratch = channels[:0]
	return failed
}

func (s *shardedPubSub) resolve(channel string) (shardedPoolKey, error) {
	switch client := s.client.(type) {
	case *rds.ClusterClient:
		node, err := client.MasterForKey(s.ctx, channel)
		return shardedPoolKey{node: node}, err
	case *rds.Client:
		return shardedPoolKey{node: client}, nil
	default:
		return shardedPoolKey{channel: channel}, nil
	}
}

func (s *shardedPubSub) subscribe(key shardedPoolKey, channel string) error {
	pool := s.pools[key]
	if pool == nil {
		pool = &shardedPool{key: key, channels: make(map[string]struct{})}
		if key.node == nil {
			pool.pubSub = s.client.SSubscribe(s.ctx, channel)
		} else {
			pool.pubSub = key.node.SSubscribe(s.ctx)
			if err := pool.pubSub.SSubscribe(s.ctx, channel); err != nil {
				_ = pool.pubSub.Close()
				return err
			}
		}
		s.pools[key] = pool
		pool.channels[channel] = struct{}{}
		s.channels[channel] = pool
		go s.receive(pool)
		return nil
	}
	if err := pool.pubSub.SSubscribe(s.ctx, channel); err != nil {
		s.dropPool(pool)
		return err
	}
	pool.channels[channel] = struct{}{}
	s.channels[channel] = pool
	return nil
}

func (s *shardedPubSub) unsubscribe(pool *shardedPool, channel string) error {
	delete(pool.channels, channel)
	delete(s.channels, channel)
	if len(pool.channels) == 0 {
		delete(s.pools, pool.key)
		return pool.pubSub.Close()
	}
	if err := pool.pubSub.SUnsubscribe(s.ctx, channel); err != nil {
		s.dropPool(pool)
		return err
	}
	return nil
}

func (s *shardedPubSub) receive(pool *shardedPool) {
	for {
		value, err := pool.pubSub.Receive(s.ctx)
		if err != nil {
			s.notify(shardedPubSubEvent{pool: pool, err: err})
			return
		}
		switch value := value.(type) {
		case *rds.Message:
			s.dispatch(value.Channel, []byte(value.Payload))
		case *rds.Subscription:
			if value.Kind == "sunsubscribe" && value.Channel != "" && s.desired(value.Channel) {
				s.notify(shardedPubSubEvent{pool: pool})
				return
			}
		}
	}
}

func (s *shardedPubSub) notify(event shardedPubSubEvent) {
	select {
	case s.events <- event:
	case <-s.ctx.Done():
	}
}

func (s *shardedPubSub) isCurrent(pool *shardedPool) bool {
	return pool != nil && s.pools[pool.key] == pool
}

func (s *shardedPubSub) dropPool(pool *shardedPool) {
	if !s.isCurrent(pool) {
		return
	}
	delete(s.pools, pool.key)
	for channel := range pool.channels {
		delete(s.channels, channel)
		if s.desired(channel) {
			s.markDirty(channel)
		}
	}
	pool.channels = nil
	_ = pool.pubSub.Close()
}

func (s *shardedPubSub) closePools() {
	for _, pool := range s.pools {
		_ = pool.pubSub.Close()
		pool.channels = nil
	}
	s.pools = nil
	s.channels = nil
}

func (s *shardedPubSub) auditOwners() bool {
	if client, ok := s.client.(*rds.ClusterClient); ok {
		if err := client.ForEachMaster(s.ctx, func(ctx context.Context, _ *rds.Client) error {
			return ctx.Err()
		}); err != nil {
			s.report(err)
			return true
		}
	}
	for _, pool := range s.pools {
		for channel := range pool.channels {
			key, err := s.resolve(channel)
			if err != nil {
				s.report(err)
				return true
			}
			if key != pool.key {
				s.dropPool(pool)
				break
			}
		}
	}
	return false
}

func (s *shardedPubSub) reportSubscriptionError(err error) {
	if err == nil || isRecoverableShardedPubSubError(err) {
		return
	}
	s.report(err)
}

func (s *shardedPubSub) report(err error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, rds.ErrClosed) {
		return
	}
	select {
	case s.errors <- err:
	default:
	}
}

func (s *shardedPubSub) emitErrors(handler func(error)) {
	for {
		select {
		case err := <-s.errors:
			handler(err)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *shardedPubSub) Close() {
	s.close.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.routes = nil
		s.dirty = nil
		s.mu.Unlock()
		s.cancel()
		<-s.done
	})
}

func isRecoverableShardedPubSubError(err error) bool {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, rds.ErrClosed) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return true
	}
	for _, prefix := range []string{"MOVED", "ASK", "CROSSSLOT", "TRYAGAIN", "CLUSTERDOWN", "READONLY"} {
		if rds.HasErrorPrefix(err, prefix) {
			return true
		}
	}
	return false
}

type shardedSubscription struct {
	pubSub   *shardedPubSub
	handler  shardedMessageHandler
	mu       sync.Mutex
	channels map[string]struct{}
	closed   bool
}

func (s *shardedSubscription) Subscribe(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if _, exists := s.channels[channel]; exists {
		return
	}
	if s.pubSub.add(channel, s) {
		s.channels[channel] = struct{}{}
	}
}

func (s *shardedSubscription) Unsubscribe(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if _, exists := s.channels[channel]; !exists {
		return
	}
	delete(s.channels, channel)
	s.pubSub.remove(channel, s)
}

func (s *shardedSubscription) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	channels := s.channels
	s.channels = nil
	s.mu.Unlock()
	for channel := range channels {
		s.pubSub.remove(channel, s)
	}
}
