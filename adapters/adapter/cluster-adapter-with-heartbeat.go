package adapter

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// ClusterAdapterWithHeartbeatBuilder is a builder for creating ClusterAdapterWithHeartbeat instances.
type (
	ClusterAdapterWithHeartbeatBuilder struct {
		Opts ClusterAdapterOptionsInterface
	}

	// clusterAdapterWithHeartbeat implements the ClusterAdapterWithHeartbeat interface.
	clusterAdapterWithHeartbeat struct {
		ClusterAdapter

		_opts ClusterAdapterOptions

		heartbeatTimer atomic.Pointer[utils.Timer]
		cleanupTimer   atomic.Pointer[utils.Timer]
		nodesMap       types.Map[ServerId, int64] // uid => timestamp of last message
		customRequests types.Map[string, *CustomClusterRequest]
	}
)

func (c *ClusterAdapterWithHeartbeatBuilder) New(nsp socket.Namespace) Adapter {
	return NewClusterAdapterWithHeartbeat(nsp, c.Opts)
}

func MakeClusterAdapterWithHeartbeat() ClusterAdapterWithHeartbeat {
	c := &clusterAdapterWithHeartbeat{
		ClusterAdapter: MakeClusterAdapter(),
	}

	c.Prototype(c)

	return c
}

func NewClusterAdapterWithHeartbeat(nsp socket.Namespace, opts any) ClusterAdapterWithHeartbeat {
	c := MakeClusterAdapterWithHeartbeat()

	c.SetOpts(opts)

	c.Construct(nsp)

	return c
}

func (a *clusterAdapterWithHeartbeat) SetOpts(opts any) {
	if options, ok := opts.(ClusterAdapterOptionsInterface); ok {
		a._opts.Assign(options)
	}
}

func (a *clusterAdapterWithHeartbeat) Construct(nsp socket.Namespace) {
	a.ClusterAdapter.Construct(nsp)

	if a._opts.GetRawHeartbeatInterval() == nil {
		a._opts.SetHeartbeatInterval(5_000 * time.Millisecond)
	}

	if a._opts.GetRawHeartbeatTimeout() == nil {
		a._opts.SetHeartbeatTimeout(10_000)
	}

	a.cleanupTimer.Store(utils.SetInterval(func() {
		now := time.Now().UnixMilli()
		heartbeatTimeout := a._opts.HeartbeatTimeout()
		a.nodesMap.Range(func(uid ServerId, lastSeen int64) bool {
			if now-lastSeen > heartbeatTimeout {
				adapterLog.Debug("[%s] node %s seems down", a.Uid(), uid)
				a.removeNode(uid, lastSeen)
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
	heartbeatTimer := a.heartbeatTimer.Load()
	if heartbeatTimer != nil {
		heartbeatTimer.Refresh()
		return
	}

	heartbeatTimer = utils.SetTimeout(func() {
		a.Publish(&ClusterMessage{Type: HEARTBEAT})
	}, a._opts.HeartbeatInterval())
	if !a.heartbeatTimer.CompareAndSwap(nil, heartbeatTimer) {
		heartbeatTimer.Stop()
		a.heartbeatTimer.Load().Refresh()
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
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] ignore message from self", a.Uid())
		}
		return
	}

	if message.Uid != "" && message.Uid != EMITTER_UID {
		// we track the UID of each sender, in order to know how many servers there are in the cluster
		a.nodesMap.Store(message.Uid, time.Now().UnixMilli())
	}

	if log.DEBUG.Load() {
		adapterLog.Debug(
			"[%s] new event of type %d from %s",
			a.Uid(),
			message.Type,
			message.Uid,
		)
	}

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

func (a *clusterAdapterWithHeartbeat) ServerCount() (int64, error) {
	return int64(a.nodesMap.Len() + 1), nil
}

func (a *clusterAdapterWithHeartbeat) Publish(message *ClusterMessage) {
	a.scheduleHeartbeat()
	a.ClusterAdapter.Publish(message)
}

func (a *clusterAdapterWithHeartbeat) ServerSideEmit(packet []any) error {
	if len(packet) == 0 {
		return fmt.Errorf("packet cannot be empty")
	}

	packetLen := len(packet)
	ack, withAck := packet[packetLen-1].(socket.Ack)
	if !withAck {
		a.Publish(&ClusterMessage{
			Type: SERVER_SIDE_EMIT,
			Data: &ServerSideEmitMessage{
				Packet: packet,
			},
		})
		return nil
	}
	missingUids := types.NewSet(a.nodesMap.Keys()...)
	expectedResponseCount := missingUids.Len()
	if log.DEBUG.Load() {
		adapterLog.Debug(
			`[%s] waiting for %d responses to "serverSideEmit" request`,
			a.Uid(),
			expectedResponseCount,
		)
	}
	if expectedResponseCount == 0 {
		ack([]any{}, nil)
		return nil
	}

	requestId := RandomId()

	request := &CustomClusterRequest{
		Type: SERVER_SIDE_EMIT,
		Resolve: func(data *types.Slice[any]) {
			ack(data.All(), nil)
		},
		Timeout:     new(atomic.Pointer[utils.Timer]),
		MissingUids: missingUids,
		Responses:   types.NewSlice[any](),
	}
	a.customRequests.Store(requestId, request)

	request.Timeout.Store(utils.SetTimeout(func() {
		if storedRequest, ok := a.customRequests.Load(requestId); ok {
			storedRequest.Once.Do(func() {
				ack(
					storedRequest.Responses.All(),
					fmt.Errorf(`timeout reached: missing %d responses`, storedRequest.MissingUids.Len()),
				)
				a.customRequests.Delete(requestId)
			})
		}
	}, DEFAULT_TIMEOUT))

	a.Publish(&ClusterMessage{
		Type: SERVER_SIDE_EMIT,
		Data: &ServerSideEmitMessage{
			RequestId: new(requestId), // the presence of this attribute defines whether an acknowledgement is needed
			Packet:    packet[:packetLen-1],
		},
	})
	return nil
}

func (a *clusterAdapterWithHeartbeat) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	if opts == nil {
		opts = &socket.BroadcastOptions{}
	}
	return func(cb func([]socket.SocketDetails, error)) {
		a.ClusterAdapter.FetchSockets(&socket.BroadcastOptions{
			Rooms:  opts.Rooms,
			Except: opts.Except,
			Flags: &socket.BroadcastFlags{
				Local: true,
			},
		})(func(localSockets []socket.SocketDetails, err error) {
			if err != nil {
				cb(nil, err)
				return
			}
			if opts.Flags != nil && opts.Flags.Local {
				cb(localSockets, nil)
				return
			}

			missingUids := a.nodesMap.Keys()
			if len(missingUids) == 0 {
				cb(localSockets, nil)
				return
			}

			requestId := RandomId()

			t := DEFAULT_TIMEOUT
			if opts.Flags != nil && opts.Flags.Timeout != nil && *opts.Flags.Timeout != 0 {
				t = utils.NormalizeTimerMilliseconds(*opts.Flags.Timeout)
			}

			request := &CustomClusterRequest{
				Type: FETCH_SOCKETS,
				Resolve: func(data *types.Slice[any]) {
					cb(AnySliceToSocketDetails(data.All()), nil)
				},
				Timeout:     new(atomic.Pointer[utils.Timer]),
				MissingUids: types.NewSet(missingUids...),
				Responses:   types.NewSlice(SocketDetailsToAny(localSockets)...),
			}
			a.customRequests.Store(requestId, request)

			request.Timeout.Store(utils.SetTimeout(func() {
				if storedRequest, ok := a.customRequests.Load(requestId); ok {
					storedRequest.Once.Do(func() {
						cb(nil, fmt.Errorf("timeout reached: missing %d responses", storedRequest.MissingUids.Len()))
						a.customRequests.Delete(requestId)
					})
				}
			}, t))

			a.Publish(&ClusterMessage{
				Type: FETCH_SOCKETS,
				Data: &FetchSocketsMessage{
					Opts:      EncodeOptions(opts),
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
			adapterLog.Debug("[%s] invalid data for FETCH_SOCKETS_RESPONSE message", a.Uid())
			return
		}
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] received response %d to request %s", a.Uid(), response.Type, data.RequestId)
		}
		if request, ok := a.customRequests.Load(data.RequestId); ok {
			request.Responses.Push(SocketResponsesToDetailsAny(data.Sockets)...)

			request.MissingUids.Delete(response.Uid)
			if request.MissingUids.Len() == 0 {
				request.Once.Do(func() {
					utils.ClearTimeout(request.Timeout.Load())
					request.Resolve(request.Responses)
					a.customRequests.Delete(data.RequestId)
				})
			}
		}

	case SERVER_SIDE_EMIT_RESPONSE:
		data, ok := response.Data.(*ServerSideEmitResponse)
		if !ok {
			adapterLog.Debug("[%s] invalid data for SERVER_SIDE_EMIT_RESPONSE message", a.Uid())
			return
		}
		if log.DEBUG.Load() {
			adapterLog.Debug("[%s] received response %d to request %s", a.Uid(), response.Type, data.RequestId)
		}
		if request, ok := a.customRequests.Load(data.RequestId); ok {
			request.Responses.Push(data.Packet)

			request.MissingUids.Delete(response.Uid)
			if request.MissingUids.Len() == 0 {
				request.Once.Do(func() {
					utils.ClearTimeout(request.Timeout.Load())
					request.Resolve(request.Responses)
					a.customRequests.Delete(data.RequestId)
				})
			}
		}

	default:
		a.ClusterAdapter.OnResponse(response)
	}
}

func (a *clusterAdapterWithHeartbeat) removeNode(uid ServerId, expectedLastSeen ...int64) {
	if len(expectedLastSeen) > 0 {
		if !a.nodesMap.CompareAndDelete(uid, expectedLastSeen[0]) {
			return
		}
	} else {
		a.nodesMap.Delete(uid)
	}

	a.customRequests.Range(func(requestId string, request *CustomClusterRequest) bool {
		request.MissingUids.Delete(uid)
		if request.MissingUids.Len() == 0 {
			request.Once.Do(func() {
				utils.ClearTimeout(request.Timeout.Load())
				request.Resolve(request.Responses)
				a.customRequests.Delete(requestId)
			})
		}
		return true
	})
}
