package parser

import (
	"sync"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Protocol is the Socket.IO protocol version.
const Protocol = 5

// binaryReconstructor manages the reconstruction of binary packets.
// It collects binary buffers until the expected number of attachments
// is received, then reconstructs the complete packet.
type binaryReconstructor struct {
	mu      sync.Mutex
	buffers []types.BufferInterface
	packet  atomic.Pointer[Packet]
}

// newBinaryReconstructor creates a new binaryReconstructor for the given packet.
// The packet should have its Attachments field set to the expected number of buffers.
func newBinaryReconstructor(packet *Packet) *binaryReconstructor {
	br := &binaryReconstructor{
		buffers: make([]types.BufferInterface, 0),
	}
	br.packet.Store(packet)
	return br
}

// takeBinaryData adds a binary buffer to the reconstruction.
// Returns the reconstructed packet when all expected buffers are received,
// or nil if more buffers are needed.
func (br *binaryReconstructor) takeBinaryData(data types.BufferInterface) (*Packet, error) {
	br.mu.Lock()
	defer br.mu.Unlock()

	br.buffers = append(br.buffers, data)

	packet := br.packet.Load()
	if packet == nil || packet.Attachments == nil {
		return nil, nil
	}

	// Check if all expected buffers have been received
	if uint64(len(br.buffers)) == *packet.Attachments {
		reconstructedPacket, err := ReconstructPacket(packet, br.buffers)
		br.reset()
		return reconstructedPacket, err
	}

	return nil, nil
}

// reset clears the reconstruction state.
func (br *binaryReconstructor) reset() {
	br.buffers = nil
	br.packet.Store(nil)
}

// finishedReconstruction signals that reconstruction is complete or cancelled.
// This should be called when the decoder is destroyed or reset.
func (br *binaryReconstructor) finishedReconstruction() {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.reset()
}
