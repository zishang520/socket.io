package utils

import (
	"testing"
)

func TestMsgPackSingleton(t *testing.T) {
	p1 := MsgPack()
	p2 := MsgPack()
	if p1 != p2 {
		t.Error("MsgPack() should return the same singleton instance")
	}
}

func TestMsgPackEncodeDecodeMap(t *testing.T) {
	pack := MsgPack()
	original := map[string]any{"key": "value", "num": int64(42)}
	data, err := pack.Encode(original)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	var result map[string]any
	if err := pack.Decode(data, &result); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
}

func TestMsgPackEncodeDecodeStruct(t *testing.T) {
	type sample struct {
		Name string `msgpack:"name"`
		Age  int    `msgpack:"age"`
	}
	pack := MsgPack()
	original := sample{Name: "test", Age: 30}
	data, err := pack.Encode(original)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	var result sample
	if err := pack.Decode(data, &result); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if result.Name != "test" || result.Age != 30 {
		t.Errorf("expected {test 30}, got %+v", result)
	}
}

func TestMsgPackDecodeInvalidData(t *testing.T) {
	pack := MsgPack()
	var result string
	err := pack.Decode([]byte{0xFF, 0xFF, 0xFF}, &result)
	if err == nil {
		t.Error("expected error decoding invalid msgpack data")
	}
}

func TestLogSingleton(t *testing.T) {
	logger := Log()
	if logger == nil {
		t.Fatal("Log() returned nil")
	}
}
