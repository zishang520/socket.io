package utils

import (
	"github.com/vmihailenco/msgpack/v5"
)

type msgPack struct {
}

var defaultMsgpack = &msgPack{}

func MsgPack() *msgPack {
	return defaultMsgpack
}

func (m *msgPack) Encode(value any) ([]byte, error) {
	return msgpack.Marshal(value)
}

func (m *msgPack) Decode(data []byte, value any) error {
	return msgpack.Unmarshal(data, value)
}
