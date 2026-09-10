// Package adapter provides a Redis-based adapter implementation for Socket.IO clustering.
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// redisLog is the logger for the Redis adapter.
var redisLog = log.NewLog("socket.io-redis")

var errRedisPublishPanicked = errors.New("redis publish panicked")

const (
	// Default configuration values.
	defaultChannelPrefix = "socket.io"
	defaultUidLength     = 6
)

type (
	// RedisAdapterBuilder builds a RedisAdapter with the given Redis client and options.
	// Use this builder to create adapters for Socket.IO namespaces.
	RedisAdapterBuilder struct {
		// Redis is the Redis client used by the adapter for Pub/Sub communication.
		Redis *redis.RedisClient
		// Opts contains configuration options for the adapter.
		Opts RedisAdapterOptionsInterface
	}

	// redisAdapter implements the Socket.IO Adapter interface using Redis for inter-node communication.
	redisAdapter struct {
		socket.Adapter

		redisClient *redis.RedisClient
		opts        *RedisAdapterOptions

		uid                              adapter.ServerId
		requestsTimeout                  time.Duration
		publishOnSpecificResponseChannel bool
		parser                           redis.Parser

		// Channel names for Redis Pub/Sub.
		channel                 string
		requestChannel          string
		responseChannel         string
		specificResponseChannel string

		// Internal state management.
		requests              types.Map[string, *RedisRequest]
		ackRequests           types.Map[string, *AckRequest]
		publisher             *queue.Queue
		responses             *queue.Queue
		queueMu               sync.Mutex
		pubSub                *redisPubSub
		broadcastSubscription *redisSubscription
		requestSubscription   *redisSubscription
		server                *socket.Server

		ctx       context.Context
		cancel    context.CancelFunc
		closeOnce sync.Once
	}
)

// New creates a new RedisAdapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (rb *RedisAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewRedisAdapter(nsp, rb.Redis, rb.Opts)
}

// MakeRedisAdapter creates a new uninitialized redisAdapter with default options.
// Call Construct() to complete initialization.
func MakeRedisAdapter() RedisAdapter {
	c := &redisAdapter{
		Adapter:   socket.MakeAdapter(),
		opts:      DefaultRedisAdapterOptions(),
		publisher: queue.New(),
		responses: queue.New(),
	}
	c.Prototype(c)

	return c
}

// registerRequest installs a pending request and removes it before invoking its
// timeout callback.
func (r *redisAdapter) registerRequest(requestId string, request *RedisRequest, timeout time.Duration, onTimeout func()) {
	r.requests.Store(requestId, request)
	request.Timeout.Store(utils.SetTimeout(func() {
		if !r.requests.CompareAndDelete(requestId, request) {
			return
		}
		if onTimeout != nil {
			onTimeout()
		}
	}, timeout))
}

// finishRequest claims a completed request, releases its timeout and reports
// whether its callback may be invoked.
func (r *redisAdapter) finishRequest(requestId string, request *RedisRequest) bool {
	if !r.requests.CompareAndDelete(requestId, request) {
		return false
	}
	utils.ClearTimeout(request.Timeout.Swap(nil))
	return request.Resolve != nil
}

// registerAckRequest installs an acknowledgement request. Its timeout is
// derived from the adapter context, so Close also triggers cleanup.
func (r *redisAdapter) registerAckRequest(requestId string, request *AckRequest, timeout time.Duration) {
	r.ackRequests.Store(requestId, request)
	timeoutCtx, cancel := context.WithTimeout(r.ctx, timeout)
	context.AfterFunc(timeoutCtx, func() {
		defer cancel()
		r.ackRequests.CompareAndDelete(requestId, request)
	})
}

// NewRedisAdapter creates and initializes a new RedisAdapter for the given namespace.
// This is the primary constructor for creating Redis adapters.
func NewRedisAdapter(nsp socket.Namespace, redisClient *redis.RedisClient, opts any) RedisAdapter {
	c := MakeRedisAdapter()
	c.SetRedis(redisClient)
	c.SetOpts(opts)
	c.Construct(nsp)
	return c
}

// SetRedis sets the Redis client for the adapter.
func (r *redisAdapter) SetRedis(redisClient *redis.RedisClient) {
	r.redisClient = redisClient
}

// SetOpts sets the options for the adapter.
// Accepts RedisAdapterOptionsInterface; other types are ignored.
func (r *redisAdapter) SetOpts(opts any) {
	if options, ok := opts.(RedisAdapterOptionsInterface); ok {
		r.opts.Assign(options)
	}
}

// Uid returns the unique server ID for this adapter instance.
func (r *redisAdapter) Uid() adapter.ServerId { return r.uid }

// RequestsTimeout returns the configured timeout duration for inter-node requests.
func (r *redisAdapter) RequestsTimeout() time.Duration { return r.requestsTimeout }

// PublishOnSpecificResponseChannel indicates if responses are published on node-specific channels.
func (r *redisAdapter) PublishOnSpecificResponseChannel() bool {
	return r.publishOnSpecificResponseChannel
}

// Parser returns the parser used for encoding/decoding Redis messages.
func (r *redisAdapter) Parser() redis.Parser { return r.parser }

// Construct initializes the Redis adapter for the given namespace.
// It sets up Redis Pub/Sub subscriptions and starts message handling goroutines.
func (r *redisAdapter) Construct(nsp socket.Namespace) {
	r.Adapter.Construct(nsp)

	r.ctx, r.cancel = context.WithCancel(r.redisClient.Context())
	r.server = nsp.Server()

	// Generate unique server ID
	r.uid = adapter.ServerId(adapter.Uid2(defaultUidLength))

	// Node.js uses `opts.requestsTimeout || 5000`.
	r.requestsTimeout = r.opts.RequestsTimeout()
	if r.requestsTimeout == 0 {
		r.requestsTimeout = DefaultRequestsTimeout
	}

	r.publishOnSpecificResponseChannel = r.opts.PublishOnSpecificResponseChannel()

	r.parser = r.opts.Parser()
	if utils.IsNil(r.parser) {
		r.parser = utils.MsgPack()
	}

	// Node.js uses `opts.key || "socket.io"`.
	prefix := utils.Value(r.opts.Key(), defaultChannelPrefix)

	r.channel = prefix + "#" + nsp.Name() + "#"
	r.requestChannel = prefix + "-request#" + r.Nsp().Name() + "#"
	r.responseChannel = prefix + "-response#" + r.Nsp().Name() + "#"
	r.specificResponseChannel = r.responseChannel + string(r.uid) + "#"

	// A connection error handler may call Close before acquisition returns.
	r.queueMu.Lock()
	r.pubSub = acquireRedisPubSub(r.server, r.redisClient)
	r.broadcastSubscription = r.pubSub.newSubscription(r.onMessage)
	r.requestSubscription = r.pubSub.newSubscription(r.onRequest)
	r.queueMu.Unlock()
	r.broadcastSubscription.PSubscribe(r.channel + "*")
	r.requestSubscription.Subscribe(r.requestChannel, r.responseChannel, r.specificResponseChannel)
	context.AfterFunc(r.ctx, r.Close)
	if r.ctx.Err() != nil {
		r.Close()
		return
	}
	if err := r.pubSub.flush(r.ctx); err != nil && r.ctx.Err() == nil {
		r.redisClient.Emit("error", err)
	}
}

// onMessage handles broadcast messages from Redis pattern subscriptions.
func (r *redisAdapter) onMessage(msg []byte, channel string) {
	if !strings.HasPrefix(channel, r.channel) {
		redisLog.Debug("ignore different channel")
		return
	}

	// Extract room from channel name
	room := ""
	if len(channel) > len(r.channel) {
		room = channel[len(r.channel) : len(channel)-1]
	}
	if room != "" && !r.hasRoom(socket.Room(room)) {
		redisLog.Debug("ignore unknown room %s", room)
		return
	}

	var packet Packet
	if err := r.parser.Decode(msg, &packet); err != nil {
		redisLog.Debug("error decoding message: %v", err)
		return
	}

	// Ignore messages from self
	if r.uid == packet.Uid {
		redisLog.Debug("ignore same uid")
		return
	}
	// Ensure namespace is set
	if packet.Packet != nil && packet.Packet.Nsp == "" {
		packet.Packet.Nsp = "/"
	}
	// Validate namespace
	if packet.Packet == nil || packet.Packet.Nsp != r.Nsp().Name() {
		redisLog.Debug("ignore different namespace")
		return
	}
	if !packet.Opts.IsValid() {
		redisLog.Debug("ignoring malformed broadcast options")
		return
	}
	r.Adapter.Broadcast(packet.Packet, adapter.DecodeOptions(packet.Opts))
}

// hasRoom checks if the adapter has the specified room.
func (r *redisAdapter) hasRoom(room socket.Room) bool {
	_, ok := r.Rooms().Load(room)
	return ok
}

// onRequest handles inter-node requests from Redis.
func (r *redisAdapter) onRequest(msg []byte, channel string) {
	// Route response messages to onResponse handler
	if strings.HasPrefix(channel, r.responseChannel) {
		r.onResponse(msg)
		return
	}
	// Validate request channel
	if !strings.HasPrefix(channel, r.requestChannel) {
		redisLog.Debug("ignore different channel")
		return
	}

	var request Request
	// Detect message format by first byte
	var err error
	if len(msg) > 0 && msg[0] == '{' {
		err = json.Unmarshal(msg, &request)
	} else {
		err = r.parser.Decode(msg, &request)
	}
	if err != nil {
		redisLog.Debug("ignoring malformed request")
		return
	}
	redisLog.Debug("received request type %d with id %s", request.Type, request.RequestId)
	if request.Uid != "" && request.Uid == r.uid {
		redisLog.Debug("ignore same uid")
		return
	}
	r.handleRequest(&request)
}

// handleRequest dispatches a request to the appropriate handler based on type.
func (r *redisAdapter) handleRequest(request *Request) {
	switch request.Type {
	case redis.SOCKETS:
		r.handleSocketsRequest(request)
	case redis.ALL_ROOMS:
		r.handleAllRoomsRequest(request)
	case redis.REMOTE_JOIN:
		r.handleRemoteJoinRequest(request)
	case redis.REMOTE_LEAVE:
		r.handleRemoteLeaveRequest(request)
	case redis.REMOTE_DISCONNECT:
		r.handleRemoteDisconnectRequest(request)
	case redis.REMOTE_FETCH:
		r.handleRemoteFetchRequest(request)
	case redis.SERVER_SIDE_EMIT:
		r.handleServerSideEmitRequest(request)
	case redis.BROADCAST:
		r.handleBroadcastRequest(request)
	default:
		redisLog.Debug("ignoring unknown request type: %d", request.Type)
	}
}

// handleSocketsRequest handles SOCKETS request type.
func (r *redisAdapter) handleSocketsRequest(request *Request) {
	sockets := r.Sockets(types.NewSet(request.Rooms...))
	r.publishJSONResponse(request, &Response{
		RequestId: request.RequestId,
		Sockets:   utils.NonNilSlice(sockets.Keys()),
	})
}

// handleAllRoomsRequest handles ALL_ROOMS request type.
func (r *redisAdapter) handleAllRoomsRequest(request *Request) {
	r.publishJSONResponse(request, &Response{
		RequestId: request.RequestId,
		Rooms:     utils.NonNilSlice(r.Rooms().Keys()),
	})
}

// handleRemoteJoinRequest handles REMOTE_JOIN request type.
func (r *redisAdapter) handleRemoteJoinRequest(request *Request) {
	if !request.Opts.IsValid() {
		redisLog.Debug("ignoring malformed REMOTE_JOIN request")
		return
	}
	r.Adapter.AddSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
}

// handleRemoteLeaveRequest handles REMOTE_LEAVE request type.
func (r *redisAdapter) handleRemoteLeaveRequest(request *Request) {
	if !request.Opts.IsValid() {
		redisLog.Debug("ignoring malformed REMOTE_LEAVE request")
		return
	}
	r.Adapter.DelSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
}

// handleRemoteDisconnectRequest handles REMOTE_DISCONNECT request type.
func (r *redisAdapter) handleRemoteDisconnectRequest(request *Request) {
	if !request.Opts.IsValid() {
		redisLog.Debug("ignoring malformed REMOTE_DISCONNECT request")
		return
	}
	r.Adapter.DisconnectSockets(adapter.DecodeOptions(request.Opts), utils.FromPtr(request.Close))
}

// handleRemoteFetchRequest handles REMOTE_FETCH request type.
func (r *redisAdapter) handleRemoteFetchRequest(request *Request) {
	if !request.Opts.IsValid() {
		redisLog.Debug("ignoring malformed REMOTE_FETCH request")
		return
	}
	r.Adapter.FetchSockets(adapter.DecodeOptions(request.Opts))(func(localSockets []socket.SocketDetails, err error) {
		if err != nil {
			redisLog.Debug("REMOTE_FETCH Adapter.FetchSockets error: %s", err.Error())
			return
		}
		r.publishJSONResponse(request, &Response{
			RequestId: request.RequestId,
			Sockets:   adapter.SocketDetailsToResponses(localSockets),
		})
	})
}

// handleServerSideEmitRequest handles SERVER_SIDE_EMIT request type.
func (r *redisAdapter) handleServerSideEmitRequest(request *Request) {
	// No acknowledgement needed
	if request.RequestId == "" {
		r.Nsp().OnServerSideEmit(request.Data)
		return
	}

	// Handle with acknowledgement
	var called atomic.Bool
	callback := func(args []any, _ error) {
		if !called.CompareAndSwap(false, true) {
			return
		}
		redisLog.Debug("calling acknowledgement with %v", args)
		response, err := json.Marshal(&Response{
			Type:      redis.SERVER_SIDE_EMIT,
			RequestId: request.RequestId,
			Data:      slices.TryGet(args, 0),
		})
		if err != nil {
			redisLog.Debug("Error marshaling SERVER_SIDE_EMIT response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponseMessage(r.responseChannel, response)
	}
	r.Nsp().OnServerSideEmit(slices.AppendCopy(request.Data, callback))
}

// handleBroadcastRequest handles BROADCAST request type.
func (r *redisAdapter) handleBroadcastRequest(request *Request) {
	if request.Uid == "" || request.RequestId == "" || request.Packet == nil || !request.Opts.IsValid() {
		redisLog.Debug("ignoring malformed BROADCAST request")
		return
	}
	r.Adapter.BroadcastWithAck(
		request.Packet,
		adapter.DecodeOptions(request.Opts),
		func(clientCount uint64) {
			redisLog.Debug("waiting for %d client acknowledgements", clientCount)
			r.publishJSONResponse(request, &Response{
				Type:        redis.BROADCAST_CLIENT_COUNT,
				RequestId:   request.RequestId,
				ClientCount: &clientCount,
			})
		},
		func(args []any, _ error) {
			redisLog.Debug("received acknowledgement with value %v", args)
			packet, err := redis.NormalizeData(slices.TryGet(args, 0))
			if err != nil {
				redisLog.Debug("Error preparing BROADCAST_ACK response: %s", err)
				return
			}
			response, err := r.parser.Encode(&Response{
				Type:      redis.BROADCAST_ACK,
				RequestId: request.RequestId,
				Packet:    packet,
			})
			if err != nil {
				redisLog.Debug("Error marshaling BROADCAST_ACK response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			r.publishResponse(request, response)
		},
	)
}

func (r *redisAdapter) publish(channel string, message []byte) error {
	result := make(chan error, 1)
	if err := r.enqueuePublish(r.publisher, channel, message, result); err != nil {
		return err
	}
	err, ok := <-result
	if !ok {
		return errRedisPublishPanicked
	}
	return err
}

func (r *redisAdapter) publishAsync(channel string, message []byte) {
	_ = r.enqueuePublish(r.publisher, channel, message, nil)
}

func (r *redisAdapter) enqueuePublish(target *queue.Queue, channel string, message []byte, result chan<- error) error {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	if target.IsShuttingDown() {
		return adapter.ErrAdapterClosed
	}
	target.Enqueue(func() {
		if result != nil {
			defer close(result)
		}
		err := r.redisClient.Client().Publish(r.redisClient.Context(), channel, message).Err()
		if err != nil {
			go r.redisClient.Emit("error", err)
		}
		if result != nil {
			result <- err
		}
	})
	return nil
}

func (r *redisAdapter) publishResponseMessage(channel string, message []byte) {
	_ = r.enqueuePublish(r.responses, channel, message, nil)
}

func (r *redisAdapter) publishJSONResponse(request *Request, response *Response) {
	message, err := json.Marshal(response)
	if err != nil {
		redisLog.Debug("Error marshaling response for request type %d with RequestId %s: %s", request.Type, request.RequestId, err.Error())
		return
	}
	r.publishResponse(request, message)
}

func (r *redisAdapter) publishResponse(request *Request, response []byte) {
	channel := r.responseChannel
	if r.publishOnSpecificResponseChannel {
		channel = r.responseChannel + string(request.Uid) + "#"
	}

	redisLog.Debug("publishing response to channel %s", channel)
	r.publishResponseMessage(channel, response)
}

// onResponse handles responses from other nodes.
func (r *redisAdapter) onResponse(msg []byte) {
	var response Response
	// Detect message format by first byte
	var err error
	if len(msg) > 0 && msg[0] == '{' {
		err = json.Unmarshal(msg, &response)
	} else {
		err = r.parser.Decode(msg, &response)
	}
	if err != nil {
		redisLog.Debug("ignoring malformed response")
		return
	}
	if response.RequestId == "" {
		redisLog.Debug("ignoring response without request id")
		return
	}

	requestId := response.RequestId
	// Handle acknowledgement responses
	if ackRequest, ok := r.ackRequests.Load(requestId); ok {
		switch response.Type {
		case redis.BROADCAST_CLIENT_COUNT:
			if response.ClientCount == nil || ackRequest.ClientCountCallback == nil {
				redisLog.Debug("ignoring malformed broadcast client count response")
				return
			}
			ackRequest.ClientCountCallback(*response.ClientCount)
		case redis.BROADCAST_ACK:
			if ackRequest.Ack != nil {
				ackRequest.Ack([]any{response.Packet}, nil)
			}
		}
		return
	}
	request, ok := r.requests.Load(requestId)
	if !ok {
		redisLog.Debug("ignoring unknown request")
		return
	}
	redisLog.Debug("received response type %d with id %s", response.Type, requestId)
	r.processResponse(request, &response)
}

// processResponse processes a response based on request type.
func (r *redisAdapter) processResponse(request *RedisRequest, response *Response) {
	requestId := response.RequestId
	switch request.Type {
	case redis.SOCKETS:
		var socketIds []socket.SocketId
		socketsPayload, ok := response.Sockets.(json.RawMessage)
		if !ok {
			redisLog.Debug("ignoring malformed SOCKETS response")
			return
		}
		if err := json.Unmarshal(socketsPayload, &socketIds); err != nil || socketIds == nil {
			redisLog.Debug("ignoring malformed SOCKETS response")
			return
		}
		request.Sockets.Add(socketIds...)
		msgCount := request.MsgCount.Add(1)
		if msgCount != request.NumSub || !r.finishRequest(requestId, request) {
			return
		}
		responses := slices.Map(request.Sockets.Keys(), func(socketId socket.SocketId) any {
			return socketId
		})
		request.Resolve(types.NewSlice(responses...))
	case redis.REMOTE_FETCH:
		var sockets []adapter.SocketResponse
		socketsPayload, ok := response.Sockets.(json.RawMessage)
		if !ok {
			redisLog.Debug("ignoring malformed REMOTE_FETCH response")
			return
		}
		if err := json.Unmarshal(socketsPayload, &sockets); err != nil || sockets == nil {
			redisLog.Debug("ignoring malformed REMOTE_FETCH response")
			return
		}
		if len(sockets) > 0 {
			request.Responses.Push(adapter.SocketResponsesToDetailsAny(sockets)...)
		}
		msgCount := request.MsgCount.Add(1)
		if msgCount == request.NumSub && r.finishRequest(requestId, request) {
			request.Resolve(request.Responses)
		}
	case redis.ALL_ROOMS:
		if response.Rooms == nil {
			redisLog.Debug("ignoring malformed ALL_ROOMS response")
			return
		}
		request.Rooms.Add(response.Rooms...)
		msgCount := request.MsgCount.Add(1)
		if msgCount == request.NumSub && r.finishRequest(requestId, request) {
			request.Resolve(nil)
		}
	case redis.REMOTE_JOIN, redis.REMOTE_LEAVE, redis.REMOTE_DISCONNECT:
		if r.finishRequest(requestId, request) {
			request.Resolve(nil)
		}
	case redis.SERVER_SIDE_EMIT:
		responseCount := request.Responses.Push(response.Data)
		redisLog.Debug("serverSideEmit: got %d responses out of %d", responseCount, request.NumSub)
		if int64(responseCount) == request.NumSub && r.finishRequest(requestId, request) {
			request.Resolve(request.Responses)
		}
	default:
		redisLog.Debug("ignoring unknown request type: %d", request.Type)
	}
}

// Broadcast broadcasts a packet to all clients, optionally propagating to other nodes.
func (r *redisAdapter) Broadcast(packet *parser.Packet, opts *socket.BroadcastOptions) {
	packet.Nsp = r.Nsp().Name()
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		packetOpts := adapter.EncodeOptions(opts)
		msg, err := r.parser.Encode(&Packet{
			Uid:    r.Uid(),
			Packet: packet,
			Opts:   packetOpts,
		})
		if err != nil {
			r.redisClient.Emit("error", err)
			return
		}

		channel := r.channel
		// Optimize channel routing for single-room broadcasts
		if len(packetOpts.Rooms) == 1 {
			channel = channel + string(packetOpts.Rooms[0]) + "#"
		}
		redisLog.Debug("publishing message to channel %s", channel)
		r.publishAsync(channel, msg)
	}
	r.Adapter.Broadcast(packet, opts)
}

// BroadcastWithAck broadcasts a packet and handles acknowledgements from clients across all nodes.
func (r *redisAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	packet.Nsp = r.Nsp().Name()
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		requestId := adapter.Uid2(defaultUidLength)
		message, err := r.parser.Encode(&Request{
			Uid:       r.uid,
			RequestId: requestId,
			Type:      redis.BROADCAST,
			Packet:    packet,
			Opts:      adapter.EncodeOptions(opts),
		})
		if err != nil {
			r.redisClient.Emit("error", err)
			return
		}

		ackRequest := &AckRequest{
			ClientCountCallback: clientCountCallback,
			Ack:                 ack,
		}
		timeout := adapter.DEFAULT_TIMEOUT
		if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
			timeout = utils.NormalizeTimerMilliseconds(*opts.Flags.Timeout)
		}
		r.registerAckRequest(requestId, ackRequest, timeout)

		r.publishAsync(r.requestChannel, message)
	}
	r.Adapter.BroadcastWithAck(packet, opts, clientCountCallback, ack)
}

// AllRooms returns all rooms across all cluster nodes.
func (r *redisAdapter) AllRooms() func(func(*types.Set[socket.Room], error)) {
	return func(cb func(*types.Set[socket.Room], error)) {
		localRooms := types.NewSet(r.Rooms().Keys()...)
		numSub, err := r.ServerCount()
		if err != nil {
			cb(nil, err)
			return
		}
		redisLog.Debug(`waiting for %d responses to "allRooms" request`, numSub)
		// Return local rooms if this is the only server
		if numSub <= 1 {
			cb(localRooms, nil)
			return
		}

		requestId := adapter.Uid2(defaultUidLength)

		message, err := json.Marshal(&Request{Type: redis.ALL_ROOMS, Uid: r.uid, RequestId: requestId})
		if err != nil {
			cb(nil, err)
			return
		}

		request := &RedisRequest{
			Type:   redis.ALL_ROOMS,
			NumSub: numSub,
			Resolve: func(*types.Slice[any]) {
				cb(localRooms, nil)
			},
			Rooms: localRooms,
		}
		request.MsgCount.Store(1) // Count self
		r.registerRequest(requestId, request, r.requestsTimeout, func() {
			cb(nil, errors.New("timeout reached while waiting for allRooms response"))
		})

		if err := r.publish(r.requestChannel, message); err != nil && r.finishRequest(requestId, request) {
			cb(nil, err)
		}
	}
}

// FetchSockets retrieves sockets across all cluster nodes.
func (r *redisAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(cb func([]socket.SocketDetails, error)) {
		r.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, err error) {
			if err != nil {
				cb(nil, err)
				return
			}
			// Return only local sockets if local flag is set
			if opts != nil && opts.Flags != nil && opts.Flags.Local {
				cb(localSockets, nil)
				return
			}

			numSub, err := r.ServerCount()
			if err != nil {
				cb(nil, err)
				return
			}
			redisLog.Debug(`waiting for %d responses to "fetchSockets" request`, numSub)

			// Return local sockets if this is the only server
			if numSub <= 1 {
				cb(localSockets, nil)
				return
			}

			requestId := adapter.Uid2(defaultUidLength)

			message, err := json.Marshal(&Request{Type: redis.REMOTE_FETCH, Uid: r.uid, RequestId: requestId, Opts: adapter.EncodeOptions(opts)})
			if err != nil {
				cb(nil, err)
				return
			}

			request := &RedisRequest{
				Type:   redis.REMOTE_FETCH,
				NumSub: numSub,
				Resolve: func(data *types.Slice[any]) {
					cb(adapter.AnySliceToSocketDetails(data.All()), nil)
				},
				Responses: types.NewSlice(adapter.SocketDetailsToAny(localSockets)...),
			}
			request.MsgCount.Store(1) // Count self
			r.registerRequest(requestId, request, r.requestsTimeout, func() {
				cb(nil, errors.New("timeout reached while waiting for fetchSockets response"))
			})

			if err := r.publish(r.requestChannel, message); err != nil && r.finishRequest(requestId, request) {
				cb(nil, err)
			}
		})
	}
}

// AddSockets adds sockets matching the options to the specified rooms across all nodes.
func (r *redisAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		message, err := json.Marshal(&Request{Uid: r.uid, Type: redis.REMOTE_JOIN, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
		if err != nil {
			redisLog.Debug("Error marshaling AddSockets request: %s", err.Error())
		} else {
			r.publishAsync(r.requestChannel, message)
		}
	}
	r.Adapter.AddSockets(opts, rooms)
}

// DelSockets removes sockets matching the options from the specified rooms across all nodes.
func (r *redisAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		message, err := json.Marshal(&Request{Uid: r.uid, Type: redis.REMOTE_LEAVE, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
		if err != nil {
			redisLog.Debug("Error marshaling DelSockets request: %s", err.Error())
		} else {
			r.publishAsync(r.requestChannel, message)
		}
	}
	r.Adapter.DelSockets(opts, rooms)
}

// DisconnectSockets disconnects sockets matching the options across all nodes.
func (r *redisAdapter) DisconnectSockets(opts *socket.BroadcastOptions, close bool) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		message, err := json.Marshal(&Request{Uid: r.uid, Type: redis.REMOTE_DISCONNECT, Opts: adapter.EncodeOptions(opts), Close: &close})
		if err != nil {
			redisLog.Debug("Error marshaling DisconnectSockets request: %s", err.Error())
		} else {
			r.publishAsync(r.requestChannel, message)
		}
	}
	r.Adapter.DisconnectSockets(opts, close)
}

// ServerSideEmit emits a packet to all servers in the cluster.
// If the last argument is a socket.Ack callback, it waits for acknowledgements from other servers.
func (r *redisAdapter) ServerSideEmit(packet []any) error {
	// Check if acknowledgement is requested
	if len(packet) > 0 {
		if ack, withAck := packet[len(packet)-1].(socket.Ack); withAck {
			return r.serverSideEmitWithAck(packet[:len(packet)-1], ack)
		}
	}

	message, err := json.Marshal(&Request{Uid: r.uid, Type: redis.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal ServerSideEmit request: %w", err)
	}

	return r.publish(r.requestChannel, message)
}

// serverSideEmitWithAck emits a packet and waits for acknowledgements from other servers.
func (r *redisAdapter) serverSideEmitWithAck(packet []any, ack socket.Ack) error {
	serverCount, err := r.ServerCount()
	if err != nil {
		return err
	}
	numSub := serverCount - 1 // Exclude self
	redisLog.Debug(`waiting for %d responses to "serverSideEmit" request`, numSub)
	// No other servers to wait for
	if numSub <= 0 {
		ack([]any{}, nil)
		return nil
	}

	requestId := adapter.Uid2(defaultUidLength)

	message, err := json.Marshal(&Request{Uid: r.uid, RequestId: requestId, Type: redis.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal serverSideEmitWithAck request: %w", err)
	}

	request := &RedisRequest{
		Type:   redis.SERVER_SIDE_EMIT,
		NumSub: numSub,
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Responses: types.NewSlice[any](),
	}
	r.registerRequest(requestId, request, r.requestsTimeout, func() {
		ack(request.Responses.All(), fmt.Errorf("timeout reached: only %d responses received out of %d", request.Responses.Len(), request.NumSub))
	})

	if err := r.publish(r.requestChannel, message); err != nil && r.finishRequest(requestId, request) {
		return err
	}
	return nil
}

// ServerCount returns the number of servers subscribed to the request channel.
func (r *redisAdapter) ServerCount() (int64, error) {
	return pubSubNumSub(r.ctx, r.redisClient.Sub(), false, r.requestChannel)
}

// Close cleans up Redis subscriptions and listeners.
// This should be called when the adapter is no longer needed.
func (r *redisAdapter) Close() {
	r.closeOnce.Do(r.close)
}

func (r *redisAdapter) close() {
	r.queueMu.Lock()
	r.responses.TryClose()
	r.publisher.TryClose()
	r.queueMu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	if r.broadcastSubscription != nil {
		r.broadcastSubscription.Close()
	}
	if r.requestSubscription != nil {
		r.requestSubscription.Close()
	}
	if r.pubSub != nil {
		releaseRedisPubSub(r.server, r.redisClient, r.pubSub)
	}
	r.Adapter.Close()
}
