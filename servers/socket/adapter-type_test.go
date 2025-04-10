package socket

import (
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestBroadcastFlagsInheritance(t *testing.T) {
	flags := BroadcastFlags{
		WriteOptions: WriteOptions{
			Volatile:   true,
			PreEncoded: false,
			Options: packet.Options{
				Compress: utils.Ptr(true),
			},
		},
		Local:     true,
		Broadcast: false,
		Binary:    true,
	}

	if flags.Volatile != true {
		t.Errorf("Expected Volatile to be true, got %v", flags.Volatile)
	}
	if flags.PreEncoded != false {
		t.Errorf("Expected PreEncoded to be false, got %v", flags.PreEncoded)
	}
	if flags.Local != true {
		t.Errorf("Expected Local to be true, got %v", flags.Local)
	}
	if flags.Broadcast != false {
		t.Errorf("Expected Broadcast to be false, got %v", flags.Broadcast)
	}
	if flags.Binary != true {
		t.Errorf("Expected Binary to be true, got %v", flags.Binary)
	}
	if flags.Options.Compress != nil && *flags.Options.Compress != true {
		t.Errorf("Expected Options.Compress to be true, got %v", flags.Options.Compress)
	}
}

func TestBroadcastOptionsInheritance(t *testing.T) {
	rooms := types.NewSet[Room]()
	except := types.NewSet[Room]()

	flags := &BroadcastFlags{
		WriteOptions: WriteOptions{
			Volatile:   false,
			PreEncoded: true,
			Options:    packet.Options{},
		},
		Local:     true,
		Broadcast: true,
		Binary:    false,
	}

	opts := BroadcastOptions{
		Rooms:  rooms,
		Except: except,
		Flags:  flags,
	}

	if opts.Flags != flags {
		t.Errorf("Expected Flags to be correctly embedded in BroadcastOptions, got %v", opts.Flags)
	}
	if opts.Rooms != rooms {
		t.Errorf("Expected Rooms to be set correctly, got %v", opts.Rooms)
	}
	if opts.Except != except {
		t.Errorf("Expected Except to be set correctly, got %v", opts.Except)
	}
}

func TestSessionInheritance(t *testing.T) {
	rooms := types.NewSet[Room]()

	session := Session{
		SessionToPersist: &SessionToPersist{
			Sid:   "socket123",
			Pid:   "private123",
			Rooms: rooms,
			Data:  "sample data",
		},
		MissedPackets: []any{"packet1", "packet2"},
	}

	if session.Sid != "socket123" {
		t.Errorf("Expected Sid to be 'socket123', got %v", session.Sid)
	}
	if session.Pid != "private123" {
		t.Errorf("Expected Pid to be 'private123', got %v", session.Pid)
	}
	if session.Rooms != rooms {
		t.Errorf("Expected Rooms to be set correctly, got %v", session.Rooms)
	}
	if session.Data != "sample data" {
		t.Errorf("Expected Data to be 'sample data', got %v", session.Data)
	}
	if len(session.MissedPackets) != 2 {
		t.Errorf("Expected MissedPackets to contain 2 packets, got %d", len(session.MissedPackets))
	}
}

func TestPersistedPacket(t *testing.T) {
	flags := &BroadcastFlags{
		WriteOptions: WriteOptions{
			Volatile:   true,
			PreEncoded: true,
		},
		Local:     true,
		Broadcast: false,
	}

	opts := &BroadcastOptions{
		Rooms:  types.NewSet[Room](),
		Except: types.NewSet[Room](),
		Flags:  flags,
	}

	packet := PersistedPacket{
		Id:        "packet1",
		EmittedAt: time.Now().Unix(),
		Data:      "packet data",
		Opts:      opts,
	}

	if packet.Id != "packet1" {
		t.Errorf("Expected Id to be 'packet1', got %v", packet.Id)
	}
	if packet.Opts != opts {
		t.Errorf("Expected BroadcastOptions to be set correctly, got %v", packet.Opts)
	}
	if packet.Data != "packet data" {
		t.Errorf("Expected Data to be 'packet data', got %v", packet.Data)
	}
}

func TestSessionWithTimestampInheritance(t *testing.T) {
	sessionWithTimestamp := SessionWithTimestamp{
		SessionToPersist: &SessionToPersist{
			Sid:   "sid123",
			Pid:   "pid123",
			Rooms: types.NewSet[Room](),
			Data:  "some data",
		},
		DisconnectedAt: time.Now().Unix(),
	}

	if sessionWithTimestamp.Sid != "sid123" {
		t.Errorf("Expected Sid to be 'sid123', got %v", sessionWithTimestamp.Sid)
	}
	if sessionWithTimestamp.Pid != "pid123" {
		t.Errorf("Expected Pid to be 'pid123', got %v", sessionWithTimestamp.Pid)
	}
	if sessionWithTimestamp.Rooms == nil {
		t.Errorf("Expected Rooms to be set, got nil")
	}
	if sessionWithTimestamp.Data != "some data" {
		t.Errorf("Expected Data to be 'some data', got %v", sessionWithTimestamp.Data)
	}
	if sessionWithTimestamp.DisconnectedAt <= 0 {
		t.Errorf("Expected DisconnectedAt to be a valid timestamp, got %d", sessionWithTimestamp.DisconnectedAt)
	}
}
