// Package adapter provides a Valkey-based adapter implementation for Socket.IO clustering.
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
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var valkeyLog = log.NewLog("socket.io-valkey")

var errValkeyPublishPanicked = errors.New("valkey publish panicked")

const (
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

		requests    types.Map[string, *ValkeyRequest]
		ackRequests types.Map[string, *AckRequest]
		publisher   *queue.Queue
		responses   *queue.Queue
		queueMu     sync.Mutex

		pubSub                *classicValkeyPubSub
		broadcastSubscription *classicValkeySubscription
		requestSubscription   *classicValkeySubscription

		ctx       context.Context
		cancel    context.CancelFunc
		closeOnce sync.Once
	}
)

// New creates a new ValkeyAdapter for the given namespace.
func (vb *ValkeyAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewValkeyAdapter(nsp, vb.Valkey, vb.Opts)
}

// MakeValkeyAdapter creates a new uninitialized valkeyAdapter with default options.
func MakeValkeyAdapter() ValkeyAdapter {
	c := &valkeyAdapter{
		Adapter:   socket.MakeAdapter(),
		opts:      DefaultValkeyAdapterOptions(),
		publisher: queue.New(),
		responses: queue.New(),
	}

	c.Prototype(c)

	return c
}

func (r *valkeyAdapter) registerRequest(requestId string, request *ValkeyRequest, timeout time.Duration, onTimeout func()) {
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

func (r *valkeyAdapter) finishRequest(requestId string, request *ValkeyRequest) bool {
	if !r.requests.CompareAndDelete(requestId, request) {
		return false
	}
	utils.ClearTimeout(request.Timeout.Swap(nil))
	return request.Resolve != nil
}

func (r *valkeyAdapter) registerAckRequest(requestId string, request *AckRequest, timeout time.Duration) {
	r.ackRequests.Store(requestId, request)
	timeoutCtx, cancel := context.WithTimeout(r.ctx, timeout)
	context.AfterFunc(timeoutCtx, func() {
		defer cancel()
		r.ackRequests.CompareAndDelete(requestId, request)
	})
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

	r.ctx, r.cancel = context.WithCancel(r.valkeyClient.Context())
	r.uid = adapter.ServerId(adapter.Uid2(defaultUidLength))

	r.requestsTimeout = r.opts.RequestsTimeout()
	if r.requestsTimeout == 0 {
		r.requestsTimeout = DefaultRequestsTimeout
	}

	r.publishOnSpecificResponseChannel = r.opts.PublishOnSpecificResponseChannel()

	r.parser = r.opts.Parser()
	if utils.IsNil(r.parser) {
		r.parser = utils.MsgPack()
	}

	prefix := utils.Value(r.opts.Key(), defaultChannelPrefix)

	r.channel = prefix + "#" + nsp.Name() + "#"
	r.requestChannel = prefix + "-request#" + r.Nsp().Name() + "#"
	r.responseChannel = prefix + "-response#" + r.Nsp().Name() + "#"
	r.specificResponseChannel = r.responseChannel + string(r.uid) + "#"

	r.pubSub = classicValkeyPubSubs.acquire(r.valkeyClient)
	r.broadcastSubscription = r.pubSub.newSubscription(r.onMessage)
	r.requestSubscription = r.pubSub.newSubscription(r.onRequest)
	r.broadcastSubscription.PSubscribe(r.channel + "*")
	r.requestSubscription.Subscribe(r.requestChannel, r.responseChannel, r.specificResponseChannel)
	if err := r.pubSub.flush(r.ctx); err != nil && r.ctx.Err() == nil {
		r.valkeyClient.Emit("error", err)
	}
}

func (r *valkeyAdapter) onMessage(msg []byte, channel string) {
	if r.ctx.Err() != nil {
		return
	}
	if !strings.HasPrefix(channel, r.channel) {
		valkeyLog.Debug("ignore different channel")
		return
	}

	room := ""
	if len(channel) > len(r.channel) {
		room = channel[len(r.channel) : len(channel)-1]
	}
	if room != "" && !r.hasRoom(socket.Room(room)) {
		valkeyLog.Debug("ignore unknown room %s", room)
		return
	}

	var packet Packet
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
	if !packet.Opts.IsValid() {
		valkeyLog.Debug("ignoring malformed broadcast options")
		return
	}
	r.Adapter.Broadcast(packet.Packet, adapter.DecodeOptions(packet.Opts))
}

func (r *valkeyAdapter) hasRoom(room socket.Room) bool {
	_, ok := r.Rooms().Load(room)
	return ok
}

func (r *valkeyAdapter) onRequest(msg []byte, channel string) {
	if r.ctx.Err() != nil {
		return
	}
	if strings.HasPrefix(channel, r.responseChannel) {
		r.onResponse(msg)
		return
	}
	if !strings.HasPrefix(channel, r.requestChannel) {
		valkeyLog.Debug("ignore different channel")
		return
	}

	var request Request
	var err error
	if len(msg) > 0 && msg[0] == '{' {
		err = json.Unmarshal(msg, &request)
	} else {
		err = r.parser.Decode(msg, &request)
	}
	if err != nil {
		valkeyLog.Debug("ignoring malformed request")
		return
	}
	valkeyLog.Debug("received request type %d with id %s", request.Type, request.RequestId)
	if request.Uid != "" && request.Uid == r.uid {
		valkeyLog.Debug("ignore same uid")
		return
	}
	r.handleRequest(&request)
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
	sockets := r.Sockets(types.NewSet(request.Rooms...))
	r.publishJSONResponse(request, &Response{
		RequestId: request.RequestId,
		Sockets:   utils.NonNilSlice(sockets.Keys()),
	})
}

func (r *valkeyAdapter) handleAllRoomsRequest(request *Request) {
	r.publishJSONResponse(request, &Response{
		RequestId: request.RequestId,
		Rooms:     utils.NonNilSlice(r.Rooms().Keys()),
	})
}

func (r *valkeyAdapter) handleRemoteJoinRequest(request *Request) {
	if !request.Opts.IsValid() {
		valkeyLog.Debug("ignoring malformed REMOTE_JOIN request")
		return
	}
	r.Adapter.AddSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
}

func (r *valkeyAdapter) handleRemoteLeaveRequest(request *Request) {
	if !request.Opts.IsValid() {
		valkeyLog.Debug("ignoring malformed REMOTE_LEAVE request")
		return
	}
	r.Adapter.DelSockets(adapter.DecodeOptions(request.Opts), request.Rooms)
}

func (r *valkeyAdapter) handleRemoteDisconnectRequest(request *Request) {
	if !request.Opts.IsValid() {
		valkeyLog.Debug("ignoring malformed REMOTE_DISCONNECT request")
		return
	}
	r.Adapter.DisconnectSockets(adapter.DecodeOptions(request.Opts), utils.FromPtr(request.Close))
}

func (r *valkeyAdapter) handleRemoteFetchRequest(request *Request) {
	if !request.Opts.IsValid() {
		valkeyLog.Debug("ignoring malformed REMOTE_FETCH request")
		return
	}
	r.Adapter.FetchSockets(adapter.DecodeOptions(request.Opts))(func(localSockets []socket.SocketDetails, err error) {
		if err != nil {
			valkeyLog.Debug("REMOTE_FETCH Adapter.FetchSockets error: %s", err.Error())
			return
		}
		r.publishJSONResponse(request, &Response{
			RequestId: request.RequestId,
			Sockets:   adapter.SocketDetailsToResponses(localSockets),
		})
	})
}

func (r *valkeyAdapter) handleServerSideEmitRequest(request *Request) {
	if request.RequestId == "" {
		r.Nsp().OnServerSideEmit(request.Data)
		return
	}

	var called atomic.Bool
	callback := func(args []any, _ error) {
		if !called.CompareAndSwap(false, true) {
			return
		}
		valkeyLog.Debug("calling acknowledgement with %v", args)
		response, err := json.Marshal(&Response{
			Type:      valkey.SERVER_SIDE_EMIT,
			RequestId: request.RequestId,
			Data:      slices.TryGet(args, 0),
		})
		if err != nil {
			valkeyLog.Debug("Error marshaling SERVER_SIDE_EMIT response for RequestId %s: %s", request.RequestId, err.Error())
			return
		}
		r.publishResponseMessage(r.responseChannel, response)
	}
	r.Nsp().OnServerSideEmit(slices.AppendCopy(request.Data, callback))
}

func (r *valkeyAdapter) handleBroadcastRequest(request *Request) {
	if request.Uid == "" || request.RequestId == "" || request.Packet == nil || !request.Opts.IsValid() {
		valkeyLog.Debug("ignoring malformed BROADCAST request")
		return
	}
	r.Adapter.BroadcastWithAck(
		request.Packet,
		adapter.DecodeOptions(request.Opts),
		func(clientCount uint64) {
			valkeyLog.Debug("waiting for %d client acknowledgements", clientCount)
			r.publishJSONResponse(request, &Response{
				Type:        valkey.BROADCAST_CLIENT_COUNT,
				RequestId:   request.RequestId,
				ClientCount: &clientCount,
			})
		},
		func(args []any, _ error) {
			valkeyLog.Debug("received acknowledgement with value %v", args)
			response, err := r.parser.Encode(&Response{
				Type:      valkey.BROADCAST_ACK,
				RequestId: request.RequestId,
				Packet:    valkey.NormalizeData(slices.TryGet(args, 0)),
			})
			if err != nil {
				valkeyLog.Debug("Error marshaling BROADCAST_ACK response for RequestId %s: %s", request.RequestId, err.Error())
				return
			}
			r.publishResponse(request, response)
		},
	)
}

func (r *valkeyAdapter) publish(channel string, message []byte) error {
	result := make(chan error, 1)
	if err := r.enqueuePublish(r.publisher, channel, message, result); err != nil {
		return err
	}
	err, ok := <-result
	if !ok {
		return errValkeyPublishPanicked
	}
	return err
}

func (r *valkeyAdapter) publishAsync(channel string, message []byte) {
	_ = r.enqueuePublish(r.publisher, channel, message, nil)
}

func (r *valkeyAdapter) enqueuePublish(target *queue.Queue, channel string, message []byte, result chan<- error) error {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	if target.IsShuttingDown() {
		return adapter.ErrAdapterClosed
	}
	target.Enqueue(func() {
		if result != nil {
			defer close(result)
		}
		err := r.valkeyClient.Publish(r.valkeyClient.Context(), channel, message)
		if err != nil {
			go r.valkeyClient.Emit("error", err)
		}
		if result != nil {
			result <- err
		}
	})
	return nil
}

func (r *valkeyAdapter) publishResponseMessage(channel string, message []byte) {
	_ = r.enqueuePublish(r.responses, channel, message, nil)
}

func (r *valkeyAdapter) publishJSONResponse(request *Request, response *Response) {
	message, err := json.Marshal(response)
	if err != nil {
		valkeyLog.Debug("Error marshaling response for request type %d with RequestId %s: %s", request.Type, request.RequestId, err.Error())
		return
	}
	r.publishResponse(request, message)
}

func (r *valkeyAdapter) publishResponse(request *Request, response []byte) {
	channel := r.responseChannel
	if r.publishOnSpecificResponseChannel {
		channel = r.responseChannel + string(request.Uid) + "#"
	}

	valkeyLog.Debug("publishing response to channel %s", channel)
	r.publishResponseMessage(channel, response)
}

func (r *valkeyAdapter) onResponse(msg []byte) {
	var response Response
	var err error
	if len(msg) > 0 && msg[0] == '{' {
		err = json.Unmarshal(msg, &response)
	} else {
		err = r.parser.Decode(msg, &response)
	}
	if err != nil {
		valkeyLog.Debug("ignoring malformed response")
		return
	}
	if response.RequestId == "" {
		valkeyLog.Debug("ignoring response without requestId")
		return
	}

	requestId := response.RequestId
	if ackRequest, ok := r.ackRequests.Load(requestId); ok {
		switch response.Type {
		case valkey.BROADCAST_CLIENT_COUNT:
			if response.ClientCount == nil || ackRequest.ClientCountCallback == nil {
				valkeyLog.Debug("ignoring malformed BROADCAST_CLIENT_COUNT response")
				return
			}
			ackRequest.ClientCountCallback(*response.ClientCount)
		case valkey.BROADCAST_ACK:
			if ackRequest.Ack != nil {
				ackRequest.Ack([]any{response.Packet}, nil)
			}
		}
		return
	}
	request, ok := r.requests.Load(requestId)
	if !ok {
		valkeyLog.Debug("ignoring unknown request")
		return
	}
	valkeyLog.Debug("received response %v", response)
	r.processResponse(request, &response)
}

func (r *valkeyAdapter) processResponse(request *ValkeyRequest, response *Response) {
	requestId := response.RequestId
	switch request.Type {
	case valkey.SOCKETS:
		var socketIds []socket.SocketId
		socketsPayload, ok := response.Sockets.(json.RawMessage)
		if !ok {
			valkeyLog.Debug("ignoring malformed SOCKETS response")
			return
		}
		if err := json.Unmarshal(socketsPayload, &socketIds); err != nil || socketIds == nil {
			valkeyLog.Debug("ignoring malformed SOCKETS response")
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
	case valkey.REMOTE_FETCH:
		var sockets []adapter.SocketResponse
		socketsPayload, ok := response.Sockets.(json.RawMessage)
		if !ok {
			valkeyLog.Debug("ignoring malformed REMOTE_FETCH response")
			return
		}
		if err := json.Unmarshal(socketsPayload, &sockets); err != nil || sockets == nil {
			valkeyLog.Debug("ignoring malformed REMOTE_FETCH response")
			return
		}
		if len(sockets) > 0 {
			request.Responses.Push(adapter.SocketResponsesToDetailsAny(sockets)...)
		}
		msgCount := request.MsgCount.Add(1)
		if msgCount == request.NumSub && r.finishRequest(requestId, request) {
			request.Resolve(request.Responses)
		}
	case valkey.ALL_ROOMS:
		if response.Rooms == nil {
			valkeyLog.Debug("ignoring malformed ALL_ROOMS response")
			return
		}
		request.Rooms.Add(response.Rooms...)
		msgCount := request.MsgCount.Add(1)
		if msgCount == request.NumSub && r.finishRequest(requestId, request) {
			request.Resolve(nil)
		}
	case valkey.REMOTE_JOIN, valkey.REMOTE_LEAVE, valkey.REMOTE_DISCONNECT:
		if r.finishRequest(requestId, request) {
			request.Resolve(nil)
		}
	case valkey.SERVER_SIDE_EMIT:
		responseCount := request.Responses.Push(response.Data)
		valkeyLog.Debug("serverSideEmit: got %d responses out of %d", responseCount, request.NumSub)
		if int64(responseCount) == request.NumSub && r.finishRequest(requestId, request) {
			request.Resolve(request.Responses)
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
		packetOpts := adapter.EncodeOptions(opts)
		msg, err := r.parser.Encode(&Packet{
			Uid:    r.Uid(),
			Packet: packet,
			Opts:   packetOpts,
		})
		if err != nil {
			r.valkeyClient.Emit("error", err)
			return
		}

		channel := r.channel
		if len(packetOpts.Rooms) == 1 {
			channel += string(packetOpts.Rooms[0]) + "#"
		}
		valkeyLog.Debug("publishing message to channel %s", channel)
		r.publishAsync(channel, msg)
	}
	r.Adapter.Broadcast(packet, opts)
}

// BroadcastWithAck broadcasts a packet and handles acknowledgements from clients across all nodes.
func (r *valkeyAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	packet.Nsp = r.Nsp().Name()
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		requestId := adapter.Uid2(defaultUidLength)
		message, err := r.parser.Encode(&Request{
			Uid:       r.uid,
			RequestId: requestId,
			Type:      valkey.BROADCAST,
			Packet:    packet,
			Opts:      adapter.EncodeOptions(opts),
		})
		if err != nil {
			r.valkeyClient.Emit("error", err)
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
func (r *valkeyAdapter) AllRooms() func(func(*types.Set[socket.Room], error)) {
	return func(cb func(*types.Set[socket.Room], error)) {
		localRooms := types.NewSet(r.Rooms().Keys()...)
		numSub, err := r.ServerCount()
		if err != nil {
			cb(nil, err)
			return
		}
		valkeyLog.Debug(`waiting for %d responses to "allRooms" request`, numSub)
		if numSub <= 1 {
			cb(localRooms, nil)
			return
		}

		requestId := adapter.Uid2(defaultUidLength)

		message, err := json.Marshal(&Request{Type: valkey.ALL_ROOMS, Uid: r.uid, RequestId: requestId})
		if err != nil {
			cb(nil, err)
			return
		}

		request := &ValkeyRequest{
			Type:   valkey.ALL_ROOMS,
			NumSub: numSub,
			Resolve: func(*types.Slice[any]) {
				cb(localRooms, nil)
			},
			Rooms: localRooms,
		}
		request.MsgCount.Store(1)
		r.registerRequest(requestId, request, r.requestsTimeout, func() {
			cb(nil, errors.New("timeout reached while waiting for allRooms response"))
		})

		if err := r.publish(r.requestChannel, message); err != nil && r.finishRequest(requestId, request) {
			cb(nil, err)
		}
	}
}

// FetchSockets retrieves sockets across all cluster nodes.
func (r *valkeyAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(cb func([]socket.SocketDetails, error)) {
		r.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, err error) {
			if err != nil {
				cb(nil, err)
				return
			}
			if opts != nil && opts.Flags != nil && opts.Flags.Local {
				cb(localSockets, nil)
				return
			}

			numSub, err := r.ServerCount()
			if err != nil {
				cb(nil, err)
				return
			}
			valkeyLog.Debug(`waiting for %d responses to "fetchSockets" request`, numSub)

			if numSub <= 1 {
				cb(localSockets, nil)
				return
			}

			requestId := adapter.Uid2(defaultUidLength)

			message, err := json.Marshal(&Request{Type: valkey.REMOTE_FETCH, Uid: r.uid, RequestId: requestId, Opts: adapter.EncodeOptions(opts)})
			if err != nil {
				cb(nil, err)
				return
			}

			request := &ValkeyRequest{
				Type:   valkey.REMOTE_FETCH,
				NumSub: numSub,
				Resolve: func(data *types.Slice[any]) {
					cb(adapter.AnySliceToSocketDetails(data.All()), nil)
				},
				Responses: types.NewSlice(adapter.SocketDetailsToAny(localSockets)...),
			}
			request.MsgCount.Store(1)
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
func (r *valkeyAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		message, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.REMOTE_JOIN, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
		if err != nil {
			valkeyLog.Debug("Error marshaling AddSockets request: %s", err.Error())
		} else {
			r.publishAsync(r.requestChannel, message)
		}
	}
	r.Adapter.AddSockets(opts, rooms)
}

// DelSockets removes sockets matching the options from specified rooms across all nodes.
func (r *valkeyAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		message, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.REMOTE_LEAVE, Opts: adapter.EncodeOptions(opts), Rooms: rooms})
		if err != nil {
			valkeyLog.Debug("Error marshaling DelSockets request: %s", err.Error())
		} else {
			r.publishAsync(r.requestChannel, message)
		}
	}
	r.Adapter.DelSockets(opts, rooms)
}

// DisconnectSockets disconnects sockets matching the options across all nodes.
func (r *valkeyAdapter) DisconnectSockets(opts *socket.BroadcastOptions, close bool) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		message, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.REMOTE_DISCONNECT, Opts: adapter.EncodeOptions(opts), Close: &close})
		if err != nil {
			valkeyLog.Debug("Error marshaling DisconnectSockets request: %s", err.Error())
		} else {
			r.publishAsync(r.requestChannel, message)
		}
	}
	r.Adapter.DisconnectSockets(opts, close)
}

// ServerSideEmit emits a packet to all servers in the cluster.
func (r *valkeyAdapter) ServerSideEmit(packet []any) error {
	if len(packet) > 0 {
		if ack, withAck := packet[len(packet)-1].(socket.Ack); withAck {
			return r.serverSideEmitWithAck(packet[:len(packet)-1], ack)
		}
	}

	request, err := json.Marshal(&Request{Uid: r.uid, Type: valkey.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal ServerSideEmit request: %w", err)
	}

	return r.publish(r.requestChannel, request)
}

func (r *valkeyAdapter) serverSideEmitWithAck(packet []any, ack socket.Ack) error {
	serverCount, err := r.ServerCount()
	if err != nil {
		return err
	}
	numSub := serverCount - 1
	valkeyLog.Debug(`waiting for %d responses to "serverSideEmit" request`, numSub)
	if numSub <= 0 {
		ack([]any{}, nil)
		return nil
	}

	requestId := adapter.Uid2(defaultUidLength)

	message, err := json.Marshal(&Request{Uid: r.uid, RequestId: requestId, Type: valkey.SERVER_SIDE_EMIT, Data: packet})
	if err != nil {
		return fmt.Errorf("failed to marshal serverSideEmitWithAck request: %w", err)
	}

	request := &ValkeyRequest{
		Type:   valkey.SERVER_SIDE_EMIT,
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
func (r *valkeyAdapter) ServerCount() (int64, error) {
	result, err := r.valkeyClient.PubSubNumSub(r.ctx, r.requestChannel)
	if err != nil {
		return 0, err
	}
	return result[r.requestChannel], nil
}

// Close cleans up Valkey subscriptions and listeners.
func (r *valkeyAdapter) Close() {
	r.closeOnce.Do(r.close)
}

func (r *valkeyAdapter) close() {
	r.queueMu.Lock()
	r.responses.TryClose()
	r.publisher.TryClose()
	r.queueMu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	if r.requestSubscription != nil {
		r.requestSubscription.Close()
	}
	if r.broadcastSubscription != nil {
		r.broadcastSubscription.Close()
	}
	if r.pubSub != nil {
		classicValkeyPubSubs.release(r.valkeyClient, r.pubSub)
	}
	r.Adapter.Close()
}
