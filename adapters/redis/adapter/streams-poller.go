package adapter

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

type sharedRedisStreamsPoller struct {
	poller        *redisStreamsPoller
	registrations map[string][]*redisStreamsAdapter
	refs          int
}

var redisStreamsPollers struct {
	mu     sync.Mutex
	groups map[redisStreamsPollerKey]*sharedRedisStreamsPoller
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
	shared := redisStreamsPollers.groups[key]
	created := shared == nil
	if shared == nil {
		ctx, cancel := context.WithCancel(adapter.redisClient.Context())
		shared = &sharedRedisStreamsPoller{poller: &redisStreamsPoller{
			key:    key,
			ctx:    ctx,
			cancel: cancel,
			client: adapter.redisClient.Sub(),
			ready:  make(chan struct{}),
			done:   make(chan struct{}),
		}, registrations: make(map[string][]*redisStreamsAdapter)}
		if redisStreamsPollers.groups == nil {
			redisStreamsPollers.groups = make(map[redisStreamsPollerKey]*sharedRedisStreamsPoller)
		}
		redisStreamsPollers.groups[key] = shared
	}
	nsp := adapter.Nsp().Name()
	shared.refs++
	shared.registrations[nsp] = append(shared.registrations[nsp], adapter)
	shared.poller.adapters.Store(nsp, adapter)
	redisStreamsPollers.mu.Unlock()

	if created {
		startID, err := shared.poller.readInitialID()
		if err != nil && shared.poller.ctx.Err() == nil {
			redisStreamsLog.Debug("error reading stream tail: %s", err.Error())
			adapter.redisClient.Emit("error", err)
		}
		go shared.poller.poll(startID, err == nil)
		close(shared.poller.ready)
	} else {
		<-shared.poller.ready
	}
	return shared.poller
}

func releaseRedisStreamsPoller(poller *redisStreamsPoller, adapter *redisStreamsAdapter) {
	redisStreamsPollers.mu.Lock()
	shared := redisStreamsPollers.groups[poller.key]
	if shared == nil || shared.poller != poller {
		redisStreamsPollers.mu.Unlock()
		return
	}

	nsp := adapter.Nsp().Name()
	registrations := shared.registrations[nsp]
	registrationIndex := -1
	for i, registration := range slices.Backward(registrations) {
		if registration == adapter {
			registrationIndex = i
			break
		}
	}
	if registrationIndex == -1 {
		redisStreamsPollers.mu.Unlock()
		return
	}
	registrations = append(registrations[:registrationIndex], registrations[registrationIndex+1:]...)
	if len(registrations) == 0 {
		delete(shared.registrations, nsp)
		poller.adapters.Delete(nsp)
	} else {
		shared.registrations[nsp] = registrations
		poller.adapters.Store(nsp, registrations[len(registrations)-1])
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

func (p *redisStreamsPoller) readInitialID() (string, error) {
	entries, err := p.client.XRevRangeN(p.ctx, p.key.streamName, "+", "-", 1).Result()
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
