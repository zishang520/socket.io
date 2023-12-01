package socket

import (
	"github.com/zishang520/engine.io/v2/events"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type BroadcastAdapter func(*parser.Packet, *BroadcastOptions)

func (BroadcastAdapter) AddListener(events.EventName, ...events.Listener) error {
	return nil
}
func (BroadcastAdapter) Emit(events.EventName, ...any) {}
func (BroadcastAdapter) EventNames() []events.EventName {
	return nil
}
func (BroadcastAdapter) GetMaxListeners() uint {
	return 0
}
func (BroadcastAdapter) ListenerCount(events.EventName) int {
	return 0
}
func (BroadcastAdapter) Listeners(events.EventName) []events.Listener {
	return nil
}
func (BroadcastAdapter) On(events.EventName, ...events.Listener) error {
	return nil
}
func (BroadcastAdapter) Once(events.EventName, ...events.Listener) error {
	return nil
}
func (BroadcastAdapter) RemoveAllListeners(events.EventName) bool {
	return false
}
func (BroadcastAdapter) RemoveListener(events.EventName, events.Listener) bool {
	return false
}
func (BroadcastAdapter) Clear()               {}
func (BroadcastAdapter) SetMaxListeners(uint) {}
func (BroadcastAdapter) Len() int {
	return 0
}
func (BroadcastAdapter) Prototype(Adapter) {
}
func (ba BroadcastAdapter) Proto() Adapter {
	return ba
}

// Construct() should be called after calling Prototype()
func (BroadcastAdapter) Construct(NamespaceInterface) {}
func (BroadcastAdapter) Rooms() *types.Map[Room, *types.Set[SocketId]] {
	return nil
}
func (BroadcastAdapter) Sids() *types.Map[SocketId, *types.Set[Room]] {
	return nil
}
func (BroadcastAdapter) Nsp() NamespaceInterface {
	return nil
}
func (BroadcastAdapter) Init()  {}
func (BroadcastAdapter) Close() {}
func (BroadcastAdapter) ServerCount() int64 {
	return 0
}
func (BroadcastAdapter) AddAll(SocketId, *types.Set[Room]) {}
func (BroadcastAdapter) Del(SocketId, Room)                {}
func (BroadcastAdapter) DelAll(SocketId)                   {}
func (ba BroadcastAdapter) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	ba(packet, opts)
}
func (BroadcastAdapter) BroadcastWithAck(*parser.Packet, *BroadcastOptions, func(uint64), func([]any, error)) {
}
func (BroadcastAdapter) Sockets(*types.Set[Room]) *types.Set[SocketId] {
	return nil
}
func (BroadcastAdapter) SocketRooms(SocketId) *types.Set[Room] {
	return nil
}
func (BroadcastAdapter) FetchSockets(*BroadcastOptions) func(func([]SocketDetails, error)) {
	return nil
}
func (BroadcastAdapter) AddSockets(*BroadcastOptions, []Room)      {}
func (BroadcastAdapter) DelSockets(*BroadcastOptions, []Room)      {}
func (BroadcastAdapter) DisconnectSockets(*BroadcastOptions, bool) {}
func (BroadcastAdapter) ServerSideEmit([]any) error {
	return nil
}
func (BroadcastAdapter) PersistSession(*SessionToPersist) {
}
func (BroadcastAdapter) RestoreSession(PrivateSessionId, string) (*Session, error) {
	return nil, nil
}
