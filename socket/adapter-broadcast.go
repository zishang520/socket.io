package socket

import (
	"github.com/zishang520/engine.io/v2/events"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type AdapterBroadcast func(*parser.Packet, *BroadcastOptions)

func (AdapterBroadcast) AddListener(events.EventName, ...events.Listener) error {
	return nil
}
func (AdapterBroadcast) Emit(events.EventName, ...any) {}
func (AdapterBroadcast) EventNames() []events.EventName {
	return nil
}
func (AdapterBroadcast) GetMaxListeners() uint {
	return 0
}
func (AdapterBroadcast) ListenerCount(events.EventName) int {
	return 0
}
func (AdapterBroadcast) Listeners(events.EventName) []events.Listener {
	return nil
}
func (AdapterBroadcast) On(events.EventName, ...events.Listener) error {
	return nil
}
func (AdapterBroadcast) Once(events.EventName, ...events.Listener) error {
	return nil
}
func (AdapterBroadcast) RemoveAllListeners(events.EventName) bool {
	return false
}
func (AdapterBroadcast) RemoveListener(events.EventName, events.Listener) bool {
	return false
}
func (AdapterBroadcast) Clear()               {}
func (AdapterBroadcast) SetMaxListeners(uint) {}
func (AdapterBroadcast) Len() int {
	return 0
}
func (AdapterBroadcast) Rooms() *types.Map[Room, *types.Set[SocketId]] {
	return nil
}
func (AdapterBroadcast) Sids() *types.Map[SocketId, *types.Set[Room]] {
	return nil
}
func (AdapterBroadcast) Nsp() NamespaceInterface {
	return nil
}
func (AdapterBroadcast) Init()  {}
func (AdapterBroadcast) Close() {}
func (AdapterBroadcast) ServerCount() int64 {
	return 0
}
func (AdapterBroadcast) AddAll(SocketId, *types.Set[Room])                    {}
func (AdapterBroadcast) Del(SocketId, Room)                                   {}
func (AdapterBroadcast) DelAll(SocketId)                                      {}
func (AdapterBroadcast) SetBroadcast(func(*parser.Packet, *BroadcastOptions)) {}
func (AdapterBroadcast) GetBroadcast() func(*parser.Packet, *BroadcastOptions) {
	return nil
}
func (ab AdapterBroadcast) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	ab(packet, opts)
}
func (AdapterBroadcast) BroadcastWithAck(*parser.Packet, *BroadcastOptions, func(uint64), func([]any, error)) {
}
func (AdapterBroadcast) Sockets(*types.Set[Room]) *types.Set[SocketId] {
	return nil
}
func (AdapterBroadcast) SocketRooms(SocketId) *types.Set[Room] {
	return nil
}
func (AdapterBroadcast) FetchSockets(*BroadcastOptions) func(func([]SocketDetails, error)) {
	return nil
}
func (AdapterBroadcast) AddSockets(*BroadcastOptions, []Room)      {}
func (AdapterBroadcast) DelSockets(*BroadcastOptions, []Room)      {}
func (AdapterBroadcast) DisconnectSockets(*BroadcastOptions, bool) {}
func (AdapterBroadcast) ServerSideEmit([]any) error {
	return nil
}
func (AdapterBroadcast) PersistSession(*SessionToPersist) {
}
func (AdapterBroadcast) RestoreSession(PrivateSessionId, string) (*Session, error) {
	return nil, nil
}
