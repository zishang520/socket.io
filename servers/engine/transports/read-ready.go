package transports

// StartReader starts read in a goroutine after the initialization permission is
// true; nil permits immediate startup. Call it once, at the end of construction,
// after preparing transport state and close handling. The permission has one
// reader: false or closing the channel without sending true cancels startup.
//
// This only waits for initialization permission, not for the backend to be ready.
// The read function may block waiting for backend readiness or incoming data.
func StartReader(transport Transport, permission <-chan bool, read func()) {
	if permission == nil {
		go read()
		return
	}

	// A custom constructor may reach us after initialization was aborted and
	// the underlying connection's close event has already been emitted.
	select {
	case allowed := <-permission:
		if !allowed {
			transport.Close()
			return
		}
		if state := transport.ReadyState(); state != "closing" && state != "closed" {
			go read()
		}
		return
	default:
	}

	closed := make(chan struct{})
	onClose := func(...any) { close(closed) }
	// Register before returning from Construct, ahead of Engine/user close
	// listeners: a blocking close callback must not keep the reader waiting.
	_ = transport.Once("close", onClose)
	if state := transport.ReadyState(); state == "closing" || state == "closed" {
		transport.RemoveListener("close", onClose)
		return
	}
	go func() {
		var allowed bool
		select {
		case allowed = <-permission:
		case <-closed:
		}
		transport.RemoveListener("close", onClose)
		if !allowed {
			return
		}
		// Both signals may be ready. Never resume a transport already closing.
		if state := transport.ReadyState(); state == "closing" || state == "closed" {
			return
		}
		read()
	}()
}
