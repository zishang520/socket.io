package adapter

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoAdapterOptionsAssign(t *testing.T) {
	changeStreamOptions := options.ChangeStream().SetMaxAwaitTime(time.Second)
	source := DefaultMongoAdapterOptions()
	source.SetUid("node-1")
	source.SetRequestsTimeout(2 * time.Second)
	source.SetAddCreatedAtField(true)
	source.SetChangeStreamOptions(changeStreamOptions)

	target := DefaultMongoAdapterOptions()
	target.Assign(source)

	if target.Uid() != "node-1" {
		t.Fatalf("unexpected uid: %q", target.Uid())
	}
	if target.RequestsTimeout() != 2*time.Second {
		t.Fatalf("unexpected requests timeout: %s", target.RequestsTimeout())
	}
	if !target.AddCreatedAtField() {
		t.Fatal("addCreatedAtField was not copied")
	}
	if target.ChangeStreamOptions() != changeStreamOptions {
		t.Fatal("changeStreamOptions was not copied")
	}
}
