package adapter

import (
	"context"
	"sync"
	"time"

	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type valkeyStreamsPollerConfig struct {
	readCount int64
	block     time.Duration
}

type valkeyStreamsPollerKey struct {
	client     *valkey.ValkeyClient
	streamName string
	config     valkeyStreamsPollerConfig
}

type valkeyStreamsPoller struct {
	key      valkeyStreamsPollerKey
	ctx      context.Context
	cancel   context.CancelFunc
	adapters types.Map[string, *valkeyStreamsAdapter]
	ready    chan struct{}
}

var valkeyStreamsPollers struct {
	mu     sync.Mutex
	groups map[valkeyStreamsPollerKey]*valkeyStreamsPoller
}

func acquireValkeyStreamsPoller(a *valkeyStreamsAdapter) {
	key := valkeyStreamsPollerKey{
		client:     a.valkeyClient,
		streamName: a.streamName,
		config: valkeyStreamsPollerConfig{
			readCount: a.opts.ReadCount(),
			block:     time.Duration(a.opts.BlockTimeInMs()) * time.Millisecond,
		},
	}

	valkeyStreamsPollers.mu.Lock()
	poller := valkeyStreamsPollers.groups[key]
	created := poller == nil
	if created {
		ctx, cancel := context.WithCancel(a.valkeyClient.Context())
		poller = &valkeyStreamsPoller{
			key:    key,
			ctx:    ctx,
			cancel: cancel,
			ready:  make(chan struct{}),
		}
		if valkeyStreamsPollers.groups == nil {
			valkeyStreamsPollers.groups = make(map[valkeyStreamsPollerKey]*valkeyStreamsPoller)
		}
		valkeyStreamsPollers.groups[key] = poller
	}
	nsp := a.Nsp().Name()
	poller.adapters.Store(nsp, a)
	valkeyStreamsPollers.mu.Unlock()
	a.streamPoller = poller

	if created {
		startID, err := poller.readInitialID()
		go poller.poll(startID, err == nil)
		close(poller.ready)
		if err != nil && poller.ctx.Err() == nil {
			valkeyStreamsLog.Debug("error reading stream tail: %s", err.Error())
			a.valkeyClient.Emit("error", err)
		}
	} else {
		select {
		case <-poller.ready:
		case <-a.ctx.Done():
		}
	}
}

func releaseValkeyStreamsPoller(poller *valkeyStreamsPoller, a *valkeyStreamsAdapter) {
	valkeyStreamsPollers.mu.Lock()
	nsp := a.Nsp().Name()
	if !poller.adapters.CompareAndDelete(nsp, a) {
		valkeyStreamsPollers.mu.Unlock()
		return
	}
	if poller.adapters.Len() != 0 {
		valkeyStreamsPollers.mu.Unlock()
		return
	}
	delete(valkeyStreamsPollers.groups, poller.key)
	if len(valkeyStreamsPollers.groups) == 0 {
		valkeyStreamsPollers.groups = nil
	}
	valkeyStreamsPollers.mu.Unlock()
	poller.cancel()
}

func (p *valkeyStreamsPoller) readInitialID() (string, error) {
	entries, err := p.key.client.XRevRangeN(p.ctx, p.key.streamName, "+", "-", 1)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "0-0", nil
	}
	return entries[0].ID, nil
}

func (p *valkeyStreamsPoller) retryInitialID() (string, bool) {
	for p.ctx.Err() == nil {
		startID, err := p.readInitialID()
		if err == nil {
			return startID, true
		}
		valkeyStreamsLog.Debug("error reading stream tail: %s", err.Error())
		if !waitValkeyPollerRetry(p.ctx) {
			return "", false
		}
	}
	return "", false
}

func waitValkeyPollerRetry(ctx context.Context) bool {
	timer := time.NewTimer(time.Second)
	select {
	case <-ctx.Done():
		timer.Stop()
		return false
	case <-timer.C:
		return true
	}
}

func (p *valkeyStreamsPoller) poll(offset string, initialized bool) {
	if !initialized {
		var ok bool
		offset, ok = p.retryInitialID()
		if !ok {
			return
		}
	}

	for p.ctx.Err() == nil {
		entries, err := p.key.client.XRead(
			p.ctx,
			p.key.streamName,
			offset,
			p.key.config.readCount,
			p.key.config.block,
		)
		if err != nil {
			if p.ctx.Err() != nil {
				return
			}
			valkeyStreamsLog.Debug("error reading from stream: %s", err.Error())
			if !waitValkeyPollerRetry(p.ctx) {
				return
			}
			continue
		}

		for _, entry := range entries {
			raw := RawClusterMessage(entry.FieldValues)
			if a, ok := p.adapters.Load(raw.Nsp()); ok && a.ctx.Err() == nil {
				if err := a.OnRawMessage(raw, entry.ID); err != nil {
					valkeyStreamsLog.Debug("error processing message: %s", err.Error())
				}
			}
			offset = entry.ID
		}
	}
}
