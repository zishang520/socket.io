package transports

import (
	"github.com/zishang520/socket.io/servers/engine/v3/types"
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

// Todo: Return []string
func (*WebSocketBuilder) UpgradesTo() *types.Set[string] {
	return types.NewSet[string]()
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

// Todo: Return []string
func (*WebTransportBuilder) UpgradesTo() *types.Set[string] {
	return types.NewSet[string]()
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

// Todo: Return []string
func (*PollingBuilder) UpgradesTo() *types.Set[string] {
	return types.NewSet(WEBSOCKET, WEBTRANSPORT)
}
