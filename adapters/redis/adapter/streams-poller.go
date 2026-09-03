package adapter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

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
	ready    chan struct{}
	done     chan struct{}
}

var redisStreamsPollers struct {
	mu     sync.Mutex
	groups map[redisStreamsPollerKey]*redisStreamsPoller
}

func redisStreamsPollerBlock(blockTimeInMs int64) (time.Duration, error) {
	if blockTimeInMs <= 0 || blockTimeInMs > DefaultBlockTimeInMs {
		return 0, fmt.Errorf(
			"redis streams: blockTimeInMs must be between 1 and %d, got %d",
			DefaultBlockTimeInMs,
			blockTimeInMs,
		)
	}
	return time.Duration(blockTimeInMs) * time.Millisecond, nil
}

func acquireRedisStreamsPoller(adapter *redisStreamsAdapter) *redisStreamsPoller {
	block, err := redisStreamsPollerBlock(adapter.opts.BlockTimeInMs())
	if err != nil {
		redisStreamsLog.Debug("invalid Redis Streams poller configuration: %s", err.Error())
		adapter.redisClient.Emit("error", err)
		block = time.Duration(DefaultBlockTimeInMs) * time.Millisecond
	}
	config := redisStreamsPollerConfig{
		readCount: adapter.opts.ReadCount(),
		block:     block,
	}
	key := redisStreamsPollerKey{
		server:     adapter.server,
		client:     adapter.redisClient,
		streamName: adapter.streamName,
		config:     config,
	}

	redisStreamsPollers.mu.Lock()
	poller := redisStreamsPollers.groups[key]
	created := poller == nil
	if poller == nil {
		ctx, cancel := context.WithCancel(adapter.redisClient.Context())
		poller = &redisStreamsPoller{
			key:    key,
			ctx:    ctx,
			cancel: cancel,
			client: adapter.redisClient.Sub(),
			ready:  make(chan struct{}),
			done:   make(chan struct{}),
		}
		if redisStreamsPollers.groups == nil {
			redisStreamsPollers.groups = make(map[redisStreamsPollerKey]*redisStreamsPoller)
		}
		redisStreamsPollers.groups[key] = poller
	}
	nsp := adapter.Nsp().Name()
	poller.adapters.Store(nsp, adapter)
	redisStreamsPollers.mu.Unlock()

	if created {
		startID, err := poller.readInitialID()
		go poller.poll(startID, err == nil)
		close(poller.ready)
		if err != nil && poller.ctx.Err() == nil {
			redisStreamsLog.Debug("error reading stream tail: %s", err.Error())
			adapter.redisClient.Emit("error", err)
		}
	} else {
		<-poller.ready
	}
	return poller
}

func releaseRedisStreamsPoller(poller *redisStreamsPoller, adapter *redisStreamsAdapter) {
	redisStreamsPollers.mu.Lock()
	if redisStreamsPollers.groups[poller.key] != poller {
		redisStreamsPollers.mu.Unlock()
		return
	}

	nsp := adapter.Nsp().Name()
	if !poller.adapters.CompareAndDelete(nsp, adapter) || poller.adapters.Len() != 0 {
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

func (p *redisStreamsPoller) readInitialID() (string, error) {
	entries, err := p.key.client.Client().XRevRangeN(p.ctx, p.key.streamName, "+", "-", 1).Result()
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "0-0", nil
	}
	return entries[0].ID, nil
}

func (p *redisStreamsPoller) retryInitialID() (string, bool) {
	for p.ctx.Err() == nil {
		startID, err := p.readInitialID()
		if err == nil {
			return startID, true
		}
		if p.ctx.Err() != nil {
			return "", false
		}
		redisStreamsLog.Debug("error reading stream tail: %s", err.Error())
		timer := time.NewTimer(time.Second)
		select {
		case <-p.ctx.Done():
			timer.Stop()
			return "", false
		case <-timer.C:
		}
	}
	return "", false
}

func (p *redisStreamsPoller) poll(startID string, initialized bool) {
	defer close(p.done)
	if !initialized {
		var ok bool
		startID, ok = p.retryInitialID()
		if !ok {
			return
		}
	}
	readArgs := &rds.XReadArgs{
		Streams: []string{p.key.streamName},
		ID:      startID,
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
