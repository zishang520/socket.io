package emitter

import (
	"testing"
)

func TestDefaultEmitterOptions(t *testing.T) {
	opts := DefaultEmitterOptions()
	opts.Assign(nil)

	t.Run("Key", func(t *testing.T) {
		if opts.GetRawKey() != nil {
			t.Fatal(`DefaultEmitterOptions.GetRawKey() value must be nil`)
		}
		if opts.Key() != "" {
			t.Fatal(`DefaultEmitterOptions.Key() value must be ""`)
		}
		opts.SetKey("test")
		if opts.Key() != "test" {
			t.Fatal(`DefaultEmitterOptions.Key() value must be "test"`)
		}
	})

	t.Run("Parser", func(t *testing.T) {
		if opts.GetRawParser() != nil {
			t.Fatal(`DefaultEmitterOptions.GetRawParser() value must be nil`)
		}
	})

	t.Run("TableName", func(t *testing.T) {
		if opts.GetRawTableName() != nil {
			t.Fatal(`DefaultEmitterOptions.GetRawTableName() value must be nil`)
		}
		if opts.TableName() != "" {
			t.Fatal(`DefaultEmitterOptions.TableName() value must be ""`)
		}
		opts.SetTableName("my_table")
		if opts.TableName() != "my_table" {
			t.Fatal(`DefaultEmitterOptions.TableName() value must be "my_table"`)
		}
	})

	t.Run("PayloadThreshold", func(t *testing.T) {
		if opts.GetRawPayloadThreshold() != nil {
			t.Fatal(`DefaultEmitterOptions.GetRawPayloadThreshold() value must be nil`)
		}
		if opts.PayloadThreshold() != 0 {
			t.Fatal(`DefaultEmitterOptions.PayloadThreshold() value must be 0`)
		}
		opts.SetPayloadThreshold(4000)
		if opts.PayloadThreshold() != 4000 {
			t.Fatal(`DefaultEmitterOptions.PayloadThreshold() value must be 4000`)
		}
	})
}

func TestEmitterOptions_Assign(t *testing.T) {
	t.Run("assign nil", func(t *testing.T) {
		opts := DefaultEmitterOptions()
		result := opts.Assign(nil)
		if result != opts {
			t.Fatal("Expected same instance when assigning nil")
		}
	})

	t.Run("assign all fields", func(t *testing.T) {
		source := DefaultEmitterOptions()
		source.SetKey("custom-key")
		source.SetTableName("custom_table")
		source.SetPayloadThreshold(4000)

		target := DefaultEmitterOptions()
		target.Assign(source)

		if target.Key() != "custom-key" {
			t.Fatalf("Expected 'custom-key', got %s", target.Key())
		}
		if target.TableName() != "custom_table" {
			t.Fatalf("Expected 'custom_table', got %s", target.TableName())
		}
		if target.PayloadThreshold() != 4000 {
			t.Fatalf("Expected 4000, got %d", target.PayloadThreshold())
		}
	})

	t.Run("partial assign preserves existing", func(t *testing.T) {
		source := DefaultEmitterOptions()
		source.SetKey("new-key")

		target := DefaultEmitterOptions()
		target.SetTableName("existing_table")
		target.Assign(source)

		if target.Key() != "new-key" {
			t.Fatalf("Expected 'new-key', got %s", target.Key())
		}
		if target.TableName() != "existing_table" {
			t.Fatalf("Expected 'existing_table' to be preserved, got %s", target.TableName())
		}
	})
}
