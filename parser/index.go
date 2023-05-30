package parser

import (
	"sync"

	"github.com/zishang520/engine.io-go-parser/types"
)

// Protocol version.
const Protocol = 5

// A manager of a binary event's 'buffer sequence'. Should
// be constructed whenever a packet of type BINARY_EVENT is
// decoded.
type binaryreconstructor struct {
	buffers   []types.BufferInterface
	reconPack *Packet

	mu sync.Mutex
}

func NewBinaryReconstructor(packet *Packet) *binaryreconstructor {
	return &binaryreconstructor{
		buffers:   []types.BufferInterface{},
		reconPack: packet,
	}
}

// Method to be called when binary data received from connection
// after a BINARY_EVENT packet.
func (b *binaryreconstructor) takeBinaryData(binData types.BufferInterface) (*Packet, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.reconPack == nil {
		return nil, nil
	}

	b.buffers = append(b.buffers, binData)

	if attachments := b.reconPack.Attachments; attachments != nil && uint64(len(b.buffers)) == *attachments {
		// done with buffer list
		packet, err := ReconstructPacket(b.reconPack, b.buffers)
		if err != nil {
			return nil, err
		}
		b.reconPack = nil
		b.buffers = nil

		return packet, nil
	}
	return nil, nil
}

// Cleans up binary packet reconstruction variables.
func (b *binaryreconstructor) finishedReconstruction() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.reconPack = nil
	b.buffers = nil
}
