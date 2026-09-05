package adapter

import (
	"testing"
	"time"
)

func TestDefaultPostgresAdapterOptions(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()
	if opts.GetRawChannelPrefix() != nil ||
		opts.GetRawTableName() != nil ||
		opts.GetRawPayloadThreshold() != nil ||
		opts.GetRawCleanupInterval() != nil ||
		opts.GetRawHeartbeatInterval() != nil ||
		opts.GetRawHeartbeatTimeout() != nil ||
		opts.GetRawErrorHandler() != nil {
		t.Fatal("default options must remain unset until adapter construction")
	}
}

func TestPostgresAdapterOptionsAssign(t *testing.T) {
	handled := false
	source := DefaultPostgresAdapterOptions()
	source.SetChannelPrefix("custom-prefix")
	source.SetTableName("custom_table")
	source.SetPayloadThreshold(4_000)
	source.SetCleanupInterval(60_000)
	source.SetHeartbeatInterval(3 * time.Second)
	source.SetHeartbeatTimeout(15_000)
	source.SetErrorHandler(func(error) { handled = true })

	target := DefaultPostgresAdapterOptions()
	target.Assign(source)
	if target.ChannelPrefix() != "custom-prefix" ||
		target.TableName() != "custom_table" ||
		target.PayloadThreshold() != 4_000 ||
		target.CleanupInterval() != 60_000 ||
		target.HeartbeatInterval() != 3*time.Second ||
		target.HeartbeatTimeout() != 15_000 ||
		target.ErrorHandler() == nil {
		t.Fatalf("assigned options do not match source: %#v", target)
	}
	target.ErrorHandler()(nil)
	if !handled {
		t.Fatal("error handler was not assigned")
	}
}

func TestPostgresAdapterOptionsAssignPreservesUnsetFields(t *testing.T) {
	source := DefaultPostgresAdapterOptions()
	source.SetChannelPrefix("new-prefix")

	target := DefaultPostgresAdapterOptions()
	target.SetTableName("existing_table")
	target.SetCleanupInterval(60_000)
	target.Assign(source)
	if target.ChannelPrefix() != "new-prefix" ||
		target.TableName() != "existing_table" ||
		target.CleanupInterval() != 60_000 {
		t.Fatalf("partial assignment replaced an unset field: %#v", target)
	}
}

func TestPostgresAdapterOptionsAssignTypedNil(t *testing.T) {
	target := DefaultPostgresAdapterOptions()
	target.SetChannelPrefix("existing")
	var source *PostgresAdapterOptions

	if result := target.Assign(source); result != target {
		t.Fatal("Assign must return its receiver")
	}
	if target.ChannelPrefix() != "existing" {
		t.Fatal("typed-nil assignment changed the target")
	}
}
