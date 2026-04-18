// Package adapter provides a Valkey-based adapter implementation for Socket.IO clustering.
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
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var valkeyLog = log.NewLog("socket.io-valkey")

const (
	subKeyPattern = "psub"
	subKeyChannel = "sub"

	defaultChannelPrefix = "socket.io"
	defaultUidLength     = 6
)

type (
	// ValkeyAdapterBuilder builds a ValkeyAdapter with the given Valkey client and options.
	ValkeyAdapterBuilder struct {
		// Valkey is the Valkey client used by the adapter for Pub/Sub communication.
		Valkey *valkey.ValkeyClient
		// Opts contains configuration options for the adapter.
		Opts ValkeyAdapterOptionsInterface
	}

	valkeyAdapter struct {
		socket.Adapter

		valkeyClient *valkey.ValkeyClient
		opts         *ValkeyAdapterOptions

		uid                              adapter.ServerId
		requestsTimeout                  time.Duration
		publishOnSpecificResponseChannel bool
		parser                           valkey.Parser

		channel                 string
		requestChannel          string
		responseChannel         string
		specificResponseChannel string

		requests             *types.Map[string, *ValkeyRequest]
		ackRequests          *types.Map[string, *AckRequest]
		valkeyListeners      *types.Map[string, *valkey.ValkeyPubSub]
		friendlyErrorHandler func(...any)
	}
)

// New creates a new ValkeyAdapter for the given namespace.
func (vb *ValkeyAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewValkeyAdapter(nsp, vb.Valkey, vb.Opts)
}

// MakeValkeyAdapter creates a new uninitialized valkeyAdapter with default options.
func MakeValkeyAdapter() ValkeyAdapter {
	c := &valkeyAdapter{
		Adapter: socket.MakeAdapter(),

		opts:                 DefaultValkeyAdapterOptions(),
		requests:             &types.Map[string, *ValkeyRequest]{},
		ackRequests:          &types.Map[string, *AckRequest]{},
		valkeyListeners:      &types.Map[string, *valkey.ValkeyPubSub]{},
		friendlyErrorHandler: func(...any) {},
	}

	c.Prototype(c)

	return c
}

// NewValkeyAdapter creates and initializes a new ValkeyAdapter for the given namespace.
func NewValkeyAdapter(nsp socket.Namespace, valkeyClient *valkey.ValkeyClient, opts any) ValkeyAdapter {
	c := MakeValkeyAdapter()
	c.SetValkey(valkeyClient)
	c.SetOpts(opts)
	c.Construct(nsp)
	return c
}

func (r *valkeyAdapter) SetValkey(valkeyClient *valkey.ValkeyClient) { r.valkeyClient = valkeyClient }

func (r *valkeyAdapter) SetOpts(opts any) {
	if options, ok := opts.(ValkeyAdapterOptionsInterface); ok {
		r.opts.Assign(options)
	}
}

func (r *valkeyAdapter) Uid() adapter.ServerId          { return r.uid }
func (r *valkeyAdapter) RequestsTimeout() time.Duration { return r.requestsTimeout }
func (r *valkeyAdapter) PublishOnSpecificResponseChannel() bool {
	return r.publishOnSpecificResponseChannel
}
func (r *valkeyAdapter) Parser() valkey.Parser { return r.parser }

// Construct initializes the Valkey adapter for the given namespace.
func (r *valkeyAdapter) Construct(nsp socket.Namespace) {
	r.Adapter.Construct(nsp)

	r.uid = adapter.ServerId(adapter.Uid2(defaultUidLength))

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
		if r.valkeyClient.ListenerCount("error") == 1 {
			valkeyLog.Warning("missing 'error' handler on this Valkey client")
		}
	}
	_ = r.valkeyClient.On("error", r.friendlyErrorHandler)

	pubsub := r.valkeyClient.PSubscribe(r.valkeyClient.Context, r.channel+"*")
	r.valkeyListeners.Store(subKeyPattern, pubsub)
	go r.handlePatternMessages(pubsub)

	sub := r.valkeyClient.Subscribe(r.valkeyClient.Context, r.requestChannel, r.responseChannel, r.specificResponseChannel)
	r.valkeyListeners.Store(subKeyChannel, sub)
	go r.handleChannelMessages(sub)
}

func (r *valkeyAdapter) handlePatternMessages(pubsub *valkey.ValkeyPubSub) {
	defer func() { _ = pubsub.Close() }()
	for {
		msg, err := pubsub.ReceiveMessage(r.valkeyClient.Context)
		if err != nil {
			if errors.Is(err, valkey.ErrValkeyPubSubClosed) || r.valkeyClient.Context.Err() != nil {
				return
			}
			r.valkeyClient.Emit("error", err)
			continue
		}
		r.onMessage(msg.Pattern, msg.Channel, []byte(msg.Payload))
	}
}

func (r *valkeyAdapter) handleChannelMessages(sub *valkey.ValkeyPubSub) {
	defer func() { _ = sub.Close() }()
	for {
		msg, err := sub.ReceiveMessage(r.valkeyClient.Context)
		if err != nil {
			if errors.Is(err, valkey.ErrValkeyPubSubClosed) || r.valkeyClient.Context.Err() != nil {
				return
			}
			r.valkeyClient.Emit("error", err)
			continue
		}
		r.onRequest(msg.Channel, []byte(msg.Payload))
	}
}

func (r *valkeyAdapter) onMessage(_ string, channel string, msg []byte) {
	if len(channel) <= len(r.channel) || !strings.HasPrefix(channel, r.channel) {
		valkeyLog.Debug("ignore channel: shorter than expected or prefix mismatch")
		return
	}

	room := channel[len(r.channel) : len(channel)-1]
	if room != "" && !r.hasRoom(socket.Room(room)) {
		valkeyLog.Debug("ignore unknown room %s", room)
		return
	}

	var packet *Packet
	if err := r.parser.Decode(msg, &packet); err != nil {
		valkeyLog.Debug("error decoding message: %v", err)
		return
	}

	if r.uid == packet.Uid {
		valkeyLog.Debug("ignore same uid")
		return
	}
	if packet.Packet != nil && packet.Packet.Nsp == "" {
		packet.Packet.Nsp = "/"
	}
	if packet.Packet == nil || packet.Packet.Nsp != r.Nsp().Name() {
		valkeyLog.Debug("ignore different namespace")
		return
	}
	r.Adapter.Broadcast(packet.Packet, adapter.DecodeOptions(packet.Opts))
}

func (r *valkeyAdapter) hasRoom(room socket.Room) bool {
	_, ok := r.Rooms().Load(room)
	return ok
}

func (r *valkeyAdapter) onRequest(channel string, msg []byte) {
	if strings.HasPrefix(channel, r.responseChannel) {
		r.onResponse(channel, msg)
		return
	}
	if !strings.HasPrefix(channel, r.requestChannel) {
		valkeyLog.Debug("ignore different channel")
		return
	}

	var request *Request
	if len(msg) > 0 && msg[0] == '{' {
		if err := json.Unmarshal(msg, &request); err != nil {
			valkeyLog.Debug("ignoring malformed request")
			return
		}
	} else {
		if err := r.parser.Decode(msg, &request); err != nil {
			valkeyLog.Debug("ignoring malformed request")
			return
		}
	}
	valkeyLog.Debug("received request %v", request)
	r.handleRequest(request)
}

func (r *valkeyAdapter) handleRequest(request *Request) {
	switch request.Type {
	case valkey.SOCKETS:
		r.handleSocketsRequest(request)
	case valkey.ALL_ROOMS:
		r.handleAllRoomsRequest(request)
	case valkey.REMOTE_JOIN:
		r.handleRemoteJoinRequest(request)
	case valkey.REMOTE_LEAVE:
		r.handleRemoteLeaveRequest(request)
	case valkey.REMOTE_DISCONNECT:
		r.handleRemoteDisconnectRequest(request)
	case valkey.REMOTE_FETCH:
		r.handleRemoteFetchRequest(request)
	case valkey.SERVER_SIDE_EMIT:
		r.handleServerSideEmitRequest(request)
	case valkey.BROADCAST:
		r.handleBroadcastRequest(request)
	default:
		valkeyLog.Debug("ignoring unknown request type: %d", request.Type)
	}
}

func (r *valkeyAdapter) handleSocketsRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}
	sockets := r.Sockets(types.NewSet(request.Rooms...))
	response, err := json.Marshal(&Response{
		RequestId: request.RequestId,
		Sockets: slices.Map(sockets.Keys(), func(socketId socket.SocketId) *adapter.SocketResponse {
			return &adapter.SocketResponse{Id: socketId}
		}),
	})
	if err != nil {
		valkeyLog.Debug("Error marshaling SOCKETS response: %s", err.Error())
		return
	}
	r.publishResponse(request, response)
}

func (r *valkeyAdapter) handleAllRoomsRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}
	response, err := json.Marshal(&Response{
		RequestId: request.RequestId,
		Rooms:     r.Rooms().Keys(),
	})
	if err != nil {
		valkeyLog.Debug("Error marshaling ALL_ROOMS response: %s", err.Error())
		return
	}
	r.publishResponse(request, response)
}

func (r *valkeyAdapter) handleRemoteJoinRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.AddSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
		return
	}
	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Join(request.Room)
		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			valkeyLog.Debug("Error marshaling REMOTE_JOIN response: %s", err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

func (r *valkeyAdapter) handleRemoteLeaveRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.DelSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
		return
	}
	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Leave(request.Room)
		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			valkeyLog.Debug("Error marshaling REMOTE_LEAVE response: %s", err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

func (r *valkeyAdapter) handleRemoteDisconnectRequest(request *Request) {
	if request.Opts != nil {
		r.Adapter.DisconnectSockets(adapter.DecodeOptions(request.Opts), request.Close)
		return
	}
	if client, ok := r.Nsp().Sockets().Load(request.Sid); ok {
		client.Disconnect(request.Close)
		response, err := json.Marshal(&Response{RequestId: request.RequestId})
		if err != nil {
			valkeyLog.Debug("Error marshaling REMOTE_DISCONNECT response: %s", err.Error())
			return
		}
		r.publishResponse(request, response)
	}
}

func (r *valkeyAdapter) handleRemoteFetchRequest(request *Request) {
	if _, ok := r.requests.Load(request.RequestId); ok {
		return
	}
	r.Adapter.FetchSockets(adapter.DecodeOptions(request.Opts))(func(localSockets []socket.SocketDetails, e error) {
		if e != nil {
			valkeyLog.Debug("REMOTE_FETCH Adapter.FetchSockets error: %s", e.Error())
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
			valkeyLog.Debug("Error marshaling REMOTE_FETCH response: %s", err.Error())
			return
		}
		r.publishResponse(request, response)
	})
}

func (r *valkeyAdapter) handleServerSideEmitRequest(request *Request) {
	if request.Uid == r.uid {
		valkeyLog.Debug("ignore same uid")
		return
	}
	if request.RequestId == "" {
		r.Nsp().OnServerSideEmit(request.Data)
		return
	}

	called := &sync.Once{}
	callback := func(args []any, err error) {
		called.Do(func() {
			valkeyLog.Debug("calling acknowledgement with %v", args)
			response, err := json.Marshal(&Response{
				Type:      valkey.SERVER_SIDE_EMIT,
				RequestId: request.RequestId,
				Data:      args,
			})
			if err != nil {
				valkeyLog.Debug("Error marshaling SERVER_SIDE_EMIT response: %s", err.Error())
				return
			}
			if err := r.valkeyClient.Publish(r.valkeyClient.Context, r.responseChannel, response); err != nil {
				r.valkeyClient.Emit("error", err)
			}
		})
	}
	r.Nsp().OnServerSideEmit(append(request.Data, callback))
}

func (r *valkeyAdapter) handleBroadcastRequest(request *Request) {
	if _, ok := r.ackRequests.Load(request.RequestId); ok {
		return
	}
	r.Adapter.BroadcastWithAck(
		request.Packet,
		adapter.DecodeOptions(request.Opts),
		func(clientCount uint64) {
			valkeyLog.Debug("waiting for %d client acknowledgements", clientCount)
			response, err := json.Marshal(&Response{
				Type:        valkey.BROADCAST_CLIENT_COUNT,
				RequestId:   request.RequestId,
				ClientCount: clientCount,
			})
			if err != nil {
				valkeyLog.Debug("Error marshaling BROADCAST_CLIENT_COUNT response: %s", err.Error())
				return
			}
			r.publishResponse(request, response)
		},
		func(args []any, _ error) {
			valkeyLog.Debug("received acknowledgement with value %v", args)
			response, err := r.parser.Encode(&Response{
				Type:      valkey.BROADCAST_ACK,
				RequestId: request.RequestId,
				Packet:    args,
			})
			if err != nil {
				valkeyLog.Debug("Error marshaling BROADCAST_ACK response: %s", err.Error())
				return
			}
			r.publishResponse(request, response)
		},
	)
}

func (r *valkeyAdapter) publishResponse(request *Request, response []byte) {
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

	valkeyLog.Debug("publishing response to channel %s", channel)
	if err := r.valkeyClient.Publish(r.valkeyClient.Context, channel, response); err != nil {
		r.valkeyClient.Emit("error", err)
	}
}

func (r *valkeyAdapter) onResponse(_ string, msg []byte) {
	var response *Response
	if len(msg) > 0 && msg[0] == '{' {
		if err := json.Unmarshal(msg, &response); err != nil {
			valkeyLog.Debug("ignoring malformed response")
			return
		}
	} else {
		if err := r.parser.Decode(msg, &response); err != nil {
			valkeyLog.Debug("ignoring malformed response")
			return
		}
	}

	requestId := response.RequestId
	if ackRequest, ok := r.ackRequests.Load(requestId); ok {
		switch response.Type {
		case valkey.BROADCAST_CLIENT_COUNT:
			ackRequest.ClientCountCallback(response.ClientCount)
		case valkey.BROADCAST_ACK:
			ackRequest.Ack(response.Packet, nil)
		}
		return
	}
	request, ok := r.requests.Load(requestId)
	if !ok {
		valkeyLog.Debug("ignoring unknown request")
		return
	}
	valkeyLog.Debug("received response %v", response)
	r.processResponse(request, response, requestId)
}

func (r *valkeyAdapter) processResponse(request *ValkeyRequest, response *Response, requestId string) {
	switch request.Type {
	case valkey.SOCKETS, valkey.REMOTE_FETCH:
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
	case valkey.ALL_ROOMS:
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
	case valkey.REMOTE_JOIN, valkey.REMOTE_LEAVE, valkey.REMOTE_DISCONNECT:
		utils.ClearTimeout(request.Timeout.Load())
		if request.Resolve != nil {
			request.Resolve(nil)
		}
		r.requests.Delete(requestId)
	case valkey.SERVER_SIDE_EMIT:
		request.Responses.Push(response.Data)
		valkeyLog.Debug("serverSideEmit: got %d responses out of %d", request.Responses.Len(), request.NumSub)
		if int64(request.Responses.Len()) == request.NumSub {
			utils.ClearTimeout(request.Timeout.Load())
			if request.Resolve != nil {
				request.Resolve(request.Responses)
			}
			r.requests.Delete(requestId)
		}
	default:
		valkeyLog.Debug("ignoring unknown request type: %d", request.Type)
	}
}

// Broadcast broadcasts a packet to all clients, optionally propagating to other nodes.
func (r *valkeyAdapter) Broadcast(packet *parser.Packet, opts *socket.BroadcastOptions) {
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
			valkeyLog.Debug("publishing message to channel %s", channel)
			if err := r.valkeyClient.Publish(r.valkeyClient.Context, channel, msg); err != nil {
				r.valkeyClient.Emit("error", err)
			}
		}
	}
	r.Adapter.Broadcast(packet, opts)
}

// BroadcastWithAck broadcasts a packet and handles acknowledgements from clients across all nodes.
func (r *valkeyAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	packet.Nsp = r.Nsp().Name()
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		requestId := adapter.Uid2(defaultUidLength)
		if request, err := r.parser.Encode(&Request{
			Uid:       r.uid,
			RequestId: requestId,
			Type:      valkey.BROADCAST,
			Packet:    packet,
			Opts:      adapter.EncodeOptions(opts),
		}); err == nil {
			if err := r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request); err != nil {
				r.valkeyClient.Emit("error", err)
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
	r.Adapter.BroadcastWithAck(packet, opts, clientCountCallback, ack)
}

// AllRooms returns all rooms across all cluster nodes.
func (r *valkeyAdapter) AllRooms() func(func(*types.Set[socket.Room], error)) {
	return func(cb func(*types.Set[socket.Room], error)) {
		localRooms := types.NewSet(r.Rooms().Keys()...)
		numSub := r.ServerCount()
		valkeyLog.Debug(`waiting for %d responses to "allRooms" request`, numSub)
		if numSub <= 1 {
			cb(localRooms, nil)
			return
		}

		requestId := adapter.Uid2(defaultUidLength)

		request, err := json.Marshal(&Request{Type: valkey.ALL_ROOMS, Uid: r.uid, RequestId: requestId})
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

		r.requests.Store(requestId, &ValkeyRequest{
			Type:   valkey.ALL_ROOMS,
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

		if err := r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request); err != nil {
			r.valkeyClient.Emit("error", err)
		}
	}
}

// FetchSockets retrieves sockets across all cluster nodes.
func (r *valkeyAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(cb func([]socket.SocketDetails, error)) {
		r.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, _ error) {
			if opts.Flags != nil && opts.Flags.Local {
				cb(localSockets, nil)
				return
			}

			numSub := r.ServerCount()
			valkeyLog.Debug(`waiting for %d responses to "fetchSockets" request`, numSub)

			if numSub <= 1 {
				cb(localSockets, nil)
				return
			}

			requestId := adapter.Uid2(defaultUidLength)

			request, err := json.Marshal(&Request{Type: valkey.REMOTE_FETCH, Uid: r.uid, RequestId: requestId, Opts: adapter.EncodeOptions(opts)})
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

			r.requests.Store(requestId, &ValkeyRequest{
				Type:   valkey.REMOTE_FETCH,
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

			if err := r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request); err != nil {
				r.valkeyClient.Emit("error", err)
			}
		})
	}
}

// AddSockets adds sockets matching the options to the specified rooms across all nodes.
func (r *valkeyAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.AddSockets(opts, rooms)
		return
	}
	request, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.REMOTE_JOIN, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
	if err != nil {
		valkeyLog.Debug("Error marshaling AddSockets request: %s", err.Error())
		return
	}
	if err := r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request); err != nil {
		r.valkeyClient.Emit("error", err)
	}
}

// DelSockets removes sockets matching the options from specified rooms across all nodes.
func (r *valkeyAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.DelSockets(opts, rooms)
		return
	}
	request, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.REMOTE_LEAVE, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
	if err != nil {
		valkeyLog.Debug("Error marshaling DelSockets request: %s", err.Error())
		return
	}
	if err := r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request); err != nil {
		r.valkeyClient.Emit("error", err)
	}
}

// DisconnectSockets disconnects sockets matching the options across all nodes.
func (r *valkeyAdapter) DisconnectSockets(opts *socket.BroadcastOptions, close bool) {
	if opts != nil && opts.Flags != nil && opts.Flags.Local {
		r.Adapter.DisconnectSockets(opts, close)
		return
	}
	request, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.REMOTE_DISCONNECT, Opts: adapter.EncodeOptions(opts), Close: close})
	if err != nil {
		valkeyLog.Debug("Error marshaling DisconnectSockets request: %s", err.Error())
		return
	}
	if err := r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request); err != nil {
		r.valkeyClient.Emit("error", err)
	}
}

// ServerSideEmit emits a packet to all servers in the cluster.
func (r *valkeyAdapter) ServerSideEmit(packet []any) error {
	if len(packet) == 0 {
		return errors.New("packet cannot be empty")
	}

	if ack, withAck := packet[len(packet)-1].(socket.Ack); withAck {
		return r.serverSideEmitWithAck(packet[:len(packet)-1], ack)
	}

	request, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal ServerSideEmit request: %w", err)
	}

	return r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request)
}

func (r *valkeyAdapter) serverSideEmitWithAck(packet []any, ack socket.Ack) error {
	numSub := r.ServerCount() - 1
	valkeyLog.Debug(`waiting for %d responses to "serverSideEmit" request`, numSub)
	if numSub <= 0 {
		ack(nil, nil)
		return nil
	}

	requestId := adapter.Uid2(defaultUidLength)

	request, err := json.Marshal(&Request{Uid: r.uid, RequestId: requestId, Type: valkey.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal serverSideEmitWithAck request: %w", err)
	}

	timeout := utils.SetTimeout(func() {
		if storedRequest, ok := r.requests.Load(requestId); ok {
			ack(storedRequest.Responses.All(), fmt.Errorf("timeout reached: only %d responses received out of %d", storedRequest.Responses.Len(), storedRequest.NumSub))
			r.requests.Delete(requestId)
		}
	}, r.requestsTimeout)

	r.requests.Store(requestId, &ValkeyRequest{
		Type:   valkey.SERVER_SIDE_EMIT,
		NumSub: numSub,
		Timeout: utils.Tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
			t.Store(timeout)
		}),
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Responses: types.NewSlice[any](),
	})

	return r.valkeyClient.Publish(r.valkeyClient.Context, r.requestChannel, request)
}

// ServerCount returns the number of servers subscribed to the request channel.
func (r *valkeyAdapter) ServerCount() int64 {
	result, err := r.valkeyClient.PubSubNumSub(r.valkeyClient.Context, r.requestChannel)
	if err != nil {
		r.valkeyClient.Emit("error", err)
		return 0
	}
	if count, ok := result[r.requestChannel]; ok {
		return count
	}
	return 0
}

// Close cleans up Valkey subscriptions and listeners.
func (r *valkeyAdapter) Close() {
	if psub, ok := r.valkeyListeners.LoadAndDelete(subKeyPattern); ok {
		if err := psub.PUnsubscribe(r.valkeyClient.Context, r.channel+"*"); err != nil {
			r.valkeyClient.Emit("error", err)
		}
		_ = psub.Close()
	}
	if sub, ok := r.valkeyListeners.LoadAndDelete(subKeyChannel); ok {
		if err := sub.Unsubscribe(r.valkeyClient.Context, r.requestChannel, r.responseChannel, r.specificResponseChannel); err != nil {
			r.valkeyClient.Emit("error", err)
		}
		_ = sub.Close()
	}
	r.valkeyClient.RemoveListener("error", r.friendlyErrorHandler)
	r.Adapter.Close()
}
