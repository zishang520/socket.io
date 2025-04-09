package socket

import (
	"testing"
	"time"
	"unsafe"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

// TestConnectionStateRecoverySkipMiddlewares tests all cases for SkipMiddlewares method
func TestConnectionStateRecoverySkipMiddlewares(t *testing.T) {
	t.Run("when skipMiddlewares is nil", func(t *testing.T) {
		recovery := &ConnectionStateRecovery{
			skipMiddlewares: nil,
		}

		result := recovery.SkipMiddlewares()

		if result != false {
			t.Errorf("Expected SkipMiddlewares() to return false when skipMiddlewares is nil, got %v", result)
		}
	})

	t.Run("when skipMiddlewares is false", func(t *testing.T) {
		skipMiddlewares := false
		recovery := &ConnectionStateRecovery{
			skipMiddlewares: &skipMiddlewares,
		}

		result := recovery.SkipMiddlewares()

		if result != false {
			t.Errorf("Expected SkipMiddlewares() to return false when skipMiddlewares is false, got %v", result)
		}
	})

	t.Run("when skipMiddlewares is true", func(t *testing.T) {
		skipMiddlewares := true
		recovery := &ConnectionStateRecovery{
			skipMiddlewares: &skipMiddlewares,
		}

		result := recovery.SkipMiddlewares()

		if result != true {
			t.Errorf("Expected SkipMiddlewares() to return true when skipMiddlewares is true, got %v", result)
		}
	})
}

func TestSetAndGetServerOptions(t *testing.T) {
	config := DefaultServerOptions()
	config.SetPingTimeout(10000 * time.Millisecond)

	opts := DefaultServerOptions().Assign(config)

	rawValue := opts.GetRawPingTimeout()
	if rawValue == nil {
		t.Error("Expected GetRawPingTimeout to return non-nil value")
	}
	if opts.PingTimeout() != 10000*time.Millisecond {
		t.Errorf("Expected Path to return %v, got %v", 10000*time.Millisecond, opts.PingTimeout())
	}
}

func TestSetAndGetAttachOptions(t *testing.T) {
	config := DefaultServerOptions()
	config.SetPath("/test")

	opts := DefaultServerOptions().Assign(config)

	rawValue := opts.GetRawPath()
	if rawValue == nil {
		t.Error("Expected GetRawPath to return non-nil value")
	}
	if opts.Path() != "/test" {
		t.Errorf("Expected Path to return %v, got %v", "/test", opts.Path())
	}
}

// TestSetAndGetServeClientTrue tests the ServeClient functionality
func TestSetAndGetServeClientTrue(t *testing.T) {
	opts := DefaultServerOptions()
	opts.SetServeClient(true)

	rawValue := opts.GetRawServeClient()
	if rawValue == nil {
		t.Error("Expected GetRawServeClient to return non-nil value")
	}
	if opts.ServeClient() != true {
		t.Errorf("Expected ServeClient to return true, got %v", *rawValue)
	}

	if !opts.ServeClient() {
		t.Error("Expected ServeClient() to return true")
	}
}

// TestSetAndGetAdapter tests the Adapter functionality
func TestSetAndGetAdapter(t *testing.T) {
	mockAdapterConstructor := &SessionAwareAdapterBuilder{}

	serverOpts := DefaultServerOptions()
	serverOpts.SetAdapter(mockAdapterConstructor)

	gotRawAdapter := serverOpts.GetRawAdapter()

	if gotRawAdapter == nil {
		t.Error("Expected adapter constructor to not be nil")
	}

	if *(*uintptr)(unsafe.Pointer(mockAdapterConstructor)) != *(*uintptr)(unsafe.Pointer(gotRawAdapter.(*SessionAwareAdapterBuilder))) {
		t.Error("Expected to get the same adapter constructor that was set")
	}

	adapterFunc := serverOpts.Adapter()
	if adapterFunc == nil {
		t.Error("Expected Adapter() to return a non-nil function")
	}

	if *(*uintptr)(unsafe.Pointer(adapterFunc.(*SessionAwareAdapterBuilder))) != *(*uintptr)(unsafe.Pointer(gotRawAdapter.(*SessionAwareAdapterBuilder))) {
		t.Error("Expected Adapter() to return the same constructor that was set")
	}
}

// TestSetAndGetParser tests the Parser functionality
func TestSetAndGetParser(t *testing.T) {
	opts := DefaultServerOptions()
	customParser := parser.NewParser()

	opts.SetParser(customParser)

	gotRawParser := opts.GetRawParser()
	if gotRawParser != customParser {
		t.Errorf("GetRawParser() = %v, want %v", gotRawParser, customParser)
	}

	gotParser := opts.Parser()
	if gotParser != customParser {
		t.Errorf("Parser() = %v, want %v", gotParser, customParser)
	}
}

// TestSetAndGetConnectTimeout tests the ConnectTimeout functionality
func TestSetAndGetConnectTimeout(t *testing.T) {
	opts := DefaultServerOptions()
	testTimeout := 5000 * time.Millisecond

	opts.SetConnectTimeout(testTimeout)

	rawTimeout := opts.GetRawConnectTimeout()
	if rawTimeout == nil {
		t.Error("Expected GetRawConnectTimeout to return non-nil value")
		return
	}

	if *rawTimeout != testTimeout {
		t.Errorf("GetRawConnectTimeout() = %v, want %v", *rawTimeout, testTimeout)
	}

	gotTimeout := opts.ConnectTimeout()
	if gotTimeout != testTimeout {
		t.Errorf("ConnectTimeout() = %v, want %v", gotTimeout, testTimeout)
	}

	// Test default value
	defaultOpts := DefaultServerOptions()
	if defaultOpts.ConnectTimeout() != 0 {
		t.Errorf("Default ConnectTimeout() = %v, want 0", defaultOpts.ConnectTimeout())
	}
}

// TestSetAndGetCleanupEmptyChildNamespaces tests the CleanupEmptyChildNamespaces functionality
func TestSetAndGetCleanupEmptyChildNamespaces(t *testing.T) {
	opts := DefaultServerOptions()

	// Test default value
	if opts.CleanupEmptyChildNamespaces() != false {
		t.Error("Expected default CleanupEmptyChildNamespaces to be false")
	}

	// Test setting to true
	opts.SetCleanupEmptyChildNamespaces(true)

	rawValue := opts.GetRawCleanupEmptyChildNamespaces()
	if rawValue == nil {
		t.Error("Expected GetRawCleanupEmptyChildNamespaces to return non-nil value")
		return
	}
	if *rawValue != true {
		t.Error("Expected GetRawCleanupEmptyChildNamespaces to return true")
	}

	if !opts.CleanupEmptyChildNamespaces() {
		t.Error("Expected CleanupEmptyChildNamespaces() to return true")
	}

	// Test setting to false
	opts.SetCleanupEmptyChildNamespaces(false)
	if opts.CleanupEmptyChildNamespaces() {
		t.Error("Expected CleanupEmptyChildNamespaces() to return false")
	}
}

// TestConnectionStateRecoveryOperations tests the ConnectionStateRecovery functionality
func TestConnectionStateRecoveryOperations(t *testing.T) {
	opts := DefaultServerOptions()
	recovery := &ConnectionStateRecovery{}

	// Test setting and getting ConnectionStateRecovery
	opts.SetConnectionStateRecovery(recovery)

	rawRecovery := opts.GetRawConnectionStateRecovery()
	if rawRecovery != recovery {
		t.Error("Expected GetRawConnectionStateRecovery to return the same instance")
	}

	gotRecovery := opts.ConnectionStateRecovery()
	if gotRecovery != recovery {
		t.Error("Expected ConnectionStateRecovery to return the same instance")
	}

	if gotRecovery.GetRawSkipMiddlewares() != nil {
		t.Error("Expected default ConnectionStateRecovery.GetRawSkipMiddlewares to be nil")
	}

	if gotRecovery.SkipMiddlewares() {
		t.Error("Expected default ConnectionStateRecovery.SkipMiddlewares to be false")
	}

	gotRecovery.SetSkipMiddlewares(true)
	if !gotRecovery.SkipMiddlewares() {
		t.Error("Expected default ConnectionStateRecovery.SkipMiddlewares to be true")
	}

	if !opts.ConnectionStateRecovery().SkipMiddlewares() {
		t.Error("Expected default ConnectionStateRecovery.SkipMiddlewares to be true")
	}

	// Test default value
	defaultOpts := DefaultServerOptions()
	if defaultOpts.ConnectionStateRecovery() != nil {
		t.Error("Expected default ConnectionStateRecovery to be nil")
	}
}

// TestMaxDisconnectionDurationOperations tests the MaxDisconnectionDuration functionality
func TestMaxDisconnectionDurationOperations(t *testing.T) {
	recovery := &ConnectionStateRecovery{}

	testDuration := int64(30000)
	recovery.SetMaxDisconnectionDuration(testDuration)

	rawDuration := recovery.GetRawMaxDisconnectionDuration()
	if rawDuration == nil {
		t.Error("Expected GetRawMaxDisconnectionDuration to return non-nil value")
		return
	}
	if *rawDuration != testDuration {
		t.Errorf("GetRawMaxDisconnectionDuration() = %v, want %v", *rawDuration, testDuration)
	}

	if recovery.MaxDisconnectionDuration() != testDuration {
		t.Errorf("MaxDisconnectionDuration() = %v, want %v", recovery.MaxDisconnectionDuration(), testDuration)
	}

	recovery.SetMaxDisconnectionDuration(0)
	if recovery.MaxDisconnectionDuration() != 0 {
		t.Errorf("MaxDisconnectionDuration() after setting 0 = %v, want 0", recovery.MaxDisconnectionDuration())
	}

	negativeDuration := int64(-5000)
	recovery.SetMaxDisconnectionDuration(negativeDuration)
	if recovery.MaxDisconnectionDuration() != negativeDuration {
		t.Errorf("MaxDisconnectionDuration() with negative input = %v, want %v",
			recovery.MaxDisconnectionDuration(), negativeDuration)
	}
}

// TestConnectionStateRecoverySkipMiddlewaresOperations tests the SkipMiddlewares functionality
func TestConnectionStateRecoverySkipMiddlewaresOperations(t *testing.T) {
	recovery := &ConnectionStateRecovery{}

	// Test default value
	if recovery.SkipMiddlewares() != false {
		t.Error("Expected default SkipMiddlewares to be false")
	}

	// Test setting to true
	recovery.SetSkipMiddlewares(true)

	rawValue := recovery.GetRawSkipMiddlewares()
	if rawValue == nil {
		t.Error("Expected GetRawSkipMiddlewares to return non-nil value")
		return
	}
	if *rawValue != true {
		t.Error("Expected GetRawSkipMiddlewares to return true")
	}

	if !recovery.SkipMiddlewares() {
		t.Error("Expected SkipMiddlewares() to return true")
	}

	// Test setting to false
	recovery.SetSkipMiddlewares(false)
	if recovery.SkipMiddlewares() {
		t.Error("Expected SkipMiddlewares() to return false")
	}
}
