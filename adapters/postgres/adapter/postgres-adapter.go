// Package adapter provides a PostgreSQL-based adapter implementation for Socket.IO clustering.
// It uses PostgreSQL LISTEN/NOTIFY for pub/sub communication between nodes, with an attachment
// table for payloads that exceed the 8000-byte NOTIFY limit.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// postgresLog is the logger for the PostgreSQL adapter.
var postgresLog = log.NewLog("socket.io-postgres")

// postgresAdapter implements the PostgresAdapter interface using PostgreSQL LISTEN/NOTIFY.
// It extends ClusterAdapterWithHeartbeat with PostgreSQL-specific functionality for
// message publishing and notification handling.
type postgresAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	postgresClient *postgres.PostgresClient
	opts           *PostgresAdapterOptions
	channel        string
	messages       *queue.Queue
	cleanupFunc    atomic.Pointer[types.Callable]
	isClosed       atomic.Bool
}

// MakePostgresAdapter creates a new uninitialized postgresAdapter.
// Call Construct() to complete initialization before use.
func MakePostgresAdapter() PostgresAdapter {
	a := &postgresAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
		opts:                        DefaultPostgresAdapterOptions(),
		messages:                    queue.New(),
	}

	a.Prototype(a)

	return a
}

// NewPostgresAdapter creates and initializes a PostgreSQL namespace adapter.
// When installing it on a server, use PostgresAdapterBuilder so the shared
// LISTEN connection and namespace routing are registered.
func NewPostgresAdapter(nsp socket.Namespace, client *postgres.PostgresClient, opts any) PostgresAdapter {
	a := MakePostgresAdapter()

	a.SetPostgres(client)
	a.SetOpts(opts)
	a.Construct(nsp)

	return a
}

// SetPostgres sets the PostgreSQL client for the adapter.
func (a *postgresAdapter) SetPostgres(client *postgres.PostgresClient) {
	a.postgresClient = client
}

// SetOpts sets the configuration options for the adapter.
// Options are merged with the parent ClusterAdapterWithHeartbeat options.
func (a *postgresAdapter) SetOpts(opts any) {
	a.ClusterAdapterWithHeartbeat.SetOpts(opts)

	if options, ok := opts.(PostgresAdapterOptionsInterface); ok {
		a.opts.Assign(options)
	}
}

// Construct initializes the PostgreSQL adapter for the given namespace.
// This method must be called before using the adapter.
func (a *postgresAdapter) Construct(nsp socket.Namespace) {
	a.ClusterAdapterWithHeartbeat.Construct(nsp)

	if a.opts.GetRawChannelPrefix() == nil {
		a.opts.SetChannelPrefix(DefaultChannelPrefix)
	}
	if a.opts.GetRawTableName() == nil {
		a.opts.SetTableName(DefaultTableName)
	}
	if a.opts.GetRawPayloadThreshold() == nil {
		a.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	}
	if a.opts.GetRawCleanupInterval() == nil {
		a.opts.SetCleanupInterval(DefaultCleanupInterval)
	}
	if a.opts.ErrorHandler() == nil {
		a.opts.SetErrorHandler(func(err error) {
			postgresLog.Debug("%s", err.Error())
		})
	}
	a.channel = a.opts.ChannelPrefix() + "#" + nsp.Name()
}

// PreparePublish selects NOTIFY or an attachment and encodes before queueing.
// Database timeouts start when the returned function actually executes.
func (a *postgresAdapter) PreparePublish(message *ClusterMessage) (_ adapter.PublishFunc, err error) {
	reportError := func(err error) {
		if err != nil && a.postgresClient.Context().Err() == nil {
			go a.onError(err)
		}
	}
	defer func() { reportError(err) }()
	wireMessage := *message
	wireData, binary, err := postgres.MarshalAdapterData(message.Data)
	if err != nil {
		return nil, err
	}
	wireMessage.Data = wireData
	var payload []byte
	if !binary {
		payload, err = json.Marshal(&wireMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to encode message: %w", err)
		}
	}
	attachment := binary || len(payload) >= a.opts.PayloadThreshold()
	if attachment {
		payload, err = utils.MsgPack().Encode(&wireMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to msgpack-encode message: %w", err)
		}
	}
	channel, table := a.channel, a.opts.TableName()
	uid, messageType := message.Uid, message.Type
	return func() (_ adapter.Offset, err error) {
		ctx, cancel := context.WithTimeout(a.postgresClient.Context(), postgres.DefaultOperationTimeout)
		defer cancel()
		defer func() { reportError(err) }()
		if !attachment {
			return "", a.postgresClient.Notify(ctx, channel, string(payload))
		}
		id, err := a.postgresClient.InsertAttachment(ctx, table, payload)
		if err != nil {
			return "", err
		}
		notification, err := json.Marshal(&NotificationMessage{
			Uid: uid, Type: messageType, AttachmentId: strconv.FormatInt(id, 10),
		})
		if err != nil {
			return "", err
		}
		return "", a.postgresClient.Notify(ctx, channel, string(notification))
	}, nil
}

// PreparePublishResponse uses the same NOTIFY/attachment encoding for responses.
func (a *postgresAdapter) PreparePublishResponse(_ adapter.ServerId, response *ClusterResponse) (adapter.PublishFunc, error) {
	return a.PreparePublish(response)
}

// OnNotification processes a raw notification payload received from PostgreSQL LISTEN/NOTIFY.
// It handles both direct JSON payloads and attachment references (msgpack-encoded in the DB).
func (a *postgresAdapter) OnNotification(payload string) {
	notification, err := parseNotification(payload)
	if err != nil {
		a.onError(err)
		return
	}

	message, err := a.decodeReceivedNotification(a.postgresClient.Context(), notification)
	if err != nil {
		a.onError(err)
		return
	}
	if message != nil && !a.isClosed.Load() {
		a.OnMessage(message, "")
	}
}

func parseNotification(payload string) (*NotificationMessage, error) {
	var notification NotificationMessage
	if err := json.Unmarshal([]byte(payload), &notification); err != nil {
		return nil, fmt.Errorf("failed to parse notification: %w", err)
	}
	return &notification, nil
}

func (a *postgresAdapter) decodeReceivedNotification(ctx context.Context, notification *NotificationMessage) (*ClusterResponse, error) {
	// Check if this is from ourselves
	if notification.Uid == a.Uid() {
		return nil, nil
	}

	var (
		message *ClusterResponse
		err     error
	)
	if notification.AttachmentId != "" {
		attachmentId, parseErr := strconv.ParseInt(notification.AttachmentId, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid attachment ID %q: %w", notification.AttachmentId, parseErr)
		}

		fetchCtx, cancel := context.WithTimeout(ctx, postgres.DefaultOperationTimeout)
		defer cancel()
		attachmentPayload, fetchErr := a.postgresClient.GetAttachment(
			fetchCtx,
			a.opts.TableName(),
			attachmentId,
		)
		if fetchErr != nil {
			return nil, fmt.Errorf("failed to fetch attachment %d: %w", attachmentId, fetchErr)
		}

		// Attachment payloads are msgpack-encoded (matches Node.js: decode(result.rows[0].payload))
		message, err = a.decodeMsgpack(attachmentPayload)
	} else {
		// Direct NOTIFY payload: decode as JSON.
		message, err = a.decodeNotification(notification)
	}
	if err != nil {
		return nil, err
	}

	nsp := a.Nsp().Name()
	if message.Nsp == "" && message.Uid == adapter.EMITTER_UID {
		// The Node.js emitter 0.1.x omits the top-level namespace and relies on the notification channel.
		message.Nsp = nsp
	}
	if message.Nsp != nsp {
		return nil, nil
	}

	return message, nil
}

func (a *postgresAdapter) onError(err error) {
	if handler := a.opts.ErrorHandler(); handler != nil {
		handler(err)
		return
	}
	postgresLog.Debug("%s", err.Error())
}

func (a *postgresAdapter) decodeNotification(notification *NotificationMessage) (*ClusterResponse, error) {
	message := &adapter.ClusterMessage{
		Uid:  notification.Uid,
		Nsp:  notification.Nsp,
		Type: notification.Type,
	}

	if len(notification.Data) == 0 {
		return message, nil
	}

	target := postgres.AdapterDataTarget(message.Type)
	if target == nil {
		return message, nil
	}
	if err := json.Unmarshal(notification.Data, target); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON data: %w", err)
	}
	message.Data = postgres.UnmarshalAdapterData(message.Type, target)

	return message, nil
}

// decodeMsgpack converts a msgpack-encoded attachment payload into a typed ClusterResponse.
// This handles binary/large messages stored in the attachment table.
func (a *postgresAdapter) decodeMsgpack(payload []byte) (*ClusterResponse, error) {
	var raw struct {
		Uid  adapter.ServerId    `msgpack:"uid,omitempty"`
		Nsp  string              `msgpack:"nsp,omitempty"`
		Type adapter.MessageType `msgpack:"type,omitempty"`
		Data msgpack.RawMessage  `msgpack:"data,omitempty"`
	}

	if err := utils.MsgPack().Decode(payload, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode msgpack message: %w", err)
	}

	message := &adapter.ClusterMessage{
		Uid:  raw.Uid,
		Nsp:  raw.Nsp,
		Type: raw.Type,
	}

	if len(raw.Data) == 0 {
		return message, nil
	}

	target := postgres.AdapterDataTarget(message.Type)
	if target == nil {
		return message, nil
	}
	if err := utils.MsgPack().Decode(raw.Data, target); err != nil {
		return nil, fmt.Errorf("failed to decode MessagePack data: %w", err)
	}
	message.Data = postgres.UnmarshalAdapterData(message.Type, target)

	return message, nil
}

// Cleanup registers a cleanup callback to be called when the adapter is closed.
func (a *postgresAdapter) Cleanup(cleanup func()) {
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

// Close releases resources and invokes the registered cleanup callback.
func (a *postgresAdapter) Close() {
	if !a.isClosed.CompareAndSwap(false, true) {
		return
	}
	a.messages.TryClose()
	a.ClusterAdapterWithHeartbeat.Close()

	if callback := a.cleanupFunc.Swap(nil); callback != nil {
		(*callback)()
	}
}
