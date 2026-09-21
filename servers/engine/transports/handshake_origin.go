package transports

import "context"

// handshakeOriginKey is an unexported context key type so values set by
// WithHandshakeOrigin cannot collide with keys defined by other packages.
type handshakeOriginKey struct{}

// WithHandshakeOrigin marks ctx as having originated from the Engine.IO
// server's own ServeHTTP entry point (HandleRequest / HandleUpgrade).
//
// transport.Construct consults this marker to decide the transport's initial
// packet-delivery gate state: when present, inbound packets are buffered
// until the caller signals readiness via Transport.ReleaseGate, closing the
// race window between the transport's reader goroutine starting and the
// caller finishing "packet" listener registration (see transport.go). When
// absent, the gate starts open and behaves exactly as before this marker was
// introduced.
//
// Known limitation: OnWebTransportSession does not necessarily receive a
// context that has passed through ServeHTTP — a WebTransport integration
// that terminates QUIC itself and builds its own *http.Request/context is
// responsible for calling WithHandshakeOrigin explicitly if it wants the
// gate. Without it, the gate simply stays open (today's behavior), so this
// is a gap in coverage, not a regression.
func WithHandshakeOrigin(ctx context.Context) context.Context {
	return context.WithValue(ctx, handshakeOriginKey{}, true)
}

// isHandshakeOrigin reports whether ctx carries the WithHandshakeOrigin marker.
func isHandshakeOrigin(ctx context.Context) bool {
	marked, _ := ctx.Value(handshakeOriginKey{}).(bool)
	return marked
}
