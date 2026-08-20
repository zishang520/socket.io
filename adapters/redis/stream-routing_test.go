package redis

import "testing"

func TestStreamNameForNamespace(t *testing.T) {
	tests := []struct {
		name        string
		streamName  string
		namespace   string
		streamCount int
		want        string
	}{
		{name: "single stream", streamName: "events", namespace: "/chat", streamCount: 1, want: "events"},
		{name: "disabled sharding", streamName: "events", namespace: "/chat", streamCount: 0, want: "events"},
		{name: "positive hash", streamName: "events", namespace: "/chat", streamCount: 5, want: "events-3"},
		{name: "signed hash", streamName: "events", namespace: "/namespace-0", streamCount: 5, want: "events--3"},
		{name: "UTF-16 hash", streamName: "events", namespace: "/" + string(rune(0x1f600)), streamCount: 7, want: "events-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StreamNameForNamespace(tt.streamName, tt.namespace, tt.streamCount); got != tt.want {
				t.Fatalf("StreamNameForNamespace(%q, %q, %d) = %q, want %q", tt.streamName, tt.namespace, tt.streamCount, got, tt.want)
			}
		})
	}
}
