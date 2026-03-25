// Package adapter provides a cache-backed adapter for Socket.IO clustering.
// It supports Redis, Valkey, and any other backend that implements cache.CacheClient.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var cacheLog = log.NewLog("socket.io-cache")

const (
	subKeyPattern = "psub"
	subKeyChannel = "sub"

	defaultChannelPrefix = "socket.io"
	defaultUidLength     = 6
)

type (
	// CacheAdapterBuilder builds a CacheAdapter for a given namespace.
	CacheAdapterBuilder struct {
		// Cache is the cache client used for pub/sub communication.
		Cache cache.CacheClient
		// Opts contains configuration options for the adapter.
		Opts CacheAdapterOptionsInterface
	}

	cacheAdapter struct {
		socket.Adapter

		cacheClient cache.CacheClient
		opts        *CacheAdapterOptions

		uid                              adapter.ServerId
		requestsTimeout                  time.Duration
		publishOnSpecificResponseChannel bool
		parser                           cache.Parser

		channel                 string
		requestChannel          string
		responseChannel         string
		specificResponseChannel string

		requests             *types.Map[string, *CacheRequest]
		ackRequests          *types.Map[string, *AckRequest]
		cacheListeners       *types.Map[string, cache.CacheSubscription]
		friendlyErrorHandler types.EventListener
	}
)

// New creates a new CacheAdapter for the given namespace.
// Implements the socket.AdapterBuilder interface.
func (cb *CacheAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewCacheAdapter(nsp, cb.Cache, cb.Opts)
}

// MakeCacheAdapter creates an uninitialized cacheAdapter. Call Construct to finish setup.
func MakeCacheAdapter() CacheAdapter {
	c := &cacheAdapter{
		Adapter: socket.MakeAdapter(),

		opts:           DefaultCacheAdapterOptions(),
		requests:       &types.Map[string, *CacheRequest]{},
		ackRequests:    &types.Map[string, *AckRequest]{},
		cacheListeners: &types.Map[string, cache.CacheSubscription]{},
	}
	c.Prototype(c)
	return c
}

// NewCacheAdapter creates and fully initializes a CacheAdapter.
func NewCacheAdapter(nsp socket.Namespace, client cache.CacheClient, opts any) CacheAdapter {
	c := MakeCacheAdapter()
	c.SetCache(client)
	c.SetOpts(opts)
	c.Construct(nsp)
	return c
}

// SetCache sets the cache client.
func (r *cacheAdapter) SetCache(client cache.CacheClient) { r.cacheClient = client }

// SetOpts applies options; non-CacheAdapterOptionsInterface values are ignored.
func (r *cacheAdapter) SetOpts(opts any) {
	if o, ok := opts.(CacheAdapterOptionsInterface); ok {
		r.opts.Assign(o)
	}
}

func (r *cacheAdapter) Uid() adapter.ServerId           { return r.uid }
func (r *cacheAdapter) RequestsTimeout() time.Duration  { return r.requestsTimeout }
func (r *cacheAdapter) PublishOnSpecificResponseChannel() bool {
	return r.publishOnSpecificResponseChannel
}
func (r *cacheAdapter) Parser() cache.Parser { return r.parser }

// Construct initialises the adapter: generates a UID, resolves configuration
// defaults, derives channel names, and starts pub/sub listener goroutines.
func (r *cacheAdapter) Construct(nsp socket.Namespace) {
	r.Adapter.Construct(nsp)

	uid, _ := adapter.Uid2(defaultUidLength)
	r.uid = adapter.ServerId(uid)

	if r.opts.GetRawRequestsTimeout() != nil {
		r.requestsTimeout = r.opts.RequestsTimeout()
	} else {
		r.requestsTimeout = DefaultRequestsTimeout
	}

	r.publishOnSpecificResponseChannel = r.opts.PublishOnSpecificResponseChannel()

	if r.opts.Parser() != nil {
		r.parser = r.opts.Parser()
	} else {
		r.parser = utils.MsgPack()
	}

	prefix := defaultChannelPrefix
	if r.opts.GetRawKey() != nil {
		prefix = r.opts.Key()
	}

	r.channel = prefix + "#" + nsp.Name() + "#"
	r.requestChannel = prefix + "-request#" + r.Nsp().Name() + "#"
	r.responseChannel = prefix + "-response#" + r.Nsp().Name() + "#"
	r.specificResponseChannel = r.responseChannel + string(r.uid) + "#"

	r.friendlyErrorHandler = func(...any) {
		if r.cacheClient.ListenerCount("error") == 1 {
			cacheLog.Warning("missing 'error' handler on this cache client")
		}
	}
	_ = r.cacheClient.On("error", r.friendlyErrorHandler)

	ctx := r.cacheClient.Context()

	psub := r.cacheClient.PSubscribe(ctx, r.channel+"*")
	r.cacheListeners.Store(subKeyPattern, psub)
	go r.handlePatternMessages(psub)

	sub := r.cacheClient.Subscribe(ctx, r.requestChannel, r.responseChannel, r.specificResponseChannel)
	r.cacheListeners.Store(subKeyChannel, sub)
	go r.handleChannelMessages(sub)
}

// handlePatternMessages delivers messages from the pattern subscription.
func (r *cacheAdapter) handlePatternMessages(sub cache.CacheSubscription) {
	ctx := r.cacheClient.Context()
	for {
		select {
		case msg, ok := <-sub.C():
			if !ok {
				return
			}
			r.onMessage(msg.Pattern, msg.Channel, msg.Payload)
		case <-ctx.Done():
			return
		}
	}
}

// handleChannelMessages delivers messages from the channel subscription.
func (r *cacheAdapter) handleChannelMessages(sub cache.CacheSubscription) {
	ctx := r.cacheClient.Context()
	for {
		select {
		case msg, ok := <-sub.C():
			if !ok {
				return
			}
			r.onRequest(msg.Channel, msg.Payload)
		case <-ctx.Done():
			return
		}
	}
}

// onMessage handles broadcast messages received via pattern subscription.
func (r *cacheAdapter) onMessage(_ string, channel string, msg []byte) {
	if len(channel) <= len(r.channel) || !strings.HasPrefix(channel, r.channel) {
		cacheLog.Debug("ignore channel: shorter than expected or prefix mismatch")
		return
	}

	room := channel[len(r.channel) : len(channel)-1]
	if room != "" && !r.hasRoom(socket.Room(room)) {
		cacheLog.Debug("ignore unknown room %s", room)
		return
	}

	var packet *Packet
	if err := r.parser.Decode(msg, &packet); err != nil {
		cacheLog.Debug("error decoding message: %v", err)
		return
	}

	if r.uid == packet.Uid {
		cacheLog.Debug("ignore same uid")
		return
	}
	if packet.Packet != nil && packet.Packet.Nsp == "" {
		packet.Packet.Nsp = "/"
	}
	if packet.Packet == nil || packet.Packet.Nsp != r.Nsp().Name() {
		cacheLog.Debug("ignore different namespace")
		return
	}
	r.Adapter.Broadcast(packet.Packet, adapter.DecodeOptions(packet.Opts))
}

func (r *cacheAdapter) hasRoom(room socket.Room) bool {
	_, ok := r.Rooms().Load(room)
	return ok
}

// onRequest routes an incoming channel message to onResponse or handleRequest.
func (r *cacheAdapter) onRequest(channel string, msg []byte) {
	if strings.HasPrefix(channel, r.responseChannel) {
		r.onResponse(channel, msg)
		return
	}
	if !strings.HasPrefix(channel, r.requestChannel) {
		cacheLog.Debug("ignore different channel")
		return
	}

	var request *Request
	if len(msg) > 0 && msg[0] == '{' {
		if err := json.Unmarshal(msg, &request); err != nil {
			cacheLog.Debug("ignoring malformed request")
			return
		}
	} else {
		if err := r.parser.Decode(msg, &request); err != nil {
			cacheLog.Debug("ignoring malformed request")
			return
		}
	}
	cacheLog.Debug("received request %v", request)
	r.handleRequest(request)
}

func (r *cacheAdapter) handleRequest(request *Request) {
	switch request.Type {
	case cache.SOCKETS:
		r.handleSocketsRequest(request)
	case cache.ALL_ROOMS:
		r.handleAllRoomsRequest(request)
	case cache.REMOTE_JOIN:
		r.handleRemoteJoinRequest(request)
	case cache.REMOTE_LEAVE:
		r.handleRemoteLeaveRequest(request)
	case cache.REMOTE_DISCONNECT:
		r.handleRemoteDisconnectRequest(request)
	case cache.REMOTE_FETCH:
		r.handleRemoteFetchRequest(request)
	case cache.SERVER_SIDE_EMIT:
		r.handleServerSideEmitRequest(request)
	case cache.BROADCAST:
		r.handleBroadcastRequest(request)
	default:
		cacheLog.Debug("ignoring unknown request type: %d", request.Type)
	}
}

func (r *cacheAdapter) handleSocketsRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}
	sockets := r.Sockets(types.NewSet(request.Rooms...))
	response, err := json.Marshal(&Response{
		RequestId: request.RequestId,
		Sockets: slices.Map(sockets.Keys(), func(id socket.SocketId) *adapter.SocketResponse {
			return &adapter.SocketResponse{Id: id}
		}),
	})
	if err != nil {
		cacheLog.Debug("Error marshaling SOCKETS response for RequestId %s: %s", request.RequestId, err.Error())
		return
	}
	r.publishResponse(request, response)
}

func (r *cacheAdapter) handleAllRoomsRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}
	response, err := json.Marshal(&Response{
		RequestId: request.RequestId,
		Rooms:     r.Rooms().Keys(),
	})
	if err != nil {
		cacheLog.Debug("Error marshaling ALL_ROOMS response for RequestId %s: %s", request.RequestId, err.Error())
		return
	}
	r.publishResponse(request, response)
}

func (r *cacheAdapter) handleRemoteJoinRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.AddSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
		return
	}
	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Join(request.Room)
		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			cacheLog.Debug("Error marshaling REMOTE_JOIN response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

func (r *cacheAdapter) handleRemoteLeaveRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.DelSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
		return
	}
	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Leave(request.Room)
		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			cacheLog.Debug("Error marshaling REMOTE_LEAVE response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

func (r *cacheAdapter) handleRemoteDisconnectRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.DisconnectSockets(adapter.DecodeOptions(request.Opts), request.Close)
		return
	}
	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Disconnect(request.Close)
		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			cacheLog.Debug("Error marshaling REMOTE_DISCONNECT response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

func (r *cacheAdapter) handleRemoteFetchRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}
	r.Adapter.FetchSockets(adapter.DecodeOptions(request.Opts))(func(localSockets []socket.SocketDetails, e error) {
		if e != nil {
			cacheLog.Debug("REMOTE_FETCH Adapter.FetchSockets error: %s", e.Error())
			return
		}
		response, err := json.Marshal(&Response{
			RequestId: request.RequestId,
			Sockets: slices.Map(localSockets, func(client socket.SocketDetails) *adapter.SocketResponse {
				return &adapter.SocketResponse{
					Id:        client.Id(),
					Handshake: client.Handshake(),
					Rooms:     client.Rooms().Keys(),
					Data:      client.Data(),
				}
			}),
		})
		if err != nil {
			cacheLog.Debug("Error marshaling REMOTE_FETCH response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	})
}

func (r *cacheAdapter) handleServerSideEmitRequest(request *Request) {
	if request.Uid == r.uid {
		cacheLog.Debug("ignore same uid")
		return
	}
	if request.RequestId == "" {
		r.Nsp().OnServerSideEmit(request.Data)
		return
	}

	called := &sync.Once{}
	callback := func(args []any, err error) {
		called.Do(func() {
			cacheLog.Debug("calling acknowledgement with %v", args)
			response, err := json.Marshal(&Response{
				Type:      cache.SERVER_SIDE_EMIT,
				RequestId: request.RequestId,
				Data:      args,
			})
			if err != nil {
				cacheLog.Debug("Error marshaling SERVER_SIDE_EMIT response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			if err := r.cacheClient.Publish(r.cacheClient.Context(), r.responseChannel, response); err != nil {
				r.cacheClient.Emit("error", err)
			}
		})
	}
	r.Nsp().OnServerSideEmit(append(request.Data, callback))
}

func (r *cacheAdapter) handleBroadcastRequest(request *Request) {
	if _, ok := r.ackRequests.Load(request.RequestId); ok {
		return
	}
	r.Adapter.BroadcastWithAck(
		request.Packet,
		adapter.DecodeOptions(request.Opts),
		func(clientCount uint64) {
			cacheLog.Debug("waiting for %d client acknowledgements", clientCount)
			response, err := json.Marshal(&Response{
				Type:        cache.BROADCAST_CLIENT_COUNT,
				RequestId:   request.RequestId,
				ClientCount: clientCount,
			})
			if err != nil {
				cacheLog.Debug("Error marshaling BROADCAST_CLIENT_COUNT response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			r.publishResponse(request, response)
		},
		func(args []any, _ error) {
			cacheLog.Debug("received acknowledgement with value %v", args)
			response, err := r.parser.Encode(&Response{
				Type:      cache.BROADCAST_ACK,
				RequestId: request.RequestId,
				Packet:    args,
			})
			if err != nil {
				cacheLog.Debug("Error marshaling BROADCAST_ACK response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			r.publishResponse(request, response)
		},
	)
}

func (r *cacheAdapter) publishResponse(request *Request, response []byte) {
	var b strings.Builder
	if r.publishOnSpecificResponseChannel {
		b.Grow(len(r.responseChannel) + len(request.Uid) + 1)
		b.WriteString(r.responseChannel)
		b.WriteString(string(request.Uid))
		b.WriteByte('#')
	} else {
		b.WriteString(r.responseChannel)
	}
	channel := b.String()

	cacheLog.Debug("publishing response to channel %s", channel)
	if err := r.cacheClient.Publish(r.cacheClient.Context(), channel, response); err != nil {
		r.cacheClient.Emit("error", err)
	}
}

func (r *cacheAdapter) onResponse(_ string, msg []byte) {
	var response *Response
	if len(msg) > 0 && msg[0] == '{' {
		if err := json.Unmarshal(msg, &response); err != nil {
			cacheLog.Debug("ignoring malformed response")
			return
		}
	} else {
		if err := r.parser.Decode(msg, &response); err != nil {
			cacheLog.Debug("ignoring malformed response")
			return
		}
	}

	requestId := response.RequestId
	if ackRequest, ok := r.ackRequests.Load(requestId); ok {
		switch response.Type {
		case cache.BROADCAST_CLIENT_COUNT:
			ackRequest.ClientCountCallback(response.ClientCount)
		case cache.BROADCAST_ACK:
			ackRequest.Ack(response.Packet, nil)
		}
		return
	}
	request, ok := r.requests.Load(requestId)
	if !ok {
		cacheLog.Debug("ignoring unknown request")
		return
	}
	cacheLog.Debug("received response %v", response)
	r.processResponse(request, response, requestId)
}

func (r *cacheAdapter) processResponse(request *CacheRequest, response *Response, requestId string) {
	switch request.Type {
	case cache.SOCKETS, cache.REMOTE_FETCH:
		request.MsgCount.Add(1)
		if len(response.Sockets) > 0 {
			request.Sockets.Push(response.Sockets...)
		}
		if request.MsgCount.Load() == request.NumSub {
			utils.ClearTimeout(request.Timeout.Load())
			if request.Resolve != nil {
				request.Resolve(types.NewSlice(slices.Map(request.Sockets.All(), func(client *adapter.SocketResponse) any {
					return socket.SocketDetails(adapter.NewRemoteSocket(client))
				})...))
			}
			r.requests.Delete(requestId)
		}
	case cache.ALL_ROOMS:
		request.MsgCount.Add(1)
		if len(response.Rooms) > 0 {
			request.Rooms.Add(response.Rooms...)
		}
		if request.MsgCount.Load() == request.NumSub {
			utils.ClearTimeout(request.Timeout.Load())
			if request.Resolve != nil {
				request.Resolve(types.NewSlice(slices.Map(request.Rooms.Keys(), func(room socket.Room) any {
					return room
				})...))
			}
			r.requests.Delete(requestId)
		}
	case cache.REMOTE_JOIN, cache.REMOTE_LEAVE, cache.REMOTE_DISCONNECT:
		utils.ClearTimeout(request.Timeout.Load())
		if request.Resolve != nil {
			request.Resolve(nil)
		}
		r.requests.Delete(requestId)
	case cache.SERVER_SIDE_EMIT:
		request.Responses.Push(response.Data)
		cacheLog.Debug("serverSideEmit: got %d responses out of %d", request.Responses.Len(), request.NumSub)
		if int64(request.Responses.Len()) == request.NumSub {
			utils.ClearTimeout(request.Timeout.Load())
			if request.Resolve != nil {
				request.Resolve(request.Responses)
			}
			r.requests.Delete(requestId)
		}
	default:
		cacheLog.Debug("ignoring unknown request type: %d", request.Type)
	}
}

// Broadcast sends a packet to all matching clients and propagates it to peer nodes.
func (r *cacheAdapter) Broadcast(packet *parser.Packet, opts *socket.BroadcastOptions) {
	packet.Nsp = r.Nsp().Name()
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		msg, err := r.parser.Encode(&Packet{
			Uid:    r.Uid(),
			Packet: packet,
			Opts:   adapter.EncodeOptions(opts),
		})
		if err == nil {
			channel := r.channel
			if opts.Rooms != nil && opts.Rooms.Len() == 1 {
				for _, room := range opts.Rooms.Keys() {
					channel += string(room) + "#"
					break
				}
			}
			cacheLog.Debug("publishing message to channel %s", channel)
			if err := r.cacheClient.Publish(r.cacheClient.Context(), channel, msg); err != nil {
				r.cacheClient.Emit("error", err)
			}
		}
	}
	r.Adapter.Broadcast(packet, opts)
}

// BroadcastWithAck broadcasts a packet and collects acknowledgements from all nodes.
func (r *cacheAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	packet.Nsp = r.Nsp().Name()
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		if requestId, err := adapter.Uid2(defaultUidLength); err == nil {
			if request, err := r.parser.Encode(&Request{
				Uid:       r.uid,
				RequestId: requestId,
				Type:      cache.BROADCAST,
				Packet:    packet,
				Opts:      adapter.EncodeOptions(opts),
			}); err == nil {
				if err := r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request); err != nil {
					r.cacheClient.Emit("error", err)
				}

				r.ackRequests.Store(requestId, &AckRequest{
					ClientCountCallback: clientCountCallback,
					Ack:                 ack,
				})

				timeout := time.Duration(0)
				if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
					timeout = *opts.Flags.Timeout
				}
				utils.SetTimeout(func() {
					r.ackRequests.Delete(requestId)
				}, timeout)
			}
		}
	}
	r.Adapter.BroadcastWithAck(packet, opts, clientCountCallback, ack)
}

// AllRooms collects rooms across all cluster nodes.
func (r *cacheAdapter) AllRooms() func(func(*types.Set[socket.Room], error)) {
	return func(cb func(*types.Set[socket.Room], error)) {
		localRooms := types.NewSet(r.Rooms().Keys()...)
		numSub := r.ServerCount()
		cacheLog.Debug(`waiting for %d responses to "allRooms" request`, numSub)
		if numSub <= 1 {
			cb(localRooms, nil)
			return
		}

		requestId, err := adapter.Uid2(defaultUidLength)
		if err != nil {
			cb(nil, err)
			return
		}

		request, err := json.Marshal(&Request{Type: cache.ALL_ROOMS, Uid: r.uid, RequestId: requestId})
		if err != nil {
			cb(nil, err)
			return
		}

		timeout := utils.SetTimeout(func() {
			if _, ok := r.requests.Load(requestId); ok {
				cb(nil, errors.New("timeout reached while waiting for allRooms response"))
				r.requests.Delete(requestId)
			}
		}, r.requestsTimeout)

		r.requests.Store(requestId, &CacheRequest{
			Type:   cache.ALL_ROOMS,
			NumSub: numSub,
			Resolve: func(data *types.Slice[any]) {
				cb(types.NewSet(slices.Map(data.All(), func(room any) socket.Room {
					return utils.TryCast[socket.Room](room)
				})...), nil)
			},
			Timeout: utils.Tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
				t.Store(timeout)
			}),
			MsgCount: utils.Tap(&atomic.Int64{}, func(c *atomic.Int64) {
				c.Store(1)
			}),
			Rooms: localRooms,
		})

		if err := r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request); err != nil {
			r.cacheClient.Emit("error", err)
		}
	}
}

// FetchSockets retrieves all sockets across the cluster.
func (r *cacheAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(cb func([]socket.SocketDetails, error)) {
		r.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, _ error) {
			if opts.Flags != nil && opts.Flags.Local {
				cb(localSockets, nil)
				return
			}

			numSub := r.ServerCount()
			cacheLog.Debug(`waiting for %d responses to "fetchSockets" request`, numSub)

			if numSub <= 1 {
				cb(localSockets, nil)
				return
			}

			requestId, err := adapter.Uid2(defaultUidLength)
			if err != nil {
				cb(nil, err)
				return
			}

			request, err := json.Marshal(&Request{Type: cache.REMOTE_FETCH, Uid: r.uid, RequestId: requestId, Opts: adapter.EncodeOptions(opts)})
			if err != nil {
				cb(nil, err)
				return
			}

			timeout := utils.SetTimeout(func() {
				if _, ok := r.requests.Load(requestId); ok {
					cb(nil, errors.New("timeout reached while waiting for fetchSockets response"))
					r.requests.Delete(requestId)
				}
			}, r.requestsTimeout)

			r.requests.Store(requestId, &CacheRequest{
				Type:   cache.REMOTE_FETCH,
				NumSub: numSub,
				Resolve: func(data *types.Slice[any]) {
					cb(slices.Map(data.All(), func(i any) socket.SocketDetails {
						return utils.TryCast[socket.SocketDetails](i)
					}), nil)
				},
				Timeout: utils.Tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
					t.Store(timeout)
				}),
				MsgCount: utils.Tap(&atomic.Int64{}, func(c *atomic.Int64) {
					c.Store(1)
				}),
				Sockets: types.NewSlice(slices.Map(localSockets, func(client socket.SocketDetails) *adapter.SocketResponse {
					return &adapter.SocketResponse{
						Id:        client.Id(),
						Handshake: client.Handshake(),
						Rooms:     client.Rooms().Keys(),
						Data:      client.Data(),
					}
				})...),
			})

			if err := r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request); err != nil {
				r.cacheClient.Emit("error", err)
			}
		})
	}
}

// AddSockets adds sockets to rooms across all cluster nodes.
func (r *cacheAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.AddSockets(opts, rooms)
		return
	}
	request, err := json.Marshal(&Request{Uid: r.uid, Type: cache.REMOTE_JOIN, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
	if err != nil {
		cacheLog.Debug("Error marshaling AddSockets request: %s", err.Error())
		return
	}
	if err := r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request); err != nil {
		r.cacheClient.Emit("error", err)
	}
}

// DelSockets removes sockets from rooms across all cluster nodes.
func (r *cacheAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.DelSockets(opts, rooms)
		return
	}
	request, err := json.Marshal(&Request{Uid: r.uid, Type: cache.REMOTE_LEAVE, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
	if err != nil {
		cacheLog.Debug("Error marshaling DelSockets request: %s", err.Error())
		return
	}
	if err := r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request); err != nil {
		r.cacheClient.Emit("error", err)
	}
}

// DisconnectSockets disconnects sockets across all cluster nodes.
func (r *cacheAdapter) DisconnectSockets(opts *socket.BroadcastOptions, close bool) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.DisconnectSockets(opts, close)
		return
	}
	request, err := json.Marshal(&Request{Uid: r.uid, Type: cache.REMOTE_DISCONNECT, Opts: adapter.EncodeOptions(opts), Close: close})
	if err != nil {
		cacheLog.Debug("Error marshaling DisconnectSockets request: %s", err.Error())
		return
	}
	if err := r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request); err != nil {
		r.cacheClient.Emit("error", err)
	}
}

// ServerSideEmit emits an event to all server nodes in the cluster.
func (r *cacheAdapter) ServerSideEmit(packet []any) error {
	if len(packet) == 0 {
		return errors.New("packet cannot be empty")
	}
	if ack, withAck := packet[len(packet)-1].(socket.Ack); withAck {
		return r.serverSideEmitWithAck(packet[:len(packet)-1], ack)
	}

	request, err := json.Marshal(&Request{Uid: r.uid, Type: cache.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal ServerSideEmit request: %w", err)
	}
	return r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request)
}

func (r *cacheAdapter) serverSideEmitWithAck(packet []any, ack socket.Ack) error {
	numSub := r.ServerCount() - 1
	cacheLog.Debug(`waiting for %d responses to "serverSideEmit" request`, numSub)
	if numSub <= 0 {
		ack(nil, nil)
		return nil
	}

	requestId, err := adapter.Uid2(defaultUidLength)
	if err != nil {
		return fmt.Errorf("failed to generate request ID: %w", err)
	}

	request, err := json.Marshal(&Request{Uid: r.uid, RequestId: requestId, Type: cache.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal serverSideEmitWithAck request: %w", err)
	}

	timeout := utils.SetTimeout(func() {
		if storedRequest, ok := r.requests.Load(requestId); ok {
			ack(storedRequest.Responses.All(), fmt.Errorf("timeout reached: only %d responses received out of %d", storedRequest.Responses.Len(), storedRequest.NumSub))
			r.requests.Delete(requestId)
		}
	}, r.requestsTimeout)

	r.requests.Store(requestId, &CacheRequest{
		Type:   cache.SERVER_SIDE_EMIT,
		NumSub: numSub,
		Timeout: utils.Tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
			t.Store(timeout)
		}),
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Responses: types.NewSlice[any](),
	})

	return r.cacheClient.Publish(r.cacheClient.Context(), r.requestChannel, request)
}

// ServerCount returns the number of nodes subscribed to the request channel.
func (r *cacheAdapter) ServerCount() int64 {
	result, err := r.cacheClient.PubSubNumSub(r.cacheClient.Context(), r.requestChannel)
	if err != nil {
		r.cacheClient.Emit("error", err)
		return 0
	}
	if count, ok := result[r.requestChannel]; ok {
		return count
	}
	return 0
}

// Close unsubscribes from all channels and removes the error listener.
func (r *cacheAdapter) Close() {
	ctx := r.cacheClient.Context()
	if psub, ok := r.cacheListeners.Load(subKeyPattern); ok {
		if err := psub.PUnsubscribe(ctx, r.channel+"*"); err != nil {
			r.cacheClient.Emit("error", err)
		}
	}
	if sub, ok := r.cacheListeners.Load(subKeyChannel); ok {
		if err := sub.Unsubscribe(ctx, r.requestChannel, r.responseChannel, r.specificResponseChannel); err != nil {
			r.cacheClient.Emit("error", err)
		}
	}
	r.cacheClient.RemoveListener("error", r.friendlyErrorHandler)
}
