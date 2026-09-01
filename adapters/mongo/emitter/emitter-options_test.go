package emitter

import "testing"

func TestEmitterOptionsAssignIgnoresTypedNil(t *testing.T) {
	target := DefaultEmitterOptions()
	target.SetAddCreatedAtField(true)
	var source *EmitterOptions

	if result := target.Assign(source); result != target {
		t.Fatal("Assign() did not return the target options")
	}
	if !target.AddCreatedAtField() {
		t.Fatal("typed-nil Assign() changed the target options")
	}
}
