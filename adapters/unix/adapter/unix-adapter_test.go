package adapter

import (
	"encoding/json"
	"testing"

	cluster "github.com/zishang520/socket.io/adapters/adapter/v3"
)

func TestUnixAdapter_MakeUnixAdapter(t *testing.T) {
	a := MakeUnixAdapter()
	if a == nil {
		t.Fatal("Expected non-nil adapter")
	}
}

func TestUnixAdapter_JsonRoundTrip(t *testing.T) {
	a := MakeUnixAdapter()
	ua := a.(*unixAdapter)

	t.Run("heartbeat without data", func(t *testing.T) {
		msg := &cluster.ClusterMessage{
			Uid:  "server1",
			Nsp:  "/",
			Type: cluster.HEARTBEAT,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		decoded, err := ua.decode(payload)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Type != cluster.HEARTBEAT {
			t.Fatalf("Expected type HEARTBEAT, got %v", decoded.Type)
		}
		if decoded.Data != nil {
			t.Fatal("Expected nil data for heartbeat")
		}
	})

	t.Run("heartbeat with null data", func(t *testing.T) {
		payload := []byte(`{"uid":"server1","nsp":"/","type":2,"data":null}`)

		decoded, err := ua.decode(payload)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Data != nil {
			t.Fatal("Expected nil data for null data field")
		}
	})

	t.Run("decode invalid JSON", func(t *testing.T) {
		_, err := ua.decode([]byte(`{invalid}`))
		if err == nil {
			t.Fatal("Expected error for invalid JSON")
		}
	})
}
