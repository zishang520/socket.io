// Package adapter provides a Redis-based adapter implementation for Socket.IO clustering.
// This adapter enables horizontal scaling of Socket.IO servers by using Redis as a message broker
// for inter-node communication.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// redisLog is the logger for the Redis adapter.
var redisLog = log.NewLog("socket.io-redis")

// Subscription type constants for Redis Pub/Sub management.
const (
	// subKeyPattern is the key for pattern subscription storage.
	subKeyPattern = "psub"
	// subKeyChannel is the key for channel subscription storage.
	subKeyChannel = "sub"
)

// Default configuration values.
const (
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
		requests             *types.Map[string, *RedisRequest]
		ackRequests          *types.Map[string, *AckRequest]
		redisListeners       *types.Map[string, *rds.PubSub]
		friendlyErrorHandler func(...any)
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
		Adapter: socket.MakeAdapter(),

		opts:                 DefaultRedisAdapterOptions(),
		requests:             &types.Map[string, *RedisRequest]{},
		ackRequests:          &types.Map[string, *AckRequest]{},
		redisListeners:       &types.Map[string, *rds.PubSub]{},
		friendlyErrorHandler: func(...any) {},
	}

	c.Prototype(c)

	return c
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
func (r *redisAdapter) Uid() adapter.ServerId {
	return r.uid
}

// RequestsTimeout returns the configured timeout duration for inter-node requests.
func (r *redisAdapter) RequestsTimeout() time.Duration {
	return r.requestsTimeout
}

// PublishOnSpecificResponseChannel indicates if responses are published on node-specific channels.
func (r *redisAdapter) PublishOnSpecificResponseChannel() bool {
	return r.publishOnSpecificResponseChannel
}

// Parser returns the parser used for encoding/decoding Redis messages.
func (r *redisAdapter) Parser() redis.Parser {
	return r.parser
}

// Construct initializes the Redis adapter for the given namespace.
// It sets up Redis Pub/Sub subscriptions and starts message handling goroutines.
func (r *redisAdapter) Construct(nsp socket.Namespace) {
	r.Adapter.Construct(nsp)

	// Generate unique server ID
	uid, _ := adapter.Uid2(defaultUidLength)
	r.uid = adapter.ServerId(uid)

	// Configure timeout with default fallback
	if r.opts.GetRawRequestsTimeout() != nil {
		r.requestsTimeout = r.opts.RequestsTimeout()
	} else {
		r.requestsTimeout = DefaultRequestsTimeout
	}

	r.publishOnSpecificResponseChannel = r.opts.PublishOnSpecificResponseChannel()

	// Configure parser with default fallback
	if r.opts.Parser() != nil {
		r.parser = r.opts.Parser()
	} else {
		r.parser = utils.MsgPack()
	}

	// Build channel names
	prefix := defaultChannelPrefix
	if r.opts.GetRawKey() != nil {
		prefix = r.opts.Key()
	}

	r.channel = prefix + "#" + nsp.Name() + "#"
	r.requestChannel = prefix + "-request#" + r.Nsp().Name() + "#"
	r.responseChannel = prefix + "-response#" + r.Nsp().Name() + "#"
	r.specificResponseChannel = r.responseChannel + string(r.uid) + "#"

	// Set up error handler
	r.friendlyErrorHandler = func(...any) {
		if r.redisClient.ListenerCount("error") == 1 {
			redisLog.Warning("missing 'error' handler on this Redis client")
		}
	}

	r.redisClient.On("error", r.friendlyErrorHandler)

	// Subscribe to broadcast channel with pattern matching
	pubsub := r.redisClient.Client.PSubscribe(r.redisClient.Context, r.channel+"*")
	r.redisListeners.Store(subKeyPattern, pubsub)

	go r.handlePatternMessages(pubsub)

	// Subscribe to request/response channels
	sub := r.redisClient.Client.Subscribe(r.redisClient.Context, r.requestChannel, r.responseChannel, r.specificResponseChannel)
	r.redisListeners.Store(subKeyChannel, sub)

	go r.handleChannelMessages(sub)
}

// handlePatternMessages processes messages from pattern subscriptions.
func (r *redisAdapter) handlePatternMessages(pubsub *rds.PubSub) {
	defer pubsub.Close()

	for {
		select {
		case <-r.redisClient.Context.Done():
			return
		default:
			msg, err := pubsub.ReceiveMessage(r.redisClient.Context)
			if err != nil {
				r.redisClient.Emit("error", err)
				if errors.Is(err, rds.ErrClosed) {
					return
				}
				continue
			}
			r.onMessage(msg.Pattern, msg.Channel, []byte(msg.Payload))
		}
	}
}

// handleChannelMessages processes messages from channel subscriptions.
func (r *redisAdapter) handleChannelMessages(sub *rds.PubSub) {
	defer sub.Close()

	for {
		select {
		case <-r.redisClient.Context.Done():
			return
		default:
			msg, err := sub.ReceiveMessage(r.redisClient.Context)
			if err != nil {
				r.redisClient.Emit("error", err)
				if errors.Is(err, rds.ErrClosed) {
					return
				}
				continue
			}
			r.onRequest(msg.Channel, []byte(msg.Payload))
		}
	}
}

// onMessage handles broadcast messages from Redis pattern subscriptions.
func (r *redisAdapter) onMessage(_ string, channel string, msg []byte) {
	// Validate channel length
	if len(channel) <= len(r.channel) {
		redisLog.Debug("ignore channel shorter than expected")
		return
	}

	if !strings.HasPrefix(channel, r.channel) {
		redisLog.Debug("ignore different channel")
		return
	}

	// Extract room from channel name
	room := channel[len(r.channel) : len(channel)-1]
	if room != "" && !r.hasRoom(socket.Room(room)) {
		redisLog.Debug("ignore unknown room %s", room)
		return
	}

	var packet *Packet
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

	r.Adapter.Broadcast(packet.Packet, adapter.DecodeOptions(packet.Opts))
}

// hasRoom checks if the adapter has the specified room.
func (r *redisAdapter) hasRoom(room socket.Room) bool {
	_, ok := r.Rooms().Load(room)
	return ok
}

// onRequest handles inter-node requests from Redis.
func (r *redisAdapter) onRequest(channel string, msg []byte) {
	// Route response messages to onResponse handler
	if strings.HasPrefix(channel, r.responseChannel) {
		r.onResponse(channel, msg)
		return
	}

	// Validate request channel
	if !strings.HasPrefix(channel, r.requestChannel) {
		redisLog.Debug("ignore different channel")
		return
	}

	var request *Request
	// Detect message format by first byte
	if len(msg) > 0 && msg[0] == '{' {
		if err := json.Unmarshal(msg, &request); err != nil {
			redisLog.Debug("ignoring malformed request")
			return
		}
	} else {
		if err := r.parser.Decode(msg, &request); err != nil {
			redisLog.Debug("ignoring malformed request")
			return
		}
	}

	redisLog.Debug("received request %v", request)

	r.handleRequest(request)
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
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}

	sockets := r.Adapter.Sockets(types.NewSet(request.Rooms...))
	response, err := json.Marshal(&Response{
		RequestId: request.RequestId,
		Sockets: slices.Map(sockets.Keys(), func(socketId socket.SocketId) *adapter.SocketResponse {
			return &adapter.SocketResponse{Id: socketId}
		}),
	})
	if err != nil {
		redisLog.Debug("Error marshaling SOCKETS response for RequestId %s: %s", request.RequestId, err.Error())
		return
	}
	r.publishResponse(request, response)
}

// handleAllRoomsRequest handles ALL_ROOMS request type.
func (r *redisAdapter) handleAllRoomsRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}

	response, err := json.Marshal(&Response{
		RequestId: request.RequestId,
		Rooms:     r.Rooms().Keys(),
	})
	if err != nil {
		redisLog.Debug("Error marshaling ALL_ROOMS response for RequestId %s: %s", request.RequestId, err.Error())
		return
	}
	r.publishResponse(request, response)
}

// handleRemoteJoinRequest handles REMOTE_JOIN request type.
func (r *redisAdapter) handleRemoteJoinRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.AddSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
		return
	}

	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Join(request.Room)

		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			redisLog.Debug("Error marshaling REMOTE_JOIN response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

// handleRemoteLeaveRequest handles REMOTE_LEAVE request type.
func (r *redisAdapter) handleRemoteLeaveRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.DelSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
		return
	}

	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Leave(request.Room)

		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			redisLog.Debug("Error marshaling REMOTE_LEAVE response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

// handleRemoteDisconnectRequest handles REMOTE_DISCONNECT request type.
func (r *redisAdapter) handleRemoteDisconnectRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.DisconnectSockets(adapter.DecodeOptions(request.Opts), request.Close)
		return
	}

	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Disconnect(request.Close)

		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			redisLog.Debug("Error marshaling REMOTE_DISCONNECT response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

// handleRemoteFetchRequest handles REMOTE_FETCH request type.
func (r *redisAdapter) handleRemoteFetchRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}

	r.Adapter.FetchSockets(adapter.DecodeOptions(request.Opts))(func(localSockets []socket.SocketDetails, e error) {
		if e != nil {
			redisLog.Debug("REMOTE_FETCH Adapter.FetchSockets error: %s", e.Error())
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
			redisLog.Debug("Error marshaling REMOTE_FETCH response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponse(request, response)
	})
}

// handleServerSideEmitRequest handles SERVER_SIDE_EMIT request type.
func (r *redisAdapter) handleServerSideEmitRequest(request *Request) {
	// Ignore messages from self
	if request.Uid == r.uid {
		redisLog.Debug("ignore same uid")
		return
	}

	// No acknowledgement needed
	if request.RequestId == "" {
		r.Nsp().OnServerSideEmit(request.Data)
		return
	}

	// Handle with acknowledgement
	called := &sync.Once{}
	callback := socket.Ack(func(args []any, err error) {
		called.Do(func() {
			redisLog.Debug("calling acknowledgement with %v", args)
			response, err := json.Marshal(&Response{
				Type:      redis.SERVER_SIDE_EMIT,
				RequestId: request.RequestId,
				Data:      args,
			})
			if err != nil {
				redisLog.Debug("Error marshaling SERVER_SIDE_EMIT response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			if err := r.redisClient.Client.Publish(r.redisClient.Context, r.responseChannel, response).Err(); err != nil {
				r.redisClient.Emit("error", err)
			}
		})
	})
	r.Nsp().OnServerSideEmit(append(request.Data, callback))
}

// handleBroadcastRequest handles BROADCAST request type.
func (r *redisAdapter) handleBroadcastRequest(request *Request) {
	if _, ok := r.ackRequests.Load(request.RequestId); ok {
		return
	}

	r.Adapter.BroadcastWithAck(
		request.Packet,
		adapter.DecodeOptions(request.Opts),
		func(clientCount uint64) {
			redisLog.Debug("waiting for %d client acknowledgements", clientCount)
			response, err := json.Marshal(&Response{
				Type:        redis.BROADCAST_CLIENT_COUNT,
				RequestId:   request.RequestId,
				ClientCount: clientCount,
			})
			if err != nil {
				redisLog.Debug("Error marshaling BROADCAST_CLIENT_COUNT response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			r.publishResponse(request, response)
		},
		func(args []any, _ error) {
			redisLog.Debug("received acknowledgement with value %v", args)
			response, err := r.parser.Encode(&Response{
				Type:      redis.BROADCAST_ACK,
				RequestId: request.RequestId,
				Packet:    args,
			})
			if err != nil {
				redisLog.Debug("Error marshaling BROADCAST_ACK response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			r.publishResponse(request, response)
		},
	)
}

// publishResponse sends a response to the requesting node via Redis.
func (r *redisAdapter) publishResponse(request *Request, response []byte) {
	responseChannel := r.responseChannel
	if r.publishOnSpecificResponseChannel {
		responseChannel += string(request.Uid) + "#"
	}
	redisLog.Debug("publishing response to channel %s", responseChannel)
	if err := r.redisClient.Client.Publish(r.redisClient.Context, responseChannel, response).Err(); err != nil {
		r.redisClient.Emit("error", err)
	}
}

// onResponse handles responses from other nodes.
func (r *redisAdapter) onResponse(_ string, msg []byte) {
	var response *Response

	// Detect message format by first byte
	if len(msg) > 0 && msg[0] == '{' {
		if err := json.Unmarshal(msg, &response); err != nil {
			redisLog.Debug("ignoring malformed response")
			return
		}
	} else {
		if err := r.parser.Decode(msg, &response); err != nil {
			redisLog.Debug("ignoring malformed response")
			return
		}
	}

	requestId := response.RequestId

	// Handle acknowledgement responses
	if ackRequest, ok := r.ackRequests.Load(requestId); ok {
		switch response.Type {
		case redis.BROADCAST_CLIENT_COUNT:
			ackRequest.ClientCountCallback(response.ClientCount)
		case redis.BROADCAST_ACK:
			ackRequest.Ack(response.Packet, nil)
		}
		return
	}

	// Handle regular responses
	if requestId == "" {
		redisLog.Debug("ignoring unknown request")
		return
	}

	request, ok := r.requests.Load(requestId)
	if !ok {
		redisLog.Debug("ignoring unknown request")
		return
	}

	redisLog.Debug("received response %v", response)
	r.processResponse(request, response, requestId)
}

// processResponse processes a response based on request type.
func (r *redisAdapter) processResponse(request *RedisRequest, response *Response, requestId string) {
	switch request.Type {
	case redis.SOCKETS, redis.REMOTE_FETCH:
		request.MsgCount.Add(1)

		if response.Sockets == nil {
			return
		}
		request.Sockets.Push(response.Sockets...)

		if request.MsgCount.Load() == request.NumSub {
			utils.ClearTimeout(request.Timeout.Load())
			if request.Resolve != nil {
				request.Resolve(types.NewSlice(slices.Map(request.Sockets.All(), func(client *adapter.SocketResponse) any {
					return socket.SocketDetails(adapter.NewRemoteSocket(client))
				})...))
			}
			r.requests.Delete(requestId)
		}

	case redis.ALL_ROOMS:
		request.MsgCount.Add(1)

		if response.Rooms == nil {
			return
		}
		request.Rooms.Add(response.Rooms...)

		if request.MsgCount.Load() == request.NumSub {
			utils.ClearTimeout(request.Timeout.Load())
			if request.Resolve != nil {
				request.Resolve(types.NewSlice(slices.Map(request.Rooms.Keys(), func(room socket.Room) any {
					return room
				})...))
			}
			r.requests.Delete(requestId)
		}

	case redis.REMOTE_JOIN, redis.REMOTE_LEAVE, redis.REMOTE_DISCONNECT:
		utils.ClearTimeout(request.Timeout.Load())
		if request.Resolve != nil {
			request.Resolve(nil)
		}
		r.requests.Delete(requestId)

	case redis.SERVER_SIDE_EMIT:
		request.Responses.Push(response.Data)

		redisLog.Debug("serverSideEmit: got %d responses out of %d", request.Responses.Len(), request.NumSub)
		if int64(request.Responses.Len()) == request.NumSub {
			utils.ClearTimeout(request.Timeout.Load())
			if request.Resolve != nil {
				request.Resolve(request.Responses)
			}
			r.requests.Delete(requestId)
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
		msg, err := r.parser.Encode(&Packet{
			Uid:    r.Uid(),
			Packet: packet,
			Opts:   adapter.EncodeOptions(opts),
		})
		if err == nil {
			channel := r.channel
			// Optimize channel routing for single-room broadcasts
			if opts.Rooms != nil && opts.Rooms.Len() == 1 {
				for _, room := range opts.Rooms.Keys() {
					channel += string(room) + "#"
					break
				}
			}
			redisLog.Debug("publishing message to channel %s", channel)
			if err := r.redisClient.Client.Publish(r.redisClient.Context, channel, msg).Err(); err != nil {
				r.redisClient.Emit("error", err)
			}
		}
	}
	r.Adapter.Broadcast(packet, opts)
}

// BroadcastWithAck broadcasts a packet and handles acknowledgements from clients across all nodes.
func (r *redisAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	packet.Nsp = r.Nsp().Name()

	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		requestId, err := adapter.Uid2(defaultUidLength)
		if err != nil {
			redisLog.Debug("Error generating request ID: %s", err.Error())
		} else {
			request, err := r.parser.Encode(&Request{
				Uid:       r.uid,
				RequestId: requestId,
				Type:      redis.BROADCAST,
				Packet:    packet,
				Opts:      adapter.EncodeOptions(opts),
			})
			if err == nil {
				if err := r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err(); err != nil {
					r.redisClient.Emit("error", err)
				}

				r.ackRequests.Store(requestId, &AckRequest{
					ClientCountCallback: clientCountCallback,
					Ack:                 ack,
				})

				// Calculate cleanup timeout
				timeout := time.Duration(0)
				if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
					timeout = *opts.Flags.Timeout
				}

				// Clean up ackRequests after timeout
				utils.SetTimeout(func() {
					r.ackRequests.Delete(requestId)
				}, timeout)
			}
		}
	}

	r.Adapter.BroadcastWithAck(packet, opts, clientCountCallback, ack)
}

// AllRooms returns a function that retrieves all rooms across all nodes in the cluster.
func (r *redisAdapter) AllRooms() func(func(*types.Set[socket.Room], error)) {
	return func(cb func(*types.Set[socket.Room], error)) {
		localRooms := types.NewSet(r.Rooms().Keys()...)
		numSub := r.ServerCount()
		redisLog.Debug(`waiting for %d responses to "allRooms" request`, numSub)

		// Return local rooms if this is the only server
		if numSub <= 1 {
			cb(localRooms, nil)
			return
		}

		requestId, err := adapter.Uid2(defaultUidLength)
		if err != nil {
			cb(nil, err)
			return
		}

		request, err := json.Marshal(&Request{
			Type:      redis.ALL_ROOMS,
			Uid:       r.uid,
			RequestId: requestId,
		})
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

		r.requests.Store(requestId, &RedisRequest{
			Type:   redis.ALL_ROOMS,
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
				c.Store(1) // Count self
			}),
			Rooms: localRooms,
		})

		if err := r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err(); err != nil {
			r.redisClient.Emit("error", err)
		}
	}
}

// FetchSockets returns a function that retrieves all sockets matching the options across all nodes.
func (r *redisAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(cb func([]socket.SocketDetails, error)) {
		r.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, _ error) {
			// Return only local sockets if local flag is set
			if opts.Flags != nil && opts.Flags.Local {
				cb(localSockets, nil)
				return
			}

			numSub := r.ServerCount()
			redisLog.Debug(`waiting for %d responses to "fetchSockets" request`, numSub)

			// Return local sockets if this is the only server
			if numSub <= 1 {
				cb(localSockets, nil)
				return
			}

			requestId, err := adapter.Uid2(defaultUidLength)
			if err != nil {
				cb(nil, err)
				return
			}

			request, err := json.Marshal(&Request{
				Type:      redis.REMOTE_FETCH,
				Uid:       r.uid,
				RequestId: requestId,
				Opts:      adapter.EncodeOptions(opts),
			})
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

			r.requests.Store(requestId, &RedisRequest{
				Type:   redis.REMOTE_FETCH,
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
					c.Store(1) // Count self
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

			if err := r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err(); err != nil {
				r.redisClient.Emit("error", err)
			}
		})
	}
}

// AddSockets adds sockets matching the options to the specified rooms across all nodes.
func (r *redisAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.AddSockets(opts, rooms)
		return
	}

	request, err := json.Marshal(&Request{
		Uid:   r.uid,
		Type:  redis.REMOTE_JOIN,
		Opts:  adapter.EncodeOptions(opts),
		Rooms: rooms,
	})
	if err != nil {
		redisLog.Debug("Error marshaling AddSockets request: %s", err.Error())
		return
	}

	if err := r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err(); err != nil {
		r.redisClient.Emit("error", err)
	}
}

// DelSockets removes sockets matching the options from the specified rooms across all nodes.
func (r *redisAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.DelSockets(opts, rooms)
		return
	}

	request, err := json.Marshal(&Request{
		Uid:   r.uid,
		Type:  redis.REMOTE_LEAVE,
		Opts:  adapter.EncodeOptions(opts),
		Rooms: rooms,
	})
	if err != nil {
		redisLog.Debug("Error marshaling DelSockets request: %s", err.Error())
		return
	}

	if err := r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err(); err != nil {
		r.redisClient.Emit("error", err)
	}
}

// DisconnectSockets disconnects sockets matching the options across all nodes.
func (r *redisAdapter) DisconnectSockets(opts *socket.BroadcastOptions, close bool) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.DisconnectSockets(opts, close)
		return
	}

	request, err := json.Marshal(&Request{
		Uid:   r.uid,
		Type:  redis.REMOTE_DISCONNECT,
		Opts:  adapter.EncodeOptions(opts),
		Close: close,
	})
	if err != nil {
		redisLog.Debug("Error marshaling DisconnectSockets request: %s", err.Error())
		return
	}

	if err := r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err(); err != nil {
		r.redisClient.Emit("error", err)
	}
}

// ServerSideEmit emits a packet to all servers in the cluster.
// If the last argument is a socket.Ack callback, it waits for acknowledgements from other servers.
func (r *redisAdapter) ServerSideEmit(packet []any) error {
	if len(packet) == 0 {
		return errors.New("packet cannot be empty")
	}

	// Check if acknowledgement is requested
	if ack, withAck := packet[len(packet)-1].(socket.Ack); withAck {
		return r.serverSideEmitWithAck(packet[:len(packet)-1], ack)
	}

	request, err := json.Marshal(&Request{
		Uid:  r.uid,
		Type: redis.SERVER_SIDE_EMIT,
		Data: packet,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal ServerSideEmit request: %w", err)
	}

	return r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err()
}

// serverSideEmitWithAck emits a packet and waits for acknowledgements from other servers.
func (r *redisAdapter) serverSideEmitWithAck(packet []any, ack socket.Ack) error {
	numSub := r.ServerCount() - 1 // Exclude self

	redisLog.Debug(`waiting for %d responses to "serverSideEmit" request`, numSub)

	// No other servers to wait for
	if numSub <= 0 {
		ack(nil, nil)
		return nil
	}

	requestId, err := adapter.Uid2(defaultUidLength)
	if err != nil {
		return fmt.Errorf("failed to generate request ID: %w", err)
	}

	request, err := json.Marshal(&Request{
		Uid:       r.uid,
		RequestId: requestId,
		Type:      redis.SERVER_SIDE_EMIT,
		Data:      packet,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal serverSideEmitWithAck request: %w", err)
	}

	timeout := utils.SetTimeout(func() {
		if storedRequest, ok := r.requests.Load(requestId); ok {
			ack(storedRequest.Responses.All(), fmt.Errorf("timeout reached: only %d responses received out of %d", storedRequest.Responses.Len(), storedRequest.NumSub))
			r.requests.Delete(requestId)
		}
	}, r.requestsTimeout)

	r.requests.Store(requestId, &RedisRequest{
		Type:   redis.SERVER_SIDE_EMIT,
		NumSub: numSub,
		Timeout: utils.Tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
			t.Store(timeout)
		}),
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Responses: types.NewSlice[any](),
	})

	return r.redisClient.Client.Publish(r.redisClient.Context, r.requestChannel, request).Err()
}

// ServerCount returns the number of servers subscribed to the request channel.
func (r *redisAdapter) ServerCount() int64 {
	result, err := r.redisClient.Client.PubSubNumSub(r.redisClient.Context, r.requestChannel).Result()
	if err != nil {
		r.redisClient.Emit("error", err)
		return 0
	}

	if count, ok := result[r.requestChannel]; ok {
		return count
	}
	return 0
}

// Close cleans up Redis subscriptions and listeners.
// This should be called when the adapter is no longer needed.
func (r *redisAdapter) Close() {
	// Unsubscribe from pattern subscription
	if psub, ok := r.redisListeners.Load(subKeyPattern); ok {
		if err := psub.PUnsubscribe(r.redisClient.Context, r.channel+"*"); err != nil {
			r.redisClient.Emit("error", err)
		}
	}

	// Unsubscribe from channel subscriptions
	if sub, ok := r.redisListeners.Load(subKeyChannel); ok {
		if err := sub.Unsubscribe(r.redisClient.Context, r.requestChannel, r.responseChannel, r.specificResponseChannel); err != nil {
			r.redisClient.Emit("error", err)
		}
	}

	// Remove error handler
	r.redisClient.RemoveListener("error", r.friendlyErrorHandler)
}
