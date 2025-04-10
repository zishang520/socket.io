package types

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	RedisPacket struct {
		Uid    adapter.ServerId       `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Packet *parser.Packet         `json:"packet,omitempty" msgpack:"packet,omitempty"`
		Opts   *adapter.PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
	}

	RedisRequest struct {
		Type      adapter.MessageType    `json:"type,omitempty" msgpack:"type,omitempty"`
		RequestId string                 `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms     []socket.Room          `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Opts      *adapter.PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		Sid       socket.SocketId        `json:"sid,omitempty" msgpack:"sid,omitempty"`
		Room      socket.Room            `json:"room,omitempty" msgpack:"room,omitempty"`
		Close     bool                   `json:"close,omitempty" msgpack:"close,omitempty"`
		Uid       adapter.ServerId       `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Data      []any                  `json:"data,omitempty" msgpack:"data,omitempty"`
		Packet    *parser.Packet         `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	RedisResponse struct {
		Type        adapter.MessageType       `json:"type,omitempty" msgpack:"type,omitempty"`
		RequestId   string                    `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms       []socket.Room             `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Sockets     []*adapter.SocketResponse `json:"sockets,omitempty" msgpack:"sockets,omitempty"`
		Data        []any                     `json:"data,omitempty" msgpack:"data,omitempty"`
		ClientCount uint64                    `json:"clientcount,omitempty" msgpack:"clientcount,omitempty"`
		Packet      []any                     `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	Parser interface {
		Encode(any) ([]byte, error)
		Decode([]byte, any) error
	}

	Map[Tkey comparable, TValue any] = types.Map[Tkey, TValue]
	Set[TValue comparable]           = types.Set[TValue]
	Slice[TValue any]                = types.Slice[TValue]
	Callable                         = types.Callable
)

func NewSet[KType comparable](keys ...KType) *Set[KType] {
	return types.NewSet(keys...)
}

func NewSlice[T any](elements ...T) *Slice[T] {
	return types.NewSlice(elements...)
}
