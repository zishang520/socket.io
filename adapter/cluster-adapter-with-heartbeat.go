package adapter

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/engine.io/v2/utils"
	"github.com/zishang520/socket.io/v2/socket"
)

type (
	ClusterAdapterWithHeartbeatBuilder struct {
		Opts *ClusterAdapterOptions
	}

	clusterAdapterWithHeartbeat struct {
		ClusterAdapter

		_opts *ClusterAdapterOptions

		heartbeatTimer atomic.Pointer[utils.Timer]
		nodesMap       *types.Map[ServerId, int64] // uid => timestamp of last message
		cleanupTimer   atomic.Pointer[utils.Timer]
		customRequests *types.Map[string, *CustomClusterRequest]
	}
)

func (c *ClusterAdapterWithHeartbeatBuilder) New(nsp socket.Namespace) Adapter {
	return NewClusterAdapterWithHeartbeat(nsp, c.Opts)
}

func MakeClusterAdapterWithHeartbeat() ClusterAdapterWithHeartbeat {
	c := &clusterAdapterWithHeartbeat{
		ClusterAdapter: MakeClusterAdapter(),

		_opts:          DefaultClusterAdapterOptions(),
		nodesMap:       &types.Map[ServerId, int64]{},
		customRequests: &types.Map[string, *CustomClusterRequest]{},
	}

	c.Prototype(c)

	return c
}

func NewClusterAdapterWithHeartbeat(nsp socket.Namespace, opts *ClusterAdapterOptions) ClusterAdapterWithHeartbeat {
	c := MakeClusterAdapterWithHeartbeat()

	c.SetOpts(opts)

	c.Construct(nsp)

	return c
}

func (a *clusterAdapterWithHeartbeat) SetOpts(opts *ClusterAdapterOptions) {
	if opts == nil {
		opts = DefaultClusterAdapterOptions()
	}
	a._opts.Assign(opts)
}

func (a *clusterAdapterWithHeartbeat) Construct(nsp socket.Namespace) {
	a.ClusterAdapter.Construct(nsp)
	a.cleanupTimer.Store(utils.SetInterval(func() {
		now := time.Now().UnixMilli()
		a.nodesMap.Range(func(uid ServerId, lastSeen int64) bool {
			if now-lastSeen > a._opts.HeartbeatTimeout() {
				adapter_log.Debug("[%s] node %s seems down", a.Uid(), uid)
				a.removeNode(uid)
			}
			return true
		})
	}, 1_000*time.Millisecond))
}

func (a *clusterAdapterWithHeartbeat) Init() {
	a.Publish(&ClusterMessage{
		Type: INITIAL_HEARTBEAT,
	})
}

func (a *clusterAdapterWithHeartbeat) scheduleHeartbeat() {
	if heartbeatTimer := a.heartbeatTimer.Load(); heartbeatTimer != nil {
		heartbeatTimer.Refresh()
	} else {
		a.heartbeatTimer.Store(utils.SetTimeout(func() {
			a.Publish(&ClusterMessage{
				Type: HEARTBEAT,
			})
		}, a._opts.HeartbeatInterval()))
	}
}

func (a *clusterAdapterWithHeartbeat) Close() {
	a.Publish(&ClusterMessage{
		Type: ADAPTER_CLOSE,
	})
	utils.ClearTimeout(a.heartbeatTimer.Load())
	utils.ClearInterval(a.cleanupTimer.Load())
}

func (a *clusterAdapterWithHeartbeat) OnMessage(message *ClusterMessage, offset Offset) {
	if message.Uid == a.Uid() {
		adapter_log.Debug("[%s] ignore message from self", a.Uid())
		return
	}

	if message.Uid != EMITTER_UID {
		// we track the UID of each sender, in order to know how many servers there are in the cluster
		a.nodesMap.Store(message.Uid, time.Now().UnixMilli())
	}

	adapter_log.Debug(
		"[%s] new event of type %d from %s",
		a.Uid(),
		message.Type,
		message.Uid,
	)

	switch message.Type {
	case INITIAL_HEARTBEAT:
		a.Publish(&ClusterMessage{Type: HEARTBEAT})
	case HEARTBEAT:
		// Do nothing
	case ADAPTER_CLOSE:
		a.removeNode(message.Uid)
	default:
		a.ClusterAdapter.OnMessage(message, offset)
	}
}

func (a *clusterAdapterWithHeartbeat) ServerCount() int64 {
	return int64(a.nodesMap.Len() + 1)
}

func (a *clusterAdapterWithHeartbeat) Publish(message *ClusterMessage) {
	a.scheduleHeartbeat()

	a.ClusterAdapter.Publish(message)
}

func (a *clusterAdapterWithHeartbeat) ServerSideEmit(packet []any) error {
	data_len := len(packet)
	ack, withAck := packet[data_len-1].(socket.Ack)
	if !withAck {
		a.Publish(&ClusterMessage{
			Type: SERVER_SIDE_EMIT,
			Data: &ServerSideEmitMessage{
				Packet: packet,
			},
		})
		return nil
	}
	expectedResponseCount := a.nodesMap.Len()

	adapter_log.Debug(
		`[%s] waiting for %d responses to "serverSideEmit" request`,
		a.Uid(),
		expectedResponseCount,
	)

	if expectedResponseCount <= 0 {
		ack(nil, nil)
		return nil
	}

	requestId, err := randomId()

	if err != nil {
		return err
	}

	timeout := utils.SetTimeout(func() {
		if storedRequest, ok := a.customRequests.Load(requestId); ok {
			ack(
				storedRequest.Responses.All(),
				fmt.Errorf(`timeout reached: missing %d responses`, storedRequest.MissingUids.Len()),
			)
			a.customRequests.Delete(requestId)
		}
	}, DEFAULT_TIMEOUT)

	a.customRequests.Store(requestId, &CustomClusterRequest{
		Type: SERVER_SIDE_EMIT,
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Timeout: tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
			t.Store(timeout)
		}),
		MissingUids: types.NewSet(a.nodesMap.Keys()...),
		Responses:   types.NewSlice[any](),
	})

	a.Publish(&ClusterMessage{
		Type: SERVER_SIDE_EMIT,
		Data: &ServerSideEmitMessage{
			RequestId: &requestId, // the presence of this attribute defines whether an acknowledgement is needed
			Packet:    packet[:data_len-1],
		},
	})
	return nil
}

func (a *clusterAdapterWithHeartbeat) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	if opts == nil {
		opts = &socket.BroadcastOptions{
			Rooms:  types.NewSet[socket.Room](),
			Except: types.NewSet[socket.Room](),
		}
	}
	return func(cb func([]socket.SocketDetails, error)) {
		a.ClusterAdapter.FetchSockets(&socket.BroadcastOptions{
			Rooms:  opts.Rooms,
			Except: opts.Except,
			Flags: &socket.BroadcastFlags{
				Local: true,
			},
		})(func(localSockets []socket.SocketDetails, _ error) {
			expectedResponseCount := a.ServerCount() - 1

			if (opts != nil && opts.Flags != nil && opts.Flags.Local) || expectedResponseCount <= 0 {
				cb(localSockets, nil)
				return
			}

			requestId, _ := randomId()

			t := DEFAULT_TIMEOUT
			if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
				t = *opts.Flags.Timeout
			}

			timeout := utils.SetTimeout(func() {
				if storedRequest, ok := a.customRequests.Load(requestId); ok {
					cb(nil, fmt.Errorf("timeout reached: missing %d responses", storedRequest.MissingUids.Len()))
					a.customRequests.Delete(requestId)
				}
			}, t)

			a.customRequests.Store(requestId, &CustomClusterRequest{
				Type: FETCH_SOCKETS,
				Resolve: func(data *types.Slice[any]) {
					cb(sliceMap(data.All(), func(i any) socket.SocketDetails {
						return i.(socket.SocketDetails)
					}), nil)
				},
				Timeout: tap(&atomic.Pointer[utils.Timer]{}, func(t *atomic.Pointer[utils.Timer]) {
					t.Store(timeout)
				}),
				MissingUids: types.NewSet(a.nodesMap.Keys()...),
				Responses: types.NewSlice(sliceMap(localSockets, func(client socket.SocketDetails) any {
					return client
				})...),
			})

			a.Publish(&ClusterMessage{
				Type: FETCH_SOCKETS,
				Data: &FetchSocketsMessage{
					Opts:      encodeOptions(opts),
					RequestId: requestId,
				},
			})
		})
	}
}

func (a *clusterAdapterWithHeartbeat) OnResponse(response *ClusterResponse) {
	switch response.Type {
	case FETCH_SOCKETS_RESPONSE:
		data, ok := response.Data.(*FetchSocketsResponse)
		if !ok {
			adapter_log.Debug("[%s] invalid data for FETCH_SOCKETS_RESPONSE message", a.Uid())
			return
		}
		adapter_log.Debug("[%s] received response %d to request %s", a.Uid(), response.Type, data.RequestId)
		if request, ok := a.customRequests.Load(data.RequestId); ok {
			request.Responses.Push(sliceMap(data.Sockets, func(client *SocketResponse) any {
				return socket.SocketDetails(NewClusterSocket(client))
			})...)

			request.MissingUids.Delete(response.Uid)
			if request.MissingUids.Len() == 0 {
				utils.ClearTimeout(request.Timeout.Load())
				request.Resolve(request.Responses)
				a.customRequests.Delete(data.RequestId)
			}
		}

	case SERVER_SIDE_EMIT_RESPONSE:
		data, ok := response.Data.(*ServerSideEmitResponse)
		if !ok {
			adapter_log.Debug("[%s] invalid data for FETCH_SOCKETS_RESPONSE message", a.Uid())
			return
		}
		adapter_log.Debug("[%s] received response %d to request %s", a.Uid(), response.Type, data.RequestId)
		if request, ok := a.customRequests.Load(data.RequestId); ok {
			request.Responses.Push(data.Packet)

			request.MissingUids.Delete(response.Uid)
			if request.MissingUids.Len() == 0 {
				utils.ClearTimeout(request.Timeout.Load())
				request.Resolve(request.Responses)
				a.customRequests.Delete(data.RequestId)
			}
		}

	default:
		a.ClusterAdapter.OnResponse(response)
	}
}

func (a *clusterAdapterWithHeartbeat) removeNode(uid ServerId) {
	a.customRequests.Range(func(requestId string, request *CustomClusterRequest) bool {
		request.MissingUids.Delete(uid)
		if request.MissingUids.Len() == 0 {
			utils.ClearTimeout(request.Timeout.Load())
			request.Resolve(request.Responses)
			a.customRequests.Delete(requestId)
		}
		return true
	})

	a.nodesMap.Delete(uid)
}
