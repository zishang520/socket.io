// Package adapter provides a MongoDB-based adapter implementation for Socket.IO clustering.
// It uses MongoDB Change Streams for pub/sub communication between nodes.
// The document format is compatible with the Node.js @socket.io/mongo-adapter package,
// allowing mixed Go and Node.js deployments.
package adapter

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	sliceUtils "github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	mongoLog = log.NewLog("socket.io-mongo-adapter")

	errAdapterClosed           = errors.New("adapter is closed")
	errInvalidOffset           = errors.New("invalid offset")
	errFetchSession            = errors.New("error while fetching session")
	errSessionOrOffsetNotFound = errors.New("session or offset not found")
	errFetchMissedPackets      = errors.New("error while fetching missed packets")
)

func onPublishError(err error) {
	if err != nil {
		mongoLog.Debug("something went wrong when inserting the MongoDB document: %s", err.Error())
	}
}

// mongoAdapter follows the standalone protocol implemented by
// @socket.io/mongo-adapter. Event type 13 represents a session on MongoDB,
// so the generic heartbeat implementation cannot be embedded here.
type mongoAdapter struct {
	adapter.Adapter

	uid               adapter.ServerId
	requestsTimeout   time.Duration
	heartbeatInterval time.Duration
	heartbeatTimeout  int64
	addCreatedAtField bool

	mongoCollection *mongo.MongoClient
	nodesMap        types.Map[adapter.ServerId, int64]
	heartbeatTimer  atomic.Pointer[utils.Timer]
	requests        types.Map[string, *mongo.Request]
	ackRequests     types.Map[string, *mongo.AckRequest]
	isClosed        atomic.Bool

	cleanupFunc atomic.Pointer[types.Callable]
}

// MakeMongoAdapter creates a new uninitialized MongoDB adapter.
func MakeMongoAdapter() MongoAdapter {
	a := &mongoAdapter{
		Adapter: adapter.MakeAdapter(),
	}
	a.Prototype(a)
	return a
}

// NewMongoAdapter creates and initializes a MongoDB adapter.
func NewMongoAdapter(nsp socket.Namespace, mongoCollection *mongo.MongoClient, opts any) MongoAdapter {
	a := MakeMongoAdapter()
	a.SetMongo(mongoCollection)
	a.SetOpts(opts)
	a.Construct(nsp)
	return a
}

func (a *mongoAdapter) SetMongo(mongoCollection *mongo.MongoClient) {
	a.mongoCollection = mongoCollection
}

func (a *mongoAdapter) SetOpts(opts any) {
	if options, ok := opts.(adapter.ClusterAdapterOptionsInterface); ok {
		if options.GetRawHeartbeatInterval() != nil {
			a.heartbeatInterval = options.HeartbeatInterval()
		}
		if options.GetRawHeartbeatTimeout() != nil {
			a.heartbeatTimeout = options.HeartbeatTimeout()
		}
	}
	if options, ok := opts.(MongoAdapterOptionsInterface); ok {
		if options.GetRawUid() != nil {
			a.uid = options.Uid()
		}
		if options.GetRawRequestsTimeout() != nil {
			a.requestsTimeout = options.RequestsTimeout()
		}
		if options.GetRawAddCreatedAtField() != nil {
			a.addCreatedAtField = options.AddCreatedAtField()
		}
	}
}

func (a *mongoAdapter) Construct(nsp socket.Namespace) {
	a.Adapter.Construct(nsp)

	if a.heartbeatInterval == 0 {
		a.heartbeatInterval = DefaultHeartbeatInterval
	}
	if a.heartbeatTimeout == 0 {
		a.heartbeatTimeout = DefaultHeartbeatTimeout
	}
	if a.requestsTimeout == 0 {
		a.requestsTimeout = DefaultRequestsTimeout
	}
	if a.uid == "" {
		a.uid = adapter.ServerId(adapter.RandomId())
	}
}

func (a *mongoAdapter) Init() {
	a.Publish(&ClusterMessage{Type: mongo.INITIAL_HEARTBEAT})
}

func (a *mongoAdapter) scheduleHeartbeat() {
	if a.isClosed.Load() {
		return
	}
	if heartbeatTimer := a.heartbeatTimer.Load(); heartbeatTimer != nil {
		heartbeatTimer.Refresh()
		return
	}

	heartbeatTimer := utils.SetTimeout(func() {
		mongoLog.Debug("sending heartbeat")
		a.Publish(&ClusterMessage{Type: mongo.HEARTBEAT})
		a.scheduleHeartbeat()
	}, a.heartbeatInterval)
	if !a.heartbeatTimer.CompareAndSwap(nil, heartbeatTimer) {
		heartbeatTimer.Stop()
		if heartbeatTimer = a.heartbeatTimer.Load(); heartbeatTimer != nil {
			heartbeatTimer.Refresh()
		}
		return
	}
	if a.isClosed.Load() && a.heartbeatTimer.CompareAndSwap(heartbeatTimer, nil) {
		heartbeatTimer.Stop()
	}
}

func (a *mongoAdapter) Uid() adapter.ServerId {
	return a.uid
}

func (a *mongoAdapter) Publish(message *ClusterMessage) {
	document, err := a.prepareDocument(message)
	if err != nil {
		onPublishError(err)
		return
	}
	go func() {
		_, err := a.insertDocument(document)
		onPublishError(err)
	}()
}

func (a *mongoAdapter) PublishAndReturnOffset(message *ClusterMessage) (adapter.Offset, error) {
	return a.publish(message)
}

func (a *mongoAdapter) DoPublish(message *ClusterMessage) (adapter.Offset, error) {
	return a.publish(message)
}

func (a *mongoAdapter) publish(document *ClusterMessage) (adapter.Offset, error) {
	event, err := a.prepareDocument(document)
	if err != nil {
		return "", err
	}
	return a.insertDocument(event)
}

func (a *mongoAdapter) prepareDocument(document *ClusterMessage) (*mongo.AdapterEvent, error) {
	if a.isClosed.Load() {
		return nil, errAdapterClosed
	}
	document.Uid = a.uid
	document.Nsp = a.Nsp().Name()

	event := &mongo.AdapterEvent{
		Uid:  document.Uid,
		Nsp:  document.Nsp,
		Type: document.Type,
	}
	if a.addCreatedAtField {
		event.CreatedAt = bson.DateTime(time.Now().UnixMilli())
	}
	a.scheduleHeartbeat()

	if document.Data != nil {
		data, err := mongo.MarshalAdapterData(document.Data)
		if err != nil {
			return nil, err
		}
		event.Data = data
	}
	return event, nil
}

func (a *mongoAdapter) insertDocument(document *mongo.AdapterEvent) (adapter.Offset, error) {
	result, err := a.mongoCollection.Collection.InsertOne(a.mongoCollection.Context, document)
	if err != nil {
		return "", err
	}
	if id, ok := result.InsertedID.(bson.ObjectID); ok {
		return adapter.Offset(id.Hex()), nil
	}
	return "", nil
}

func (a *mongoAdapter) PublishResponse(_ adapter.ServerId, response *ClusterResponse) {
	a.Publish(response)
}

// MongoDB responses share the same collection, so requesterUid is carried by
// the request ID rather than a transport-specific channel.
func (a *mongoAdapter) DoPublishResponse(_ adapter.ServerId, response *ClusterResponse) error {
	_, err := a.publish(response)
	return err
}

// OnEvent decodes a Change Stream document and routes it through OnMessage.
func (a *mongoAdapter) OnEvent(document *mongo.AdapterEvent) {
	if a.isClosed.Load() {
		return
	}
	if document.Uid != "" && document.Uid != mongo.EMITTER_UID {
		a.nodesMap.Store(document.Uid, time.Now().UnixMilli())
	}
	message := &ClusterMessage{
		Uid:  document.Uid,
		Nsp:  document.Nsp,
		Type: document.Type,
	}
	if document.Data.Type != 0 {
		data, err := mongo.UnmarshalAdapterData(document.Type, document.Data)
		if err != nil {
			mongoLog.Debug("failed to decode data for type %d: %s", document.Type, err.Error())
			return
		}
		message.Data = data
	}
	a.OnMessage(message, adapter.Offset(document.ID.Hex()))
}

func (a *mongoAdapter) OnMessage(message *ClusterMessage, offset adapter.Offset) {
	if a.isClosed.Load() || message.Uid == a.uid || message.Nsp != a.Nsp().Name() {
		return
	}

	switch message.Type {
	case mongo.INITIAL_HEARTBEAT:
		a.Publish(&ClusterMessage{Type: mongo.HEARTBEAT})
	case mongo.HEARTBEAT, mongo.SESSION:
		return
	case mongo.BROADCAST:
		data, ok := message.Data.(*BroadcastMessage)
		if !ok {
			return
		}
		opts := adapter.DecodeOptions(data.Opts)
		if data.RequestId == nil {
			a.addOffsetIfNecessary(data.Packet, opts, offset)
			a.Adapter.Broadcast(data.Packet, opts)
			return
		}

		a.Adapter.BroadcastWithAck(
			data.Packet,
			opts,
			func(clientCount uint64) {
				a.PublishResponse(message.Uid, &ClusterResponse{
					Type: mongo.BROADCAST_CLIENT_COUNT,
					Data: &BroadcastClientCount{
						RequestId:   *data.RequestId,
						ClientCount: clientCount,
					},
				})
			},
			func(packet []any, _ error) {
				a.PublishResponse(message.Uid, &ClusterResponse{
					Type: mongo.BROADCAST_ACK,
					Data: &BroadcastAck{
						RequestId: *data.RequestId,
						Packet:    sliceUtils.TryGet(packet, 0),
					},
				})
			},
		)
	case mongo.SOCKETS_JOIN:
		if data, ok := message.Data.(*SocketsJoinLeaveMessage); ok {
			a.Adapter.AddSockets(adapter.DecodeOptions(data.Opts), data.Rooms)
		}
	case mongo.SOCKETS_LEAVE:
		if data, ok := message.Data.(*SocketsJoinLeaveMessage); ok {
			a.Adapter.DelSockets(adapter.DecodeOptions(data.Opts), data.Rooms)
		}
	case mongo.DISCONNECT_SOCKETS:
		if data, ok := message.Data.(*DisconnectSocketsMessage); ok {
			a.Adapter.DisconnectSockets(adapter.DecodeOptions(data.Opts), data.Close)
		}
	case mongo.FETCH_SOCKETS:
		data, ok := message.Data.(*FetchSocketsMessage)
		if !ok {
			return
		}
		a.Adapter.FetchSockets(adapter.DecodeOptions(data.Opts))(func(localSockets []socket.SocketDetails, err error) {
			if err != nil {
				return
			}
			a.PublishResponse(message.Uid, &ClusterResponse{
				Type: mongo.FETCH_SOCKETS_RESPONSE,
				Data: &FetchSocketsResponse{
					RequestId: data.RequestId,
					Sockets:   socketDetailsToResponses(localSockets),
				},
			})
		})
	case mongo.SERVER_SIDE_EMIT:
		data, ok := message.Data.(*ServerSideEmitMessage)
		if !ok {
			return
		}
		if data.RequestId == nil {
			a.Nsp().OnServerSideEmit(data.Packet)
			return
		}

		var called atomic.Bool
		callback := func(packet []any, _ error) {
			if !called.CompareAndSwap(false, true) {
				return
			}
			a.PublishResponse(message.Uid, &ClusterResponse{
				Type: mongo.SERVER_SIDE_EMIT_RESPONSE,
				Data: &ServerSideEmitResponse{
					RequestId: *data.RequestId,
					Packet:    sliceUtils.TryGet(packet, 0),
				},
			})
		}
		a.Nsp().OnServerSideEmit(append(data.Packet, callback))
	case mongo.BROADCAST_CLIENT_COUNT, mongo.BROADCAST_ACK,
		mongo.FETCH_SOCKETS_RESPONSE, mongo.SERVER_SIDE_EMIT_RESPONSE:
		a.OnResponse(message)
	}
}

func (a *mongoAdapter) OnResponse(response *ClusterResponse) {
	switch response.Type {
	case mongo.BROADCAST_CLIENT_COUNT:
		data, ok := response.Data.(*BroadcastClientCount)
		if !ok {
			return
		}
		if request, found := a.ackRequests.Load(data.RequestId); found {
			request.ClientCountCallback(data.ClientCount)
		}
	case mongo.BROADCAST_ACK:
		data, ok := response.Data.(*BroadcastAck)
		if !ok {
			return
		}
		if request, found := a.ackRequests.Load(data.RequestId); found {
			request.Ack([]any{data.Packet}, nil)
		}
	case mongo.FETCH_SOCKETS_RESPONSE:
		data, ok := response.Data.(*FetchSocketsResponse)
		if !ok {
			return
		}
		request, found := a.requests.Load(data.RequestId)
		if !found {
			return
		}
		request.Lock()
		stored, found := a.requests.Load(data.RequestId)
		if !found || stored != request {
			request.Unlock()
			return
		}
		request.Current++
		request.Responses = slices.Grow(request.Responses, len(data.Sockets))
		for i := range data.Sockets {
			request.Responses = append(request.Responses, adapter.NewRemoteSocket(&data.Sockets[i]))
		}
		if request.Current != request.Expected {
			request.Unlock()
			return
		}
		if !a.requests.CompareAndDelete(data.RequestId, request) {
			request.Unlock()
			return
		}
		timer := request.Timeout
		responses := request.Responses
		resolve := request.Resolve
		request.Unlock()

		utils.ClearTimeout(timer)
		resolve(responses)
	case mongo.SERVER_SIDE_EMIT_RESPONSE:
		data, ok := response.Data.(*ServerSideEmitResponse)
		if !ok {
			return
		}
		request, found := a.requests.Load(data.RequestId)
		if !found {
			return
		}
		request.Lock()
		stored, found := a.requests.Load(data.RequestId)
		if !found || stored != request {
			request.Unlock()
			return
		}
		request.Current++
		request.Responses = append(request.Responses, data.Packet)
		if request.Current != request.Expected {
			request.Unlock()
			return
		}
		if !a.requests.CompareAndDelete(data.RequestId, request) {
			request.Unlock()
			return
		}
		timer := request.Timeout
		responses := request.Responses
		resolve := request.Resolve
		request.Unlock()

		utils.ClearTimeout(timer)
		resolve(responses)
	}
}

func (a *mongoAdapter) ServerCount() int64 {
	now := time.Now().UnixMilli()
	a.nodesMap.Range(func(uid adapter.ServerId, lastSeen int64) bool {
		if now-lastSeen > a.heartbeatTimeout {
			a.nodesMap.CompareAndDelete(uid, lastSeen)
		}
		return true
	})
	return int64(a.nodesMap.Len() + 1)
}

func (a *mongoAdapter) Broadcast(packet *parser.Packet, opts *socket.BroadcastOptions) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	if !onlyLocal {
		offset, err := a.publish(&ClusterMessage{
			Type: mongo.BROADCAST,
			Data: &BroadcastMessage{
				Packet: packet,
				Opts:   adapter.EncodeOptions(opts),
			},
		})
		if err != nil {
			mongoLog.Debug("[%s] error while inserting document: %s", a.uid, err.Error())
			return
		}
		a.addOffsetIfNecessary(packet, opts, offset)
	}
	a.Adapter.Broadcast(packet, opts)
}

func (a *mongoAdapter) BroadcastWithAck(
	packet *parser.Packet,
	opts *socket.BroadcastOptions,
	clientCountCallback func(uint64),
	ack socket.Ack,
) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	if !onlyLocal {
		requestId := adapter.RandomId()
		a.ackRequests.Store(requestId, &mongo.AckRequest{
			Type:                mongo.BROADCAST,
			ClientCountCallback: clientCountCallback,
			Ack:                 ack,
		})
		a.Publish(&ClusterMessage{
			Type: mongo.BROADCAST,
			Data: &BroadcastMessage{
				Packet:    packet,
				Opts:      adapter.EncodeOptions(opts),
				RequestId: new(requestId),
			},
		})

		var timeout time.Duration
		if opts != nil && opts.Flags != nil && opts.Flags.Timeout != nil {
			timeout = utils.FromMilliseconds(*opts.Flags.Timeout)
		}
		utils.SetTimeout(func() {
			a.ackRequests.Delete(requestId)
		}, timeout)
	}
	a.Adapter.BroadcastWithAck(packet, opts, clientCountCallback, ack)
}

func (a *mongoAdapter) AddSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	a.Adapter.AddSockets(opts, rooms)
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	if onlyLocal {
		return
	}
	a.Publish(&ClusterMessage{
		Type: mongo.SOCKETS_JOIN,
		Data: &SocketsJoinLeaveMessage{
			Opts:  adapter.EncodeOptions(opts),
			Rooms: rooms,
		},
	})
}

func (a *mongoAdapter) DelSockets(opts *socket.BroadcastOptions, rooms []socket.Room) {
	a.Adapter.DelSockets(opts, rooms)
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	if onlyLocal {
		return
	}
	a.Publish(&ClusterMessage{
		Type: mongo.SOCKETS_LEAVE,
		Data: &SocketsJoinLeaveMessage{
			Opts:  adapter.EncodeOptions(opts),
			Rooms: rooms,
		},
	})
}

func (a *mongoAdapter) DisconnectSockets(opts *socket.BroadcastOptions, close bool) {
	a.Adapter.DisconnectSockets(opts, close)
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	if onlyLocal {
		return
	}
	a.Publish(&ClusterMessage{
		Type: mongo.DISCONNECT_SOCKETS,
		Data: &DisconnectSocketsMessage{
			Opts:  adapter.EncodeOptions(opts),
			Close: close,
		},
	})
}

func (a *mongoAdapter) FetchSockets(opts *socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	onlyLocal := opts != nil && opts.Flags != nil && opts.Flags.Local
	return func(callback func([]socket.SocketDetails, error)) {
		a.Adapter.FetchSockets(opts)(func(localSockets []socket.SocketDetails, err error) {
			if err != nil {
				callback(nil, err)
				return
			}
			expected := a.ServerCount() - 1
			if onlyLocal || expected == 0 {
				callback(localSockets, nil)
				return
			}

			requestId := adapter.RandomId()
			responses := make([]any, len(localSockets))
			for i, details := range localSockets {
				responses[i] = details
			}
			request := &mongo.Request{
				Type:      mongo.FETCH_SOCKETS,
				Expected:  expected,
				Responses: responses,
				Resolve: func(responses []any) {
					sockets := make([]socket.SocketDetails, len(responses))
					for i, response := range responses {
						sockets[i], _ = response.(socket.SocketDetails)
					}
					callback(sockets, nil)
				},
			}
			request.Lock()
			a.requests.Store(requestId, request)
			request.Timeout = utils.SetTimeout(func() {
				request.Lock()
				if !a.requests.CompareAndDelete(requestId, request) {
					request.Unlock()
					return
				}
				current, expected := request.Current, request.Expected
				request.Unlock()

				callback(nil, fmt.Errorf(
					"timeout reached: only %d responses received out of %d",
					current, expected,
				))
			}, a.requestsTimeout)

			a.Publish(&ClusterMessage{
				Type: mongo.FETCH_SOCKETS,
				Data: &FetchSocketsMessage{
					Opts:      adapter.EncodeOptions(opts),
					RequestId: requestId,
				},
			})
			request.Unlock()
		})
	}
}

func (a *mongoAdapter) ServerSideEmit(packet []any) error {
	if len(packet) != 0 {
		if ack, withAck := packet[len(packet)-1].(socket.Ack); withAck {
			return a.serverSideEmitWithAck(packet[:len(packet)-1], ack)
		}
	}

	a.Publish(&ClusterMessage{
		Type: mongo.SERVER_SIDE_EMIT,
		Data: &ServerSideEmitMessage{
			Packet: packet,
		},
	})
	return nil
}

func (a *mongoAdapter) serverSideEmitWithAck(packet []any, ack socket.Ack) error {
	expected := a.ServerCount() - 1
	if expected <= 0 {
		ack([]any{}, nil)
		return nil
	}

	requestId := adapter.RandomId()
	request := &mongo.Request{
		Type:      mongo.FETCH_SOCKETS,
		Expected:  expected,
		Responses: make([]any, 0, expected),
		Resolve: func(responses []any) {
			ack(responses, nil)
		},
	}
	request.Lock()
	a.requests.Store(requestId, request)
	request.Timeout = utils.SetTimeout(func() {
		request.Lock()
		if !a.requests.CompareAndDelete(requestId, request) {
			request.Unlock()
			return
		}
		responses := request.Responses
		current, expected := request.Current, request.Expected
		request.Unlock()

		ack(
			responses,
			fmt.Errorf(
				"timeout reached: only %d responses received out of %d",
				current, expected,
			),
		)
	}, a.requestsTimeout)

	a.Publish(&ClusterMessage{
		Type: mongo.SERVER_SIDE_EMIT,
		Data: &ServerSideEmitMessage{
			RequestId: new(requestId),
			Packet:    packet,
		},
	})
	request.Unlock()
	return nil
}

func (a *mongoAdapter) PersistSession(session *socket.SessionToPersist) {
	if session == nil {
		return
	}
	var rooms []socket.Room
	if session.Rooms != nil {
		rooms = session.Rooms.Keys()
	}
	a.Publish(&ClusterMessage{
		Type: mongo.SESSION,
		Data: &mongo.SessionDocument{
			Sid:   session.Sid,
			Pid:   session.Pid,
			Rooms: rooms,
			Data:  session.Data,
		},
	})
}

func (a *mongoAdapter) RestoreSession(pid socket.PrivateSessionId, offset string) (*socket.Session, error) {
	eventOffset, err := bson.ObjectIDFromHex(offset)
	if err != nil {
		return nil, errInvalidOffset
	}

	var (
		session    *mongo.SessionDocument
		sessionErr error
		offsetErr  error
		waitGroup  sync.WaitGroup
	)
	waitGroup.Go(func() {
		session, sessionErr = a.findSession(pid)
	})
	waitGroup.Go(func() {
		offsetErr = a.mongoCollection.Collection.FindOne(a.mongoCollection.Context, bson.D{
			{Key: "type", Value: mongo.BROADCAST},
			{Key: "_id", Value: eventOffset},
		}, options.FindOne().SetProjection(bson.D{
			{Key: "_id", Value: 1},
		})).Err()
	})
	waitGroup.Wait()

	if sessionErr != nil || (offsetErr != nil && !errors.Is(offsetErr, mongod.ErrNoDocuments)) {
		return nil, errFetchSession
	}
	if session == nil || errors.Is(offsetErr, mongod.ErrNoDocuments) {
		return nil, errSessionOrOffsetNotFound
	}

	filter := bson.D{{Key: "$and", Value: bson.A{
		bson.D{{Key: "type", Value: mongo.BROADCAST}},
		bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: eventOffset}}}},
		bson.D{{Key: "nsp", Value: a.Nsp().Name()}},
		bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "data.opts.rooms", Value: bson.D{{Key: "$size", Value: 0}}}},
			bson.D{{Key: "data.opts.rooms", Value: bson.D{{Key: "$in", Value: session.Rooms}}}},
		}}},
		bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "data.opts.except", Value: bson.D{{Key: "$size", Value: 0}}}},
			bson.D{{Key: "data.opts.except", Value: bson.D{{Key: "$nin", Value: session.Rooms}}}},
		}}},
	}}}
	cursor, err := a.mongoCollection.Collection.Find(
		a.mongoCollection.Context,
		filter,
		options.Find().SetProjection(bson.D{
			{Key: "data.packet.data", Value: 1},
			{Key: "_id", Value: 0},
		}),
	)
	if err != nil {
		return nil, errFetchMissedPackets
	}
	defer func() { _ = cursor.Close(a.mongoCollection.Context) }()

	missedPackets := make([]any, 0)
	for cursor.Next(a.mongoCollection.Context) {
		var event struct {
			Data struct {
				Packet struct {
					Data any `bson:"data"`
				} `bson:"packet"`
			} `bson:"data"`
		}
		if err := mongo.UnmarshalDocument(cursor.Current, &event); err != nil {
			return nil, errFetchMissedPackets
		}
		if event.Data.Packet.Data != nil {
			missedPackets = append(missedPackets, event.Data.Packet.Data)
		}
	}
	if cursor.Err() != nil {
		return nil, errFetchMissedPackets
	}

	return &socket.Session{
		SessionToPersist: &socket.SessionToPersist{
			Sid:   session.Sid,
			Pid:   session.Pid,
			Rooms: types.NewSet(session.Rooms...),
			Data:  session.Data,
		},
		MissedPackets: missedPackets,
	}, nil
}

func (a *mongoAdapter) findSession(pid socket.PrivateSessionId) (*mongo.SessionDocument, error) {
	filter := bson.D{
		{Key: "type", Value: mongo.SESSION},
		{Key: "data.pid", Value: pid},
	}
	projection := bson.D{
		{Key: "data", Value: 1},
		{Key: "_id", Value: 0},
	}

	var result *mongod.SingleResult
	if a.addCreatedAtField {
		result = a.mongoCollection.Collection.FindOneAndDelete(
			a.mongoCollection.Context,
			filter,
			options.FindOneAndDelete().SetProjection(projection),
		)
	} else {
		result = a.mongoCollection.Collection.FindOne(
			a.mongoCollection.Context,
			filter,
			options.FindOne().SetProjection(projection).SetSort(bson.D{{Key: "_id", Value: -1}}),
		)
	}

	raw, err := result.Raw()
	if errors.Is(err, mongod.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var event struct {
		Data *mongo.SessionDocument `bson:"data"`
	}
	if err := mongo.UnmarshalDocument(raw, &event); err != nil {
		return nil, err
	}
	if event.Data == nil || event.Data.Sid == "" {
		return nil, nil
	}

	if !a.addCreatedAtField {
		a.Publish(&ClusterMessage{
			Type: mongo.SESSION,
			Data: &mongo.SessionTombstone{
				Pid:       pid,
				Tombstone: true,
			},
		})
	}
	return event.Data, nil
}

func (a *mongoAdapter) addOffsetIfNecessary(packet *parser.Packet, opts *socket.BroadcastOptions, offset adapter.Offset) {
	if packet == nil || a.Nsp().Server().Opts().ConnectionStateRecovery() == nil ||
		packet.Type != parser.EVENT || packet.Id != nil ||
		(opts != nil && opts.Flags != nil && opts.Flags.Volatile) {
		return
	}
	if data, ok := packet.Data.([]any); ok {
		data = append(data, offset)
		packet.Data = data
	}
}

func socketDetailsToResponses(sockets []socket.SocketDetails) []adapter.SocketResponse {
	responses := make([]adapter.SocketResponse, len(sockets))
	for i, client := range sockets {
		var rooms []socket.Room
		if clientRooms := client.Rooms(); clientRooms != nil {
			rooms = clientRooms.Keys()
		}
		responses[i] = adapter.SocketResponse{
			Id:        client.Id(),
			Handshake: client.Handshake(),
			Rooms:     rooms,
			Data:      client.Data(),
		}
	}
	return responses
}

func (a *mongoAdapter) Cleanup(cleanup func()) {
	if cleanup == nil {
		a.cleanupFunc.Store(nil)
		return
	}
	a.cleanupFunc.Store(&cleanup)
	if a.isClosed.Load() {
		if callback := a.cleanupFunc.Swap(nil); callback != nil {
			(*callback)()
		}
	}
}

func (a *mongoAdapter) Close() {
	if !a.isClosed.CompareAndSwap(false, true) {
		return
	}
	utils.ClearTimeout(a.heartbeatTimer.Swap(nil))
	if callback := a.cleanupFunc.Swap(nil); callback != nil {
		(*callback)()
	}
}
