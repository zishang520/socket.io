package adapter

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// ErrAdapterClosed is returned when a cluster publish is attempted after the adapter is closed.
var ErrAdapterClosed = errors.New("adapter is closed")

var errClusterPublishPanicked = errors.New("cluster publish panicked")

// ClusterAdapterBuilder is a builder for creating ClusterAdapter instances.
//
// A cluster-ready adapter. Any extending interface must:
//   - implement ClusterAdapter.PreparePublish and ClusterAdapter.PreparePublishResponse
//   - call ClusterAdapter.OnMessage and ClusterAdapter.OnResponse
type (
	ClusterAdapterBuilder struct{}

	// clusterAdapter implements the ClusterAdapter interface for cluster communication.
	clusterAdapter struct {
		Adapter

		// uid is the unique server identifier.
		uid ServerId

		requests    types.Map[string, *ClusterRequest]
		ackRequests types.Map[string, *ClusterAckRequest]
		queueMu     sync.Mutex
		publisher   *queue.Queue
		responses   *queue.Queue
	}
)

// New creates a new ClusterAdapter for the given Namespace.
func (cb *ClusterAdapterBuilder) New(nsp socket.Namespace) Adapter {
	return NewClusterAdapter(nsp)
}

// MakeClusterAdapter returns a new default ClusterAdapter instance.
func MakeClusterAdapter() ClusterAdapter {
	c := &clusterAdapter{
		Adapter:   MakeAdapter(),
		publisher: queue.New(),
		responses: queue.New(),
	}
	c.Prototype(c)
	return c
}

// NewClusterAdapter creates a new ClusterAdapter for the given Namespace.
func NewClusterAdapter(nsp socket.Namespace) ClusterAdapter {
	c := MakeClusterAdapter()
	c.Construct(nsp)
	return c
}

// finishClusterRequest atomically claims a pending request and releases its timeout.
func finishClusterRequest[T any](requests *types.Map[string, *T], requestId string, request *T, timeout *atomic.Pointer[utils.Timer]) bool {
	if !requests.CompareAndDelete(requestId, request) {
		return false
	}
	utils.ClearTimeout(timeout.Swap(nil))
	return true
}

// Uid returns the unique server identifier.
func (c *clusterAdapter) Uid() ServerId {
	return c.uid
}

// Construct initializes the clusterAdapter with the given Namespace.
func (c *clusterAdapter) Construct(nsp socket.Namespace) {
	c.Adapter.Construct(nsp)
	c.uid = ServerId(RandomId())
}

// OnMessage handles incoming messages
func (c *clusterAdapter) OnMessage(message *ClusterMessage, offset Offset) {
	if message.Uid == c.uid {
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] ignore message from self", c.uid)
		}
		return
	}

	if message.Nsp != c.Nsp().Name() {
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] ignore message from another namespace (%s)", c.uid, message.Nsp)
		}
		return
	}

	if log.DEBUG.Load() {
		adapterLog.Debug("[%s] new event of type %d from %s", c.uid, message.Type, message.Uid)
	}

	switch message.Type {
	case BROADCAST:
		data, ok := message.Data.(*BroadcastMessage)
		if !ok || data == nil || data.Packet == nil || !data.Opts.IsValid() {
			adapterLog.Debug("[%s] invalid data for BROADCAST message", c.uid)
			return
		}

		opts := DecodeOptions(data.Opts)
		if data.RequestId != nil {
			c.Adapter.BroadcastWithAck(
				data.Packet,
				opts,
				func(clientCount uint64) {
					if log.DEBUG.Load() {
						adapterLog.Debug("[%s] waiting for %d client acknowledgements", c.uid, clientCount)
					}
					c.PublishResponse(message.Uid, &ClusterResponse{
						Type: BROADCAST_CLIENT_COUNT,
						Data: &BroadcastClientCount{
							RequestId:   *data.RequestId,
							ClientCount: clientCount,
						},
					})
				},
				func(args []any, _ error) {
					if log.DEBUG.Load() {
						adapterLog.Debug("[%s] received acknowledgement with value %v", c.uid, args)
					}
					c.PublishResponse(message.Uid, &ClusterResponse{
						Type: BROADCAST_ACK,
						Data: &BroadcastAck{
							RequestId: *data.RequestId,
							Packet:    slices.TryGet(args, 0),
						},
					})
				},
			)
		} else {
			c.addOffsetIfNecessary(data.Packet, opts, offset)
			c.Adapter.Broadcast(data.Packet, opts)
		}

	case SOCKETS_JOIN:
		data, ok := message.Data.(*SocketsJoinLeaveMessage)
		if !ok || data == nil || !data.Opts.IsValid() {
			adapterLog.Debug("[%s] invalid data for SOCKETS_JOIN message", c.uid)
			return
		}
		c.Adapter.AddSockets(DecodeOptions(data.Opts), data.Rooms)

	case SOCKETS_LEAVE:
		data, ok := message.Data.(*SocketsJoinLeaveMessage)
		if !ok || data == nil || !data.Opts.IsValid() {
			adapterLog.Debug("[%s] invalid data for SOCKETS_LEAVE message", c.uid)
			return
		}
		c.Adapter.DelSockets(DecodeOptions(data.Opts), data.Rooms)

	case DISCONNECT_SOCKETS:
		data, ok := message.Data.(*DisconnectSocketsMessage)
		if !ok || data == nil || !data.Opts.IsValid() {
			adapterLog.Debug("[%s] invalid data for DISCONNECT_SOCKETS message", c.uid)
			return
		}
		c.Adapter.DisconnectSockets(DecodeOptions(data.Opts), data.Close)

	case FETCH_SOCKETS:
		data, ok := message.Data.(*FetchSocketsMessage)
		if !ok || data == nil || !data.Opts.IsValid() {
			adapterLog.Debug("[%s] invalid data for FETCH_SOCKETS message", c.uid)
			return
		}
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] calling fetchSockets with opts %v", c.uid, data.Opts)
		}

		c.Adapter.FetchSockets(DecodeOptions(data.Opts))(
			func(localSockets []socket.SocketDetails, err error) {
				if err != nil {
					adapterLog.Debug("FETCH_SOCKETS Adapter.OnMessage error: %s", err.Error())
					return
				}

				c.PublishResponse(message.Uid, &ClusterResponse{
					Type: FETCH_SOCKETS_RESPONSE,
					Data: &FetchSocketsResponse{
						RequestId: data.RequestId,
						Sockets:   SocketDetailsToResponses(localSockets),
					},
				})
			},
		)

	case SERVER_SIDE_EMIT:
		data, ok := message.Data.(*ServerSideEmitMessage)
		if !ok {
			adapterLog.Debug("[%s] invalid data for SERVER_SIDE_EMIT message", c.uid)
			return
		}
		packet := data.Packet
		if data.RequestId == nil {
			c.Nsp().OnServerSideEmit(packet)
			return
		}

		called := new(atomic.Bool)
		callback := func(arg []any, _ error) {
			// Claim before encoding: a user marshaler may invoke this ACK again.
			if !called.CompareAndSwap(false, true) {
				return
			}
			if log.DEBUG.Load() {
				adapterLog.Debug("[%s] calling acknowledgement with %v", c.uid, arg)
			}
			c.PublishResponse(message.Uid, &ClusterResponse{
				Type: SERVER_SIDE_EMIT_RESPONSE,
				Data: &ServerSideEmitResponse{
					RequestId: *data.RequestId,
					Packet:    slices.TryGet(arg, 0),
				},
			})
		}

		c.Nsp().OnServerSideEmit(slices.AppendCopy(packet, callback))

	case BROADCAST_CLIENT_COUNT, BROADCAST_ACK, FETCH_SOCKETS_RESPONSE, SERVER_SIDE_EMIT_RESPONSE:
		// extending classes may not make a distinction between a ClusterMessage and a ClusterResponse payload and may
		// always call the OnMessage() method
		c.Proto().(ClusterAdapter).OnResponse(message)
	default:
		adapterLog.Debug("[%s] unknown message type: %d", c.uid, message.Type)
	}
}

// OnResponse handles incoming responses
func (c *clusterAdapter) OnResponse(response *ClusterResponse) {
	switch response.Type {
	case BROADCAST_CLIENT_COUNT:
		if data, ok := response.Data.(*BroadcastClientCount); ok {
			if log.DEBUG.Load() {
				adapterLog.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
			}
			if ackRequest, ok := c.ackRequests.Load(data.RequestId); ok {
				if ackRequest.Sockets.Add(response.Uid) {
					ackRequest.ClientCountCallback(data.ClientCount)
				}
			}
		} else {
			adapterLog.Debug("[%s] invalid data for BROADCAST_CLIENT_COUNT message", c.uid)
		}

	case BROADCAST_ACK:
		if data, ok := response.Data.(*BroadcastAck); ok {
			if log.DEBUG.Load() {
				adapterLog.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
			}
			if ackRequest, ok := c.ackRequests.Load(data.RequestId); ok {
				ackRequest.Ack([]any{data.Packet}, nil)
			}
		} else {
			adapterLog.Debug("[%s] invalid data for BROADCAST_ACK message", c.uid)
		}

	case FETCH_SOCKETS_RESPONSE:
		data, ok := response.Data.(*FetchSocketsResponse)
		if !ok {
			adapterLog.Debug("[%s] invalid data for FETCH_SOCKETS_RESPONSE message", c.uid)
			return
		}
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
		}

		c.collectResponse(data.RequestId, response.Uid, SocketResponsesToDetailsAny(data.Sockets)...)

	case SERVER_SIDE_EMIT_RESPONSE:
		data, ok := response.Data.(*ServerSideEmitResponse)
		if !ok {
			adapterLog.Debug("[%s] invalid data for SERVER_SIDE_EMIT_RESPONSE message", c.uid)
			return
		}
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
		}

		c.collectResponse(data.RequestId, response.Uid, data.Packet)
	default:
		adapterLog.Debug("[%s] unknown response type: %d", c.uid, response.Type)
	}
}

func (c *clusterAdapter) collectResponse(requestId string, uid ServerId, values ...any) {
	request, ok := c.requests.Load(requestId)
	if !ok {
		return
	}
	var complete bool
	request.Responses.DoWrite(func(responses []any) []any {
		if current, ok := c.requests.Load(requestId); !ok || current != request {
			return responses
		}
		if !request.Sockets.Add(uid) {
			return responses
		}
		responses = append(responses, values...)
		if request.Current.Add(1) == request.Expected {
			complete = finishClusterRequest(&c.requests, requestId, request, request.Timeout)
		}
		return responses
	})
	if complete {
		request.Resolve(request.Responses)
	}
}

func (c *clusterAdapter) Broadcast(packet *parser.Packet, opts *socket.BroadcastOptions) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		if _, err := prepareClusterPacket(packet); err != nil {
			c.Emit("error", err)
			return
		}
		offset, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: BROADCAST,
			Data: &BroadcastMessage{
				Packet: packet,
				Opts:   EncodeOptions(opts),
			},
		})
		if err != nil {
			adapterLog.Debug("[%s] error while broadcasting message: %s", c.uid, err.Error())
		} else {
			c.addOffsetIfNecessary(packet, opts, offset)
		}
	}

	c.Adapter.Broadcast(packet, opts)
}

// Adds an offset at the end of the data array in order to allow the client to receive any missed packets when it
// reconnects after a temporary disconnection.
func (c *clusterAdapter) addOffsetIfNecessary(packet *parser.Packet, opts *socket.BroadcastOptions, offset Offset) {
	if c.Nsp().Server().Opts().ConnectionStateRecovery() == nil {
		return
	}

	isEventPacket := packet.Type == parser.EVENT
	// packets with acknowledgement are not stored because the acknowledgement function cannot be serialized and
	// restored on another server upon reconnection
	withoutAcknowledgement := packet.Id == nil
	notVolatile := opts == nil || opts.Flags == nil || !opts.Flags.Volatile

	if isEventPacket && withoutAcknowledgement && notVolatile {
		packet.Data = append(utils.TryCast[[]any](packet.Data), offset)
	}
}

func (c *clusterAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	if !onlyLocal {
		if _, err := prepareClusterPacket(packet); err != nil {
			ack(nil, err)
			clientCountCallback(0)
			return
		}
		requestId := RandomId()

		c.ackRequests.Store(requestId, &ClusterAckRequest{
			ClientCountCallback: clientCountCallback,
			Ack:                 ack,
		})

		var timeout time.Duration
		if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
			timeout = utils.NormalizeTimerMilliseconds(*opts.Flags.Timeout)
		}

		// This layer cannot count all client ACKs, so expire the request by timeout.
		// Start before Publish: synchronous user encoding may block.
		utils.SetTimeout(func() {
			c.ackRequests.Delete(requestId)
		}, timeout)

		c.Proto().(ClusterAdapter).Publish(&ClusterMessage{
			Type: BROADCAST,
			Data: &BroadcastMessage{
				Packet:    packet,
				RequestId: new(requestId),
				Opts:      EncodeOptions(opts),
			},
		})
	}

	c.Adapter.BroadcastWithAck(packet, opts, clientCountCallback, ack)
}

func (c *clusterAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		_, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: SOCKETS_JOIN,
			Data: &SocketsJoinLeaveMessage{
				Opts:  EncodeOptions(opts),
				Rooms: rooms,
			},
		})
		if err != nil {
			adapterLog.Debug("[%s] error while publishing message: %s", c.uid, err.Error())
		}
	}
	c.Adapter.AddSockets(opts, rooms)
}

func (c *clusterAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		_, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: SOCKETS_LEAVE,
			Data: &SocketsJoinLeaveMessage{
				Opts:  EncodeOptions(opts),
				Rooms: rooms,
			},
		})
		if err != nil {
			adapterLog.Debug("[%s] error while publishing message: %s", c.uid, err.Error())
		}
	}
	c.Adapter.DelSockets(opts, rooms)
}

func (c *clusterAdapter) DisconnectSockets(opts *socket.BroadcastOptions, state bool) {
	if opts == nil || opts.Flags == nil || !opts.Flags.Local {
		_, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: DISCONNECT_SOCKETS,
			Data: &DisconnectSocketsMessage{
				Opts:  EncodeOptions(opts),
				Close: state,
			},
		})
		if err != nil {
			adapterLog.Debug("[%s] error while publishing message: %s", c.uid, err.Error())
		}
	}
	c.Adapter.DisconnectSockets(opts, state)
}

func (c *clusterAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(callback func([]socket.SocketDetails, error)) {
		c.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, err error) {
			if err != nil {
				callback(nil, err)
				return
			}
			if opts != nil && opts.Flags != nil && opts.Flags.Local {
				callback(localSockets, nil)
				return
			}
			count, err := c.Proto().ServerCount()
			if err != nil {
				callback(nil, err)
				return
			}
			expectedResponseCount := count - 1

			if expectedResponseCount <= 0 {
				callback(localSockets, nil)
				return
			}

			requestId := RandomId()

			t := DEFAULT_TIMEOUT
			if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil && *opts.Flags.Timeout != 0 && !math.IsNaN(*opts.Flags.Timeout) {
				t = utils.NormalizeTimerMilliseconds(*opts.Flags.Timeout)
			}

			request := &ClusterRequest{
				Type: FETCH_SOCKETS,
				Resolve: func(data *types.Slice[any]) {
					callback(AnySliceToSocketDetails(data.All()), nil)
				},
				Timeout:   new(atomic.Pointer[utils.Timer]),
				Current:   new(atomic.Int64),
				Expected:  expectedResponseCount,
				Responses: types.NewSlice(SocketDetailsToAny(localSockets)...),
			}
			c.requests.Store(requestId, request)

			request.Timeout.Store(utils.SetTimeout(func() {
				if !finishClusterRequest(&c.requests, requestId, request, request.Timeout) {
					return
				}
				callback(nil, fmt.Errorf("timeout reached: only %d responses received out of %d", request.Current.Load(), request.Expected))
			}, t))

			_, publishErr := c.PublishAndReturnOffset(&ClusterMessage{
				Type: FETCH_SOCKETS,
				Data: &FetchSocketsMessage{
					Opts:      EncodeOptions(opts),
					RequestId: requestId,
				},
			})
			if publishErr != nil && finishClusterRequest(&c.requests, requestId, request, request.Timeout) {
				callback(nil, publishErr)
			}
		})
	}
}

func (c *clusterAdapter) ServerSideEmit(packet []any) error {
	packetLen := len(packet)
	if packetLen == 0 {
		return fmt.Errorf("packet cannot be empty")
	}

	ack, withAck := packet[packetLen-1].(socket.Ack)
	if !withAck {
		_, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: SERVER_SIDE_EMIT,
			Data: &ServerSideEmitMessage{
				Packet: packet,
			},
		})
		return err
	}

	count, err := c.Proto().ServerCount()
	if err != nil {
		return err
	}
	expectedResponseCount := count - 1
	if log.DEBUG.Load() {
		adapterLog.Debug(`[%s] waiting for %d responses to "serverSideEmit" request`, c.uid, expectedResponseCount)
	}

	if expectedResponseCount <= 0 {
		ack([]any{}, nil)
		return nil
	}

	requestId := RandomId()

	request := &ClusterRequest{
		Type: SERVER_SIDE_EMIT,
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Timeout:   new(atomic.Pointer[utils.Timer]),
		Current:   new(atomic.Int64),
		Expected:  expectedResponseCount,
		Responses: types.NewSlice[any](),
	}
	c.requests.Store(requestId, request)

	request.Timeout.Store(utils.SetTimeout(func() {
		if !finishClusterRequest(&c.requests, requestId, request, request.Timeout) {
			return
		}
		ack(
			request.Responses.All(),
			fmt.Errorf(`timeout reached: only %d responses received out of %d`, request.Current.Load(), request.Expected),
		)
	}, DEFAULT_TIMEOUT))

	_, err = c.PublishAndReturnOffset(&ClusterMessage{
		Type: SERVER_SIDE_EMIT,
		Data: &ServerSideEmitMessage{
			RequestId: new(requestId), // the presence of this attribute defines whether an acknowledgement is needed
			Packet:    packet[:packetLen-1],
		},
	})
	if err != nil && finishClusterRequest(&c.requests, requestId, request, request.Timeout) {
		return err
	}
	return nil
}

func (c *clusterAdapter) Publish(message *ClusterMessage) {
	if c.publisher.IsShuttingDown() {
		return
	}
	message.Uid = c.uid
	message.Nsp = c.Nsp().Name()
	publish, err := c.Proto().(ClusterAdapter).PreparePublish(message)
	if err != nil {
		adapterLog.Debug(`[%s] error while publishing message: %s`, c.uid, err.Error())
		return
	}
	if err := c.enqueue(c.publisher, func() {
		_, err := publish()
		if err != nil {
			adapterLog.Debug(`[%s] error while publishing message: %s`, c.uid, err.Error())
		}
	}); err != nil {
		adapterLog.Debug(`[%s] error while publishing message: %s`, c.uid, err.Error())
	}
}

func (c *clusterAdapter) PublishAndReturnOffset(message *ClusterMessage) (Offset, error) {
	if c.publisher.IsShuttingDown() {
		return "", ErrAdapterClosed
	}
	message.Uid = c.uid
	message.Nsp = c.Nsp().Name()
	publish, prepareErr := c.Proto().(ClusterAdapter).PreparePublish(message)
	if prepareErr != nil {
		return "", prepareErr
	}
	result := struct {
		offset Offset
		err    error
	}{err: errClusterPublishPanicked}
	done := make(chan struct{})
	if enqueueErr := c.enqueue(c.publisher, func() {
		defer close(done)
		result.offset, result.err = publish()
	}); enqueueErr != nil {
		return "", enqueueErr
	}
	<-done
	return result.offset, result.err
}

func (c *clusterAdapter) enqueue(tasks *queue.Queue, task func()) error {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()
	if tasks.IsShuttingDown() {
		return ErrAdapterClosed
	}
	tasks.Enqueue(task)
	return nil
}

func (c *clusterAdapter) PreparePublish(*ClusterMessage) (PublishFunc, error) {
	return nil, errors.New("PreparePublish() is not supported on parent ClusterAdapter")
}

func (c *clusterAdapter) Close() {
	c.queueMu.Lock()
	if c.publisher.IsShuttingDown() {
		c.queueMu.Unlock()
		return
	}
	c.responses.TryClose()
	c.publisher.TryClose()
	c.queueMu.Unlock()
	c.Adapter.Close()
}

func (c *clusterAdapter) PublishAndClose(message *ClusterMessage) {
	if c.publisher.IsShuttingDown() {
		return
	}
	message.Uid = c.uid
	message.Nsp = c.Nsp().Name()
	// Preparation may invoke user encoders; never run it under queueMu.
	publish, prepareErr := c.Proto().(ClusterAdapter).PreparePublish(message)
	c.queueMu.Lock()
	if c.publisher.IsShuttingDown() {
		c.queueMu.Unlock()
		return
	}

	responsesDone := make(chan struct{})
	c.responses.Enqueue(func() { close(responsesDone) })
	c.publisher.Enqueue(func() {
		defer c.Adapter.Close()
		<-responsesDone
		err := prepareErr
		if err == nil {
			_, err = publish()
		}
		if err != nil {
			adapterLog.Debug(`[%s] error while publishing message: %s`, c.uid, err.Error())
		}
	})
	c.responses.TryClose()
	c.publisher.TryClose()
	c.queueMu.Unlock()
}

func (c *clusterAdapter) PublishResponse(requesterUid ServerId, response *ClusterResponse) {
	if c.responses.IsShuttingDown() {
		return
	}
	response.Uid = c.uid
	response.Nsp = c.Nsp().Name()
	publish, err := c.Proto().(ClusterAdapter).PreparePublishResponse(requesterUid, response)
	if err != nil {
		adapterLog.Debug(`[%s] error while publishing response: %s`, c.uid, err.Error())
		return
	}
	if err = c.enqueue(c.responses, func() {
		if _, publishErr := publish(); publishErr != nil {
			adapterLog.Debug(`[%s] error while publishing response: %s`, c.uid, publishErr.Error())
		}
	}); err != nil {
		adapterLog.Debug(`[%s] error while publishing response: %s`, c.uid, err.Error())
	}
}

func (c *clusterAdapter) PreparePublishResponse(ServerId, *ClusterResponse) (PublishFunc, error) {
	return nil, errors.New("PreparePublishResponse() is not supported on parent ClusterAdapter")
}
