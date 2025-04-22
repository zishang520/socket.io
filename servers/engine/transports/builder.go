// Package transports provides builder types for Engine.IO transport registration and instantiation.
package transports

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type WebSocketBuilder struct{}

func (*WebSocketBuilder) New(ctx *types.HttpContext) Transport {
	return NewWebSocket(ctx)
}
func (*WebSocketBuilder) Name() string {
	return WEBSOCKET
}
func (*WebSocketBuilder) HandlesUpgrades() bool {
	return true
}

func (*WebSocketBuilder) UpgradesTo() []string {
	return nil
}

type WebTransportBuilder struct{}

func (*WebTransportBuilder) New(ctx *types.HttpContext) Transport {
	return NewWebTransport(ctx)
}
func (*WebTransportBuilder) Name() string {
	return WEBTRANSPORT
}
func (*WebTransportBuilder) HandlesUpgrades() bool {
	return true
}

func (*WebTransportBuilder) UpgradesTo() []string {
	return nil
}

type PollingBuilder struct{}

func (*PollingBuilder) New(ctx *types.HttpContext) Transport {
	if ctx.Query().Has("j") {
		return NewJSONP(ctx)
	}
	return NewPolling(ctx)
}
func (*PollingBuilder) Name() string {
	return POLLING
}
func (*PollingBuilder) HandlesUpgrades() bool {
	return false
}

func (*PollingBuilder) UpgradesTo() []string {
	return []string{WEBSOCKET, WEBTRANSPORT}
}
