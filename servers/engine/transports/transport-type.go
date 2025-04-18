// Package transports provides implementations for Engine.IO transport types and constants.
package transports

import (
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	Transport interface {
		// Extends

		types.EventEmitter

		// Prototype

		Prototype(Transport)
		Proto() Transport

		// Setters

		SetSid(string)
		SetWritable(bool)

		SetSupportsBinary(bool)
		SetReadyState(string)
		SetHttpCompression(*types.HttpCompression)
		SetPerMessageDeflate(*types.PerMessageDeflate)
		SetMaxHttpBufferSize(int64)

		// Getters

		// The session ID.
		Sid() string
		// Whether the transport is currently ready to send packets.
		Writable() bool
		// The revision of the protocol:
		//
		// - 3 is used in Engine.IO v3 / Socket.IO v2
		// - 4 is used in Engine.IO v4 and above / Socket.IO v3 and above
		//
		// It is found in the `EIO` query parameters of the HTTP requests.
		//
		// See https://github.com/socketio/engine.io-protocol
		Protocol() int
		// Whether the transport is discarded and can be safely closed (used during upgrade).
		//
		// Protected
		Discarded() bool
		// The parser to use (depends on the revision of the {@link Transport#protocol}.
		//
		// Protected
		Parser() parser.Parser
		// Whether the transport supports binary payloads (else it will be base64-encoded)
		//
		// Protected
		SupportsBinary() bool
		// The current state of the transport.
		//
		// Protected
		ReadyState() string
		HttpCompression() *types.HttpCompression
		PerMessageDeflate() *types.PerMessageDeflate
		MaxHttpBufferSize() int64
		// Abstract
		HandlesUpgrades() bool
		// Abstract
		Name() string

		// Methods

		// [Transport.Construct] should be called after calling [Transport.Prototype]
		Construct(*types.HttpContext)
		// Private
		//
		// Flags the transport as discarded.
		Discard()
		// Protected
		//
		// Called with an incoming HTTP request.
		OnRequest(*types.HttpContext)
		// Private
		//
		// Closes the transport.
		Close(...types.Callable)
		// Protected
		//
		// Called with a transport error.
		OnError(string, error)
		// Protected
		//
		// Called with parsed out a packets from the data stream.
		OnPacket(*packet.Packet)
		// Protected
		//
		// Called with the encoded packet data.
		OnData(types.BufferInterface)
		// Protected
		//
		// Called upon transport close.
		OnClose()
		// Protected
		//
		// Writes a packet payload.
		Send([]*packet.Packet)
		// Protected
		//
		// Closes the transport.
		DoClose(types.Callable)
	}

	Polling interface {
		// Extends

		Transport

		DoWrite(*types.HttpContext, types.BufferInterface, *packet.Options, func(error))
	}

	Jsonp interface {
		// Extends

		Polling
	}

	Websocket interface {
		// Extends

		Transport
	}

	WebTransport interface {
		// Extends

		Transport
	}

	TransportCtor interface {
		New(*types.HttpContext) Transport
		HandlesUpgrades() bool
		// Todo: Return []string
		UpgradesTo() *types.Set[string]
	}
)
