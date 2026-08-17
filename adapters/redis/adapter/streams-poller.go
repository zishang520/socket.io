package adapter

import (
	"context"
	"errors"
	"sync"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const maxRedisStreamsPollerBlock = time.Duration(DefaultBlockTimeInMs) * time.Millisecond

type redisStreamsPollerConfig struct {
	readCount int64
	block     time.Duration
}

type redisStreamsPollerKey struct {
	server     *socket.Server
	client     *redis.RedisClient
	streamName string
	config     redisStreamsPollerConfig
}

type redisStreamsPoller struct {
	key      redisStreamsPollerKey
	ctx      context.Context
	cancel   context.CancelFunc
	client   rds.UniversalClient
	adapters types.Map[string, *redisStreamsAdapter]
	done     chan struct{}
}

type sharedRedisStreamsPoller struct {
	poller *redisStreamsPoller
	refs   int
}

var redisStreamsPollers struct {
	mu     sync.Mutex
	groups map[redisStreamsPollerKey]*sharedRedisStreamsPoller
}

func redisStreamsPollerBlock(blockTimeInMs int64) time.Duration {
	if blockTimeInMs <= 0 || blockTimeInMs > DefaultBlockTimeInMs {
		return maxRedisStreamsPollerBlock
	}
	return time.Duration(blockTimeInMs) * time.Millisecond
}

func acquireRedisStreamsPoller(adapter *redisStreamsAdapter) *redisStreamsPoller {
	config := redisStreamsPollerConfig{
		readCount: adapter.opts.ReadCount(),
		block:     redisStreamsPollerBlock(adapter.opts.BlockTimeInMs()),
	}
	key := redisStreamsPollerKey{
		server:     adapter.server,
		client:     adapter.redisClient,
		streamName: adapter.streamName,
		config:     config,
	}

	redisStreamsPollers.mu.Lock()
	shared := redisStreamsPollers.groups[key]
	created := shared == nil
	if shared == nil {
		ctx, cancel := context.WithCancel(adapter.redisClient.Context())
		shared = &sharedRedisStreamsPoller{poller: &redisStreamsPoller{
			key:    key,
			ctx:    ctx,
			cancel: cancel,
			client: adapter.redisClient.Sub(),
			done:   make(chan struct{}),
		}}
		if redisStreamsPollers.groups == nil {
			redisStreamsPollers.groups = make(map[redisStreamsPollerKey]*sharedRedisStreamsPoller)
		}
		redisStreamsPollers.groups[key] = shared
	}
	shared.refs++
	shared.poller.adapters.Store(adapter.Nsp().Name(), adapter)
	redisStreamsPollers.mu.Unlock()

	if created {
		go shared.poller.poll()
	}
	return shared.poller
}

func releaseRedisStreamsPoller(poller *redisStreamsPoller, adapter *redisStreamsAdapter) {
	poller.adapters.CompareAndDelete(adapter.Nsp().Name(), adapter)

	redisStreamsPollers.mu.Lock()
	shared := redisStreamsPollers.groups[poller.key]
	if shared == nil || shared.poller != poller {
		redisStreamsPollers.mu.Unlock()
		return
	}
	shared.refs--
	if shared.refs != 0 {
		redisStreamsPollers.mu.Unlock()
		return
	}
	delete(redisStreamsPollers.groups, poller.key)
	if len(redisStreamsPollers.groups) == 0 {
		redisStreamsPollers.groups = nil
	}
	redisStreamsPollers.mu.Unlock()
	poller.cancel()
}

func (p *redisStreamsPoller) poll() {
	defer close(p.done)
	readArgs := &rds.XReadArgs{
		Streams: []string{p.key.streamName},
		ID:      "$",
		Count:   p.key.config.readCount,
		Block:   p.key.config.block,
	}

	for p.ctx.Err() == nil {
		response, err := p.client.XRead(p.ctx, readArgs).Result()
		if err != nil {
			if p.ctx.Err() != nil {
				return
			}
			if errors.Is(err, rds.Nil) {
				continue
			}
			redisStreamsLog.Debug("error reading from stream: %s", err.Error())
			timer := time.NewTimer(time.Second)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}

		for _, stream := range response {
			for _, entry := range stream.Messages {
				message := RawClusterMessage(entry.Values)
				if adapter, ok := p.adapters.Load(message.Nsp()); ok && adapter.ctx.Err() == nil {
					if err := adapter.OnRawMessage(message, entry.ID); err != nil {
						redisStreamsLog.Debug("error processing message: %s", err.Error())
					}
				}
				readArgs.ID = entry.ID
			}
		}
	}
}
