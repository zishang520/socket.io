package emitter

import "testing"

func TestDefaultEmitterOptions(t *testing.T) {
	opts := DefaultEmitterOptions()
	if opts.GetRawChannelPrefix() != nil ||
		opts.GetRawTableName() != nil ||
		opts.GetRawPayloadThreshold() != nil {
		t.Fatal("default options must remain unset until emitter construction")
	}
}

func TestEmitterOptionsAssign(t *testing.T) {
	source := DefaultEmitterOptions()
	source.SetChannelPrefix("custom-prefix")
	source.SetTableName("custom_table")
	source.SetPayloadThreshold(4_000)

	target := DefaultEmitterOptions()
	target.Assign(source)
	if target.ChannelPrefix() != "custom-prefix" ||
		target.TableName() != "custom_table" ||
		target.PayloadThreshold() != 4_000 {
		t.Fatalf("assigned options do not match source: %#v", target)
	}
}

func TestEmitterOptionsAssignPreservesUnsetFields(t *testing.T) {
	source := DefaultEmitterOptions()
	source.SetChannelPrefix("new-prefix")

	target := DefaultEmitterOptions()
	target.SetTableName("existing_table")
	target.SetPayloadThreshold(4_000)
	target.Assign(source)
	if target.ChannelPrefix() != "new-prefix" ||
		target.TableName() != "existing_table" ||
		target.PayloadThreshold() != 4_000 {
		t.Fatalf("partial assignment replaced an unset field: %#v", target)
	}
}

func TestEmitterOptionsAssignTypedNil(t *testing.T) {
	target := DefaultEmitterOptions()
	target.SetChannelPrefix("existing")
	var source *EmitterOptions

	if result := target.Assign(source); result != target {
		t.Fatal("Assign must return its receiver")
	}
	if target.ChannelPrefix() != "existing" {
		t.Fatal("typed-nil assignment changed the target")
	}
}
