package adapter

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// ClusterAdapterBuilder is a builder for creating ClusterAdapter instances.
//
// A cluster-ready adapter. Any extending interface must:
//   - implement ClusterAdapter.DoPublish and ClusterAdapter.DoPublishResponse
//   - call ClusterAdapter.OnMessage and ClusterAdapter.OnResponse
type (
	ClusterAdapterBuilder struct {
	}

	// clusterAdapter implements the ClusterAdapter interface for cluster communication.
	clusterAdapter struct {
		Adapter

		// uid is the unique server identifier.
		uid ServerId

		requests    *types.Map[string, *ClusterRequest]
		ackRequests *types.Map[string, *ClusterAckRequest]
	}
)

// New creates a new ClusterAdapter for the given Namespace.
func (cb *ClusterAdapterBuilder) New(nsp socket.Namespace) Adapter {
	return NewClusterAdapter(nsp)
}

// MakeClusterAdapter returns a new default ClusterAdapter instance.
func MakeClusterAdapter() ClusterAdapter {
	c := &clusterAdapter{
		Adapter:     MakeAdapter(),
		requests:    &types.Map[string, *ClusterRequest]{},
		ackRequests: &types.Map[string, *ClusterAckRequest]{},
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

// Uid returns the unique server identifier.
func (c *clusterAdapter) Uid() ServerId {
	return c.uid
}

// Construct initializes the clusterAdapter with the given Namespace.
func (c *clusterAdapter) Construct(nsp socket.Namespace) {
	c.Adapter.Construct(nsp)
	uid, _ := RandomId()
	c.uid = ServerId(uid)
}

// OnMessage handles incoming messages
func (c *clusterAdapter) OnMessage(message *ClusterMessage, offset Offset) {
	if message.Uid == c.uid {
		adapter_log.Debug("[%s] ignore message from self", c.uid)
		return
	}

	adapter_log.Debug("[%s] new event of type %d from %s", c.uid, message.Type, message.Uid)

	switch message.Type {
	case BROADCAST:
		data, ok := message.Data.(*BroadcastMessage)
		if !ok {
			adapter_log.Debug("[%s] invalid data for BROADCAST message", c.uid)
			return
		}

		withAck := data.RequestId != nil
		if withAck {
			c.Adapter.BroadcastWithAck(
				data.Packet,
				DecodeOptions(data.Opts),
				func(clientCount uint64) {
					adapter_log.Debug("[%s] waiting for %d client acknowledgements", c.uid, clientCount)
					c.PublishResponse(message.Uid, &ClusterResponse{
						Type: BROADCAST_CLIENT_COUNT,
						Data: &BroadcastClientCount{
							RequestId:   *data.RequestId,
							ClientCount: clientCount,
						},
					})
				},
				func(args []any, _ error) {
					adapter_log.Debug("[%s] received acknowledgement with value %v", c.uid, args)
					c.PublishResponse(message.Uid, &ClusterResponse{
						Type: BROADCAST_ACK,
						Data: &BroadcastAck{
							RequestId: *data.RequestId,
							Packet:    args,
						},
					})
				},
			)
		} else {
			opts := DecodeOptions(data.Opts)
			c.addOffsetIfNecessary(data.Packet, opts, offset)
			c.Adapter.Broadcast(data.Packet, opts)
		}

	case SOCKETS_JOIN:
		data, ok := message.Data.(*SocketsJoinLeaveMessage)
		if !ok {
			adapter_log.Debug("[%s] invalid data for SOCKETS_JOIN message", c.uid)
			return
		}
		c.Adapter.AddSockets(DecodeOptions(data.Opts), data.Rooms)

	case SOCKETS_LEAVE:
		data, ok := message.Data.(*SocketsJoinLeaveMessage)
		if !ok {
			adapter_log.Debug("[%s] invalid data for SOCKETS_LEAVE message", c.uid)
			return
		}
		c.Adapter.DelSockets(DecodeOptions(data.Opts), data.Rooms)

	case DISCONNECT_SOCKETS:
		data, ok := message.Data.(*DisconnectSocketsMessage)
		if !ok {
			adapter_log.Debug("[%s] invalid data for DISCONNECT_SOCKETS message", c.uid)
			return
		}
		c.Adapter.DisconnectSockets(
			DecodeOptions(data.Opts),
			data.Close,
		)

	case FETCH_SOCKETS:
		data, ok := message.Data.(*FetchSocketsMessage)
		if !ok {
			adapter_log.Debug("[%s] invalid data for FETCH_SOCKETS message", c.uid)
			return
		}
		adapter_log.Debug(
			"[%s] calling fetchSockets with opts %v",
			c.uid,
			data.Opts,
		)
		c.Adapter.FetchSockets(DecodeOptions(data.Opts))(
			func(localSockets []socket.SocketDetails, err error) {
				if err != nil {
					adapter_log.Debug("FETCH_SOCKETS Adapter.OnMessage error: %s", err.Error())
					return
				}
				c.PublishResponse(message.Uid, &ClusterResponse{
					Type: FETCH_SOCKETS_RESPONSE,
					Data: &FetchSocketsResponse{
						RequestId: data.RequestId,
						Sockets: SliceMap(localSockets, func(client socket.SocketDetails) *SocketResponse {
							return &SocketResponse{
								Id:        client.Id(),
								Handshake: client.Handshake(),
								Rooms:     client.Rooms().Keys(),
								Data:      client.Data(),
							}
						}),
					},
				},
				)
			},
		)

	case SERVER_SIDE_EMIT:
		data, ok := message.Data.(*ServerSideEmitMessage)
		if !ok {
			adapter_log.Debug("[%s] invalid data for SERVER_SIDE_EMIT message", c.uid)
			return
		}
		packet := data.Packet
		if data.RequestId == nil {
			c.Nsp().OnServerSideEmit(packet)
			return
		}
		called := sync.Once{}
		callback := socket.Ack(func(arg []any, _ error) {
			// only one argument is expected
			called.Do(func() {
				adapter_log.Debug("[%s] calling acknowledgement with %v", c.uid, arg)
				c.PublishResponse(message.Uid, &ClusterResponse{
					Type: SERVER_SIDE_EMIT_RESPONSE,
					Data: &ServerSideEmitResponse{
						RequestId: *data.RequestId,
						Packet:    arg,
					},
				})
			})
		})

		c.Nsp().OnServerSideEmit(append(packet, callback))

	case BROADCAST_CLIENT_COUNT, BROADCAST_ACK, FETCH_SOCKETS_RESPONSE, SERVER_SIDE_EMIT_RESPONSE:
		// extending classes may not make a distinction between a ClusterMessage and a ClusterResponse payload and may
		// always call the OnMessage() method
		c.OnResponse(message)
	default:
		adapter_log.Debug("[%s] unknown message type: %d", c.uid, message.Type)
	}
}

// OnResponse handles incoming responses
func (c *clusterAdapter) OnResponse(response *ClusterResponse) {
	switch response.Type {
	case BROADCAST_CLIENT_COUNT:
		data, ok := response.Data.(*BroadcastClientCount)
		if !ok {
			adapter_log.Debug("[%s] invalid data for BROADCAST_CLIENT_COUNT message", c.uid)
			return
		}
		adapter_log.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
		if ackRequest, ok := c.ackRequests.Load(data.RequestId); ok {
			ackRequest.ClientCountCallback(data.ClientCount)
		}

	case BROADCAST_ACK:
		data, ok := response.Data.(*BroadcastAck)
		if !ok {
			adapter_log.Debug("[%s] invalid data for BROADCAST_ACK message", c.uid)
			return
		}
		adapter_log.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
		if ackRequest, ok := c.ackRequests.Load(data.RequestId); ok {
			ackRequest.Ack(data.Packet, nil)
		}

	case FETCH_SOCKETS_RESPONSE:
		data, ok := response.Data.(*FetchSocketsResponse)
		if !ok {
			adapter_log.Debug("[%s] invalid data for FETCH_SOCKETS_RESPONSE message", c.uid)
			return
		}
		adapter_log.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
		if request, ok := c.requests.Load(data.RequestId); ok {
			request.Current.Add(1)
			request.Responses.Push(SliceMap(data.Sockets, func(client *SocketResponse) any {
				return socket.SocketDetails(NewRemoteSocket(client))
			})...)

			if request.Current.Load() == request.Expected {
				utils.ClearTimeout(request.Timeout.Load())
				request.Resolve(request.Responses)
				c.requests.Delete(data.RequestId)
			}
		}

	case SERVER_SIDE_EMIT_RESPONSE:
		data, ok := response.Data.(*ServerSideEmitResponse)
		if !ok {
			adapter_log.Debug("[%s] invalid data for SERVER_SIDE_EMIT_RESPONSE message", c.uid)
			return
		}
		adapter_log.Debug("[%s] received response %d to request %s", c.uid, response.Type, data.RequestId)
		if request, ok := c.requests.Load(data.RequestId); ok {
			request.Current.Add(1)
			request.Responses.Push(data.Packet)

			if request.Current.Load() == request.Expected {
				utils.ClearTimeout(request.Timeout.Load())
				request.Resolve(request.Responses)
				c.requests.Delete(data.RequestId)
			}
		}
	default:
		adapter_log.Debug("[%s] unknown response type: %d", c.uid, response.Type)
	}
}

func (c *clusterAdapter) Broadcast(packet *parser.Packet, opts *socket.BroadcastOptions) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		offset, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: BROADCAST,
			Data: &BroadcastMessage{
				Packet: packet,
				Opts:   EncodeOptions(opts),
			},
		})
		if err != nil {
			adapter_log.Debug("[%s] error while broadcasting message: %s", c.uid, err.Error())
			return
		}
		c.addOffsetIfNecessary(packet, opts, offset)
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
	notVolatile := opts == nil || opts.Flags == nil || opts.Flags.Volatile == false

	if isEventPacket && withoutAcknowledgement && notVolatile {
		packet.Data = append(packet.Data.([]any), offset)
	}
}

func (c *clusterAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	if !onlyLocal {
		requestId, _ := RandomId()

		c.ackRequests.Store(requestId, &ClusterAckRequest{
			ClientCountCallback: clientCountCallback,
			Ack:                 ack,
		})

		c.Publish(&ClusterMessage{
			Type: BROADCAST,
			Data: &BroadcastMessage{
				Packet:    packet,
				RequestId: &requestId,
				Opts:      EncodeOptions(opts),
			},
		})

		timeout := time.Duration(0)
		if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
			timeout = *opts.Flags.Timeout
		}

		// we have no way to know at this level whether the server has received an acknowledgement from each client, so we
		// will simply clean up the ackRequests map after the given delay
		utils.SetTimeout(func() {
			c.ackRequests.Delete(requestId)
		}, timeout)
	}

	c.Adapter.BroadcastWithAck(packet, opts, clientCountCallback, ack)
}

func (c *clusterAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		_, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: SOCKETS_JOIN,
			Data: &SocketsJoinLeaveMessage{
				Opts:  EncodeOptions(opts),
				Rooms: rooms,
			},
		})
		if err != nil {
			adapter_log.Debug("[%s] error while publishing message: %s", c.uid, err.Error())
		}
	}

	c.Adapter.AddSockets(opts, rooms)
}

func (c *clusterAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		_, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: SOCKETS_LEAVE,
			Data: &SocketsJoinLeaveMessage{
				Opts:  EncodeOptions(opts),
				Rooms: rooms,
			},
		})
		if err != nil {
			adapter_log.Debug("[%s] error while publishing message: %s", c.uid, err.Error())
		}
	}

	c.Adapter.DelSockets(opts, rooms)
}

func (c *clusterAdapter) DisconnectSockets(opts *socket.BroadcastOptions, state bool) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local

	if !onlyLocal {
		_, err := c.PublishAndReturnOffset(&ClusterMessage{
			Type: DISCONNECT_SOCKETS,
			Data: &DisconnectSocketsMessage{
				Opts:  EncodeOptions(opts),
				Close: state,
			},
		})
		if err != nil {
			adapter_log.Debug("[%s] error while publishing message: %s", c.uid, err.Error())
		}
	}

	c.Adapter.DisconnectSockets(opts, state)
}

func (c *clusterAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(callback func([]socket.SocketDetails, error)) {
		c.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, _ error) {
			expectedResponseCount := c.ServerCount() - 1

			if (opts != nil && opts.Flags != nil && opts.Flags.Local) || expectedResponseCount <= 0 {
				callback(localSockets, nil)
				return
			}

			requestId, _ := RandomId()

			t := DEFAULT_TIMEOUT
			if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
				t = *opts.Flags.Timeout
			}

			timeout := utils.SetTimeout(func() {
				if storedRequest, ok := c.requests.Load(requestId); ok {
					callback(nil, fmt.Errorf("timeout reached: only %d responses received out of %d", storedRequest.Current.Load(), storedRequest.Expected))
					c.requests.Delete(requestId)
				}
			}, t)

			c.requests.Store(requestId, &ClusterRequest{
				Type: FETCH_SOCKETS,
				Resolve: func(data *types.Slice[any]) {
					callback(SliceMap(data.All(), func(i any) socket.SocketDetails {
						return i.(socket.SocketDetails)
					}), nil)
				},
				Timeout: Tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
					t.Store(timeout)
				}),
				Current:  &atomic.Int64{},
				Expected: expectedResponseCount,
				Responses: types.NewSlice(SliceMap(localSockets, func(client socket.SocketDetails) any {
					return client
				})...),
			})

			c.Publish(&ClusterMessage{
				Type: FETCH_SOCKETS,
				Data: &FetchSocketsMessage{
					Opts:      EncodeOptions(opts),
					RequestId: requestId,
				},
			})
		})
	}
}

func (c *clusterAdapter) ServerSideEmit(packet []any) error {
	if len(packet) == 0 {
		return fmt.Errorf("packet cannot be empty")
	}

	data_len := len(packet)
	ack, withAck := packet[data_len-1].(socket.Ack)
	if !withAck {
		c.Publish(&ClusterMessage{
			Type: SERVER_SIDE_EMIT,
			Data: &ServerSideEmitMessage{
				Packet: packet,
			},
		})
		return nil
	}

	expectedResponseCount := c.ServerCount() - 1

	adapter_log.Debug(`[%s] waiting for %d responses to "serverSideEmit" request`, c.uid, expectedResponseCount)

	if expectedResponseCount <= 0 {
		ack(nil, nil)
		return nil
	}

	requestId, err := RandomId()

	if err != nil {
		return err
	}

	timeout := utils.SetTimeout(func() {
		if storedRequest, ok := c.requests.Load(requestId); ok {
			ack(
				storedRequest.Responses.All(),
				fmt.Errorf(`timeout reached: only %d responses received out of %d`, storedRequest.Current.Load(), storedRequest.Expected),
			)
			c.requests.Delete(requestId)
		}
	}, DEFAULT_TIMEOUT)

	c.requests.Store(requestId, &ClusterRequest{
		Type: SERVER_SIDE_EMIT,
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Timeout: Tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
			t.Store(timeout)
		}),
		Current:   &atomic.Int64{},
		Expected:  expectedResponseCount,
		Responses: types.NewSlice[any](),
	})

	c.Publish(&ClusterMessage{
		Type: SERVER_SIDE_EMIT,
		Data: &ServerSideEmitMessage{
			RequestId: &requestId, // the presence of this attribute defines whether an acknowledgement is needed
			Packet:    packet[:data_len-1],
		},
	})
	return nil
}

func (c *clusterAdapter) Publish(message *ClusterMessage) {
	_, err := c.PublishAndReturnOffset(message)
	if err != nil {
		adapter_log.Debug(`[%s] error while publishing message: %s`, c.uid, err.Error())
	}
}

func (c *clusterAdapter) PublishAndReturnOffset(message *ClusterMessage) (Offset, error) {
	message.Uid = c.uid
	message.Nsp = c.Nsp().Name()
	return c.Proto().(ClusterAdapter).DoPublish(message)
}

// Send a message to the other members of the cluster.
func (c *clusterAdapter) DoPublish(message *ClusterMessage) (Offset, error) {
	return "", errors.New("DoPublish() is not supported on parent ClusterAdapter")
}

func (c *clusterAdapter) PublishResponse(requesterUid ServerId, response *ClusterResponse) {
	response.Uid = c.uid
	response.Nsp = c.Nsp().Name()

	err := c.Proto().(ClusterAdapter).DoPublishResponse(requesterUid, response)
	if err != nil {
		adapter_log.Debug(`[%s] error while publishing response: %s`, c.uid, err.Error())
	}
}

// Send a response to the given member of the cluster.
func (c *clusterAdapter) DoPublishResponse(requesterUid ServerId, response *ClusterResponse) error {
	return errors.New("DoPublishResponse() is not supported on parent ClusterAdapter")
}
