package parser

import (
	"sync"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Protocol version.
const Protocol = 5

// binaryReconstructor manages a binary event's buffer sequence.
// It should be constructed whenever a packet of type BINARY_EVENT is decoded.
type binaryReconstructor struct {
	mu        sync.Mutex
	buffers   []types.BufferInterface
	reconPack atomic.Pointer[Packet]
}

// newBinaryReconstructor creates a new binaryReconstructor.
func newBinaryReconstructor(packet *Packet) *binaryReconstructor {
	br := &binaryReconstructor{
		buffers: []types.BufferInterface{},
	}
	br.reconPack.Store(packet)
	return br
}

// takeBinaryData handles incoming binary data for the reconstruction.
func (br *binaryReconstructor) takeBinaryData(binData types.BufferInterface) (*Packet, error) {
	br.mu.Lock()
	defer br.mu.Unlock()

	br.buffers = append(br.buffers, binData)

	// Check if reconPack and Attachments are valid
	if reconPack := br.reconPack.Load(); reconPack != nil && reconPack.Attachments != nil && uint64(len(br.buffers)) == *reconPack.Attachments {
		// Done with buffer list - reconstruct the packet
		packet, err := ReconstructPacket(reconPack, br.buffers)
		br.cleanUp()
		return packet, err
	}
	return nil, nil
}

// cleanUp cleans up the reconstruction state.
func (br *binaryReconstructor) cleanUp() {
	br.buffers = nil
	br.reconPack.Store(nil)
}

// finishedReconstruction cleans up the reconstruction state.
func (br *binaryReconstructor) finishedReconstruction() {
	br.mu.Lock()
	defer br.mu.Unlock()

	br.cleanUp()
}
