package socket_test

import (
	"fmt"
	"log"
	"time"

	client "github.com/zishang520/socket.io/clients/socket/v3"
	server "github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ExampleSocket_basic demonstrates the basic usage of Socket.IO client
func ExampleSocket_basic() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)
	server.NewServer(httpServer, config)

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.On("connect", func(...any) {
			socket.Emit("message", "Hello server!")
			fmt.Println("Connected!")
			defer socket.Close()
			close(done)
		})

		socket.On("reply", func(args ...any) {
			if len(args) > 0 {
				if msg, ok := args[0].(string); ok {
					fmt.Printf("Received: %s\n", msg)
				}
			}
		})
	})

	<-done
	httpServer.Close(nil)

	// Output:
	// Connected!
}

// ExampleSocket_disconnect demonstrates how to disconnect the socket
func ExampleSocket_disconnect() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)
	server.NewServer(httpServer, config)

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.On("connect", func(...any) {
			fmt.Println("Connected!")
			socket.Disconnect()
		})

		socket.On("disconnect", func(args ...any) {
			if len(args) > 0 {
				if reason, ok := args[0].(string); ok {
					fmt.Printf("Disconnected: %s\n", reason)
					defer socket.Close()
					close(done)
				}
			}
		})

	})

	<-done
	httpServer.Close(nil)

	// Output:
	// Connected!
	// Disconnected: io client disconnect
}

// ExampleSocket_emitWithAck demonstrates how to emit events with acknowledgement
func ExampleSocket_emitWithAck() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)
	server.NewServer(httpServer, config).On("connection", func(clients ...any) {
		socket := clients[0].(*server.Socket)
		socket.On("custom-event", func(args ...any) {
			ack := args[len(args)-1].(server.Ack)
			ack(args[:len(args)-1], nil)
		})
	})

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.EmitWithAck("custom-event", "received hello")(func(args []any, err error) {
			if err != nil {
				fmt.Println("Failed to receive ack")
			} else {
				fmt.Printf("Server acknowledged with: %v\n", args)
			}
			defer socket.Close()
			close(done)
		})
	})

	<-done
	httpServer.Close(nil)

	// Output:
	// Server acknowledged with: [received hello]
}

// ExampleSocket_volatile demonstrates how to send messages that may be lost
func ExampleSocket_volatile() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)
	server.NewServer(httpServer, config)

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling))
		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.On("connect", func(...any) {
			// The server may or may not receive this message
			socket.Volatile().Emit("hello", "world")
			defer socket.Close()
			close(done)
		})
	})

	<-done
	httpServer.Close(nil)
}

// ExampleSocket_onAny demonstrates how to listen to all events
func ExampleSocket_onAny() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)
	server.NewServer(httpServer, config).On("connection", func(clients ...any) {
		socket := clients[0].(*server.Socket)
		socket.On("test-event", func(args ...any) {
			socket.Emit("test-event")
		})
	})

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.OnAny(func(args ...any) {
			fmt.Printf("Caught event: %v\n", args[0])
			defer socket.Close()
			close(done)
		})

		socket.Emit("test-event", "data")
	})

	<-done
	httpServer.Close(nil)

	// Output:
	// Caught event: test-event
}

// ExampleSocket_timeout demonstrates how to set timeout for acknowledgements
func ExampleSocket_timeout() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)
	server.NewServer(httpServer, config)

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.Timeout(5*time.Second).EmitWithAck("delayed-event", "data")(func(args []any, err error) {
			if err != nil {
				fmt.Println("Event timed out")
			} else {
				fmt.Printf("Received response: %v\n", args)
			}
			defer socket.Close()
			close(done)
		})
	})

	<-done
	httpServer.Close(nil)

	// Output:
	// Event timed out
}

// ExampleSocket_auth demonstrates how to use authentication with Socket.IO
func ExampleSocket_auth() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)

	// Create server with authentication middleware
	io := server.NewServer(httpServer, config)

	// Add authentication middleware
	io.Use(func(socket *server.Socket, next func(*server.ExtendedError)) {
		// Get auth data from handshake
		auth := socket.Handshake().Auth

		// Check if token exists and is valid
		if token, ok := auth["Token"]; ok {
			if tokenStr, ok := token.(string); ok && tokenStr == "test" {
				fmt.Printf("Authentication successful for token: %s\n", tokenStr)
				next(nil) // Authentication passed
				return
			}
		}

		// Authentication failed
		fmt.Println("Authentication failed - invalid or missing token")
		next(server.NewExtendedError("Authentication failed", map[string]any{
			"type": "authentication_error",
		}))
	})

	// Handle successful connections
	io.On("connection", func(clients ...any) {
		socket := clients[0].(*server.Socket)
		fmt.Printf("Client connected\n")

		socket.On("secure-message", func(args ...any) {
			if len(args) > 0 {
				if msg, ok := args[0].(string); ok {
					fmt.Printf("Received secure message: %s\n", msg)
					socket.Emit("secure-reply", "Message received securely")
				}
			}
		})

		socket.On("disconnect", func(args ...any) {
			fmt.Printf("Client disconnected\n")
		})
	})

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		// Client connection with authentication
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		opts.SetAuth(map[string]any{"Token": "test"})

		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.On("connect", func(...any) {
			fmt.Println("Client connected successfully!")
			socket.Emit("secure-message", "Hello from authenticated client!")
		})

		socket.On("secure-reply", func(args ...any) {
			if len(args) > 0 {
				if msg, ok := args[0].(string); ok {
					fmt.Printf("Server reply: %s\n", msg)
				}
			}
			defer socket.Close()
			close(done)
		})

		socket.On("connect_error", func(args ...any) {
			if len(args) > 0 {
				fmt.Printf("Connection error: %v\n", args[0])
			}
			defer socket.Close()
			close(done)
		})
	})

	<-done
	httpServer.Close(nil)

	// Output:
	// Authentication successful for token: test
	// Client connected
	// Client connected successfully!
	// Received secure message: Hello from authenticated client!
	// Server reply: Message received securely
	// Client disconnected
}

// ExampleSocket_authFailed demonstrates authentication failure
func ExampleSocket_authFailed() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)

	// Create server with authentication middleware
	io := server.NewServer(httpServer, config)

	// Add authentication middleware
	io.Use(func(socket *server.Socket, next func(*server.ExtendedError)) {
		// Get auth data from handshake
		auth := socket.Handshake().Auth

		// Check if token exists and is valid
		if token, ok := auth["Token"]; ok {
			if tokenStr, ok := token.(string); ok && tokenStr == "valid-token" {
				fmt.Printf("Authentication successful for token: %s\n", tokenStr)
				next(nil) // Authentication passed
				return
			}
		}

		// Authentication failed
		fmt.Println("Authentication failed - invalid or missing token")
		next(server.NewExtendedError("Authentication failed", map[string]any{
			"type": "authentication_error",
		}))
	})

	// Handle successful connections (won't be reached in this example)
	io.On("connection", func(clients ...any) {
		socket := clients[0].(*server.Socket)
		fmt.Printf("Client connected with ID: %s\n", socket.Id())
	})

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		// Client connection with invalid authentication
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		opts.SetAuth(map[string]any{"Token": "invalid-token"}) // Wrong token

		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.On("connect", func(...any) {
			fmt.Println("This shouldn't be called due to auth failure")
		})

		socket.On("connect_error", func(args ...any) {
			fmt.Printf("Authentication failed as expected: %v\n", args[0])
			defer socket.Close()
			close(done)
		})
	})

	<-done
	httpServer.Close(nil)

	// Output:
	// Authentication failed - invalid or missing token
	// Authentication failed as expected: Authentication failed
}

// ExampleSocket_authWithUserInfo demonstrates authentication with user information
func ExampleSocket_authWithUserInfo() {
	config := server.DefaultServerOptions()
	config.SetTransports(types.NewSet(server.Polling, server.WebSocket, server.WebTransport))

	httpServer := types.NewWebServer(nil)

	// Create server with authentication middleware
	io := server.NewServer(httpServer, config)

	// Add authentication middleware with user info extraction
	io.Use(func(socket *server.Socket, next func(*server.ExtendedError)) {
		auth := socket.Handshake().Auth

		// Validate token and extract user info
		if token, ok := auth["Token"]; ok {
			if tokenStr, ok := token.(string); ok && tokenStr == "user123" {
				// Store user info in socket data
				socket.SetData(map[string]string{
					"userId":   "123",
					"username": "testuser",
				})

				fmt.Printf("User authenticated: %s (ID: %s)\n", "testuser", "123")
				next(nil)
				return
			}
		}

		fmt.Println("Authentication failed")
		next(server.NewExtendedError("Invalid credentials", nil))
	})

	// Handle successful connections
	io.On("connection", func(clients ...any) {
		socket := clients[0].(*server.Socket)

		// Access user data
		userId, _ := socket.Data().(map[string]string)["userId"]
		username, _ := socket.Data().(map[string]string)["username"]

		fmt.Printf("Welcome %s! (User ID: %s)\n",
			username, userId)

		socket.On("user-action", func(args ...any) {
			if len(args) > 0 {
				if action, ok := args[0].(string); ok {
					fmt.Printf("User %s performed action: %s\n", username, action)
				}
			}
			socket.Emit("logout")
		})

		socket.Emit("ready")
	})

	done := make(chan struct{})

	httpServer.Listen("127.0.0.1:8000", func() {
		// Client connection with user token
		opts := client.DefaultOptions()
		opts.SetTransports(types.NewSet(client.Polling, client.WebSocket))
		opts.SetAuth(map[string]any{
			"Token": "user123",
		})

		socket, err := client.Connect("http://127.0.0.1:8000/", opts)
		if err != nil {
			log.Fatal(err)
		}

		socket.On("ready", func(...any) {
			socket.Emit("user-action", "login")
		})

		socket.On("logout", func(...any) {
			defer socket.Close()
			close(done)
		})

		socket.On("connect", func(...any) {
			fmt.Println("Connected with user authentication!")
		})

		socket.On("connect_error", func(args ...any) {
			fmt.Printf("Connection error: %v\n", args[0])
			defer socket.Close()
			close(done)
		})
	})

	<-done
	httpServer.Close(nil)

	// Output:
	// User authenticated: testuser (ID: 123)
	// Welcome testuser! (User ID: 123)
	// Connected with user authentication!
	// User testuser performed action: login
}
