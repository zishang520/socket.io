// Copyright 2017 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webtransport

// PreparedMessage owns the encoded representation of a message payload so it
// can be shared by multiple connections without encoding or copying it again.
type PreparedMessage struct {
	frame []byte
}

// NewPreparedMessage copies and encodes data for use with WritePreparedMessage.
func NewPreparedMessage(messageType int, data []byte) (*PreparedMessage, error) {
	if !isData(messageType) {
		return nil, errBadWriteOpCode
	}
	var header [maxFrameHeaderSize]byte
	start := putFrameHeader(header[:], messageType, len(data))
	headerLen := maxFrameHeaderSize - start
	frame := make([]byte, headerLen+len(data))
	copy(frame, header[start:])
	copy(frame[headerLen:], data)
	return &PreparedMessage{frame: frame}, nil
}
