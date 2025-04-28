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
