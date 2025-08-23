package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var (
	parser_log = log.NewLog("socket.io:parser")

	// These strings must not be used as event names, as they have a special meaning.
	RESERVED_EVENTS = types.NewSet(
		"connect",       // used on the client side
		"connect_error", // used on the client side
		"disconnect",    // used on both sides
		"disconnecting", // used on the server side
	)
)

// A socket.io Decoder instance
type decoder struct {
	types.EventEmitter

	reconstructor atomic.Pointer[binaryReconstructor]
}

func NewDecoder() Decoder {
	return &decoder{EventEmitter: types.NewEventEmitter()}
}

// Decodes an encoded packet string into packet JSON.
func (d *decoder) Add(data any) error {
	switch tdata := data.(type) {
	case string:
		if d.reconstructor.Load() != nil {
			return errors.New("got plaintext data when reconstructing a packet")
		}
		return d.decodeAsString(types.NewStringBufferString(tdata))
	case *strings.Reader:
		if d.reconstructor.Load() != nil {
			return errors.New("got plaintext data when reconstructing a packet")
		}
		rdata, err := types.NewStringBufferReader(tdata)
		if err != nil {
			return err
		}
		return d.decodeAsString(rdata)
	case *types.StringBuffer:
		if d.reconstructor.Load() != nil {
			return errors.New("got plaintext data when reconstructing a packet")
		}
		return d.decodeAsString(tdata)
	default:
		if !IsBinary(data) {
			return errors.New(fmt.Sprintf("Unknown type: %v", data))
		}

		// raw binary data
		reconstructor := d.reconstructor.Load()
		if reconstructor == nil {
			return errors.New("got binary data when not reconstructing a packet")
		}

		rdata := types.NewBytesBuffer(nil)
		switch tdata := data.(type) {
		case io.Reader:
			if c, ok := data.(io.Closer); ok {
				defer c.Close()
			}
			if _, err := rdata.ReadFrom(tdata); err != nil {
				return err
			}
		case []byte:
			if _, err := rdata.Write(tdata); err != nil {
				return err
			}
		}
		packet, err := reconstructor.takeBinaryData(rdata)
		if err != nil {
			return errors.New(fmt.Sprintf("Decode error: %v", err.Error()))
		}
		if packet != nil {
			// received final buffer
			d.reconstructor.Store(nil)
			d.Emit("decoded", packet)
		}
	}

	return nil
}

func (d *decoder) decodeAsString(str types.BufferInterface) error {
	packet, err := d.decodeString(str)
	if err != nil {
		parser_log.Debug("decode err %v", err)
		return err
	}
	if packet.Type == BINARY_EVENT || packet.Type == BINARY_ACK {
		// binary packet's json
		d.reconstructor.Store(newBinaryReconstructor(packet))
		// no attachments, labeled binary but no binary data to follow
		if attachments := packet.Attachments; attachments != nil && *attachments == 0 {
			d.Emit("decoded", packet)
		}
	} else {
		// non-binary full packet
		d.Emit("decoded", packet)
	}
	return nil
}

// Decode a packet String (JSON data)
func (d *decoder) decodeString(str types.BufferInterface) (packet *Packet, err error) {
	defer func(str string) {
		if err == nil {
			parser_log.Debug("decoded %s as %v", str, packet)
		}
	}(str.String())

	// look up type
	packet = &Packet{}
	msgType, err := str.ReadByte()
	if err != nil {
		return nil, errors.New("invalid payload")
	}
	packet.Type = PacketType(msgType)
	if !packet.Type.Valid() {
		return nil, errors.New(fmt.Sprintf("unknown packet type %d", packet.Type))
	}
	// look up attachments if type binary
	if packet.Type == BINARY_EVENT || packet.Type == BINARY_ACK {
		buf, err := str.ReadString('-')
		if err != nil {
			// The scan is over and it is not found '-' indicating that there is a problem.
			return nil, errors.New("Illegal attachments")
		}
		_l := len(buf)
		if _l < 2 { // 'xxx-'
			return nil, errors.New("Illegal attachments")
		}
		attachments, err := strconv.ParseUint(buf[:_l-1], 10, 64)
		if err != nil {
			return nil, errors.New("Illegal attachments")
		}
		packet.Attachments = &attachments
	}

	// look up namespace (if any)
	if nsp, err := str.ReadByte(); err != nil {
		if err != io.EOF {
			return nil, errors.New("Illegal namespace")
		}
		packet.Nsp = "/"
	} else {
		if '/' == nsp {
			_nsp, err := str.ReadString(',')
			if err != nil {
				if err != io.EOF {
					return nil, errors.New("Illegal namespace")
				}
				packet.Nsp = "/" + _nsp
			} else {
				packet.Nsp = "/" + _nsp[:len(_nsp)-1]
			}
		} else {
			if err := str.UnreadByte(); err != nil {
				return nil, errors.New("Illegal namespace")
			}
			packet.Nsp = "/"
		}
	}

	if str.Len() > 0 {
		// look up id
		id := new(strings.Builder)

		for {
			b, err := str.ReadByte()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			if '0' <= b && '9' >= b {
				if err := id.WriteByte(b); err != nil {
					return nil, err
				}
			} else {
				if err := str.UnreadByte(); err != nil {
					return nil, errors.New("Illegal id")
				}
				break
			}
		}

		if id.Len() > 0 {
			id, err := strconv.ParseUint(id.String(), 10, 64)
			if err != nil {
				return nil, err
			}
			packet.Id = &id
		}
	}

	var payload any

	// look up json data
	if str.Len() > 0 {
		if json.NewDecoder(str).Decode(&payload) != nil {
			return nil, errors.New("invalid payload")
		}
	}

	if !isPayloadValid(packet.Type, payload) {
		return nil, errors.New("invalid payload")
	}

	packet.Data = payload

	return packet, nil
}

func isPayloadValid(t PacketType, payload any) bool {
	switch t {
	case CONNECT:
		_, ok := payload.(map[string]any)
		return ok
	case DISCONNECT:
		return payload == nil
	case CONNECT_ERROR:
		_, ok := payload.(map[string]any)
		if !ok {
			_, ok = payload.(string)
		}
		return ok
	case EVENT, BINARY_EVENT:
		data, ok := payload.([]any)
		if ok && len(data) > 0 {
			event, isString := data[0].(string)
			return isString && !RESERVED_EVENTS.Has(event)
		}
	case ACK, BINARY_ACK:
		_, ok := payload.([]any)
		return ok
	}
	return false
}

// Deallocates a parser's resources
func (d *decoder) Destroy() {
	if reconstructor := d.reconstructor.Load(); reconstructor != nil {
		reconstructor.finishedReconstruction()
	}
}
