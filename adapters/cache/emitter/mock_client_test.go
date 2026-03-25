package emitter

// mockCacheClient is a minimal in-memory CacheClient used by emitter tests.
// It records every Publish / SPublish call so tests can assert channel routing.

import (
	"context"
	"sync"
	"time"

	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type publishCall struct {
	channel string
	message any
}

type mockCacheClient struct {
	types.EventEmitter
	ctx context.Context

	mu       sync.Mutex
	publishes  []publishCall
	spublishes []publishCall
}

func newMock() *mockCacheClient {
	return &mockCacheClient{
		EventEmitter: types.NewEventEmitter(),
		ctx:          context.Background(),
	}
}

func (m *mockCacheClient) Context() context.Context { return m.ctx }

func (m *mockCacheClient) Publish(_ context.Context, channel string, message any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishes = append(m.publishes, publishCall{channel, message})
	return nil
}

func (m *mockCacheClient) SPublish(_ context.Context, channel string, message any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spublishes = append(m.spublishes, publishCall{channel, message})
	return nil
}

// lastPublish returns the most recent Publish call or panics if there was none.
func (m *mockCacheClient) lastPublish() publishCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.publishes) == 0 {
		panic("no Publish calls recorded")
	}
	return m.publishes[len(m.publishes)-1]
}

// lastSPublish returns the most recent SPublish call or panics if there was none.
func (m *mockCacheClient) lastSPublish() publishCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.spublishes) == 0 {
		panic("no SPublish calls recorded")
	}
	return m.spublishes[len(m.spublishes)-1]
}

func (m *mockCacheClient) publishCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.publishes)
}

// --- unused interface stubs ---

func (m *mockCacheClient) Subscribe(_ context.Context, _ ...string) cache.CacheSubscription {
	return &noopSub{}
}
func (m *mockCacheClient) PSubscribe(_ context.Context, _ ...string) cache.CacheSubscription {
	return &noopSub{}
}
func (m *mockCacheClient) PubSubNumSub(_ context.Context, _ ...string) (map[string]int64, error) {
	return nil, nil
}
func (m *mockCacheClient) SSubscribe(_ context.Context, _ ...string) cache.CacheSubscription {
	return &noopSub{}
}
func (m *mockCacheClient) PubSubShardNumSub(_ context.Context, _ ...string) (map[string]int64, error) {
	return nil, nil
}
func (m *mockCacheClient) XAdd(_ context.Context, _ string, _ int64, _ bool, _ map[string]any) (string, error) {
	return "0-0", nil
}
func (m *mockCacheClient) XRead(_ context.Context, _ []string, _ string, _ int64, _ time.Duration) ([]cache.CacheStream, error) {
	return nil, nil
}
func (m *mockCacheClient) XRange(_ context.Context, _, _, _ string) ([]cache.CacheStreamEntry, error) {
	return nil, nil
}
func (m *mockCacheClient) XRangeN(_ context.Context, _, _, _ string, _ int64) ([]cache.CacheStreamEntry, error) {
	return nil, nil
}
func (m *mockCacheClient) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}
func (m *mockCacheClient) GetDel(_ context.Context, _ string) (string, error) {
	return "", cache.ErrNil
}

// noopSub is a no-op CacheSubscription.
type noopSub struct{}

func (n *noopSub) C() <-chan *cache.CacheMessage                         { return nil }
func (n *noopSub) PUnsubscribe(_ context.Context, _ ...string) error    { return nil }
func (n *noopSub) Unsubscribe(_ context.Context, _ ...string) error     { return nil }
func (n *noopSub) SUnsubscribe(_ context.Context, _ ...string) error    { return nil }
func (n *noopSub) Close() error                                          { return nil }

// compile-time checks
var _ cache.CacheClient       = (*mockCacheClient)(nil)
var _ cache.CacheSubscription = (*noopSub)(nil)
