package socket

import (
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var client_log = log.NewLog("socket.io:client")

// Client represents a Socket.IO client connection.
type Client struct {
	conn engine.Socket

	id             string
	server         *Server
	encoder        parser.Encoder
	decoder        parser.Decoder
	sockets        *types.Map[SocketId, *Socket]
	nsps           *types.Map[string, *Socket]
	connectTimeout atomic.Pointer[utils.Timer]

	mu sync.Mutex
}

// MakeClient creates a new Client instance.
func MakeClient() *Client {
	c := &Client{
		sockets: &types.Map[SocketId, *Socket]{},
		nsps:    &types.Map[string, *Socket]{},
	}

	return c
}

// NewClient creates a new Client and initializes it with the given server and connection.
func NewClient(server *Server, conn engine.Socket) *Client {
	c := MakeClient()

	c.Construct(server, conn)

	return c
}

// Conn returns the underlying Engine.IO socket connection.
func (c *Client) Conn() engine.Socket {
	return c.conn
}

// Construct initializes the client with the given server and connection.
func (c *Client) Construct(server *Server, conn engine.Socket) {
	c.server = server
	c.conn = conn
	c.encoder = server.Encoder()
	c.decoder = server._parser.NewDecoder()
	c.id = conn.Id()
	c.setup()
}

// Request returns the reference to the request that originated the Engine.IO connection.
func (c *Client) Request() *types.HttpContext {
	return c.conn.Request()
}

// setup sets up event listeners for the client.
func (c *Client) setup() {
	c.decoder.On("decoded", c.ondecoded)
	c.conn.On("data", c.ondata)
	c.conn.On("error", c.onerror)
	c.conn.Once("close", c.onclose)

	c.connectTimeout.Store(utils.SetTimeout(func() {
		if c.nsps.Len() == 0 {
			client_log.Debug("no namespace joined yet, close the client")
			c.close()
		} else {
			client_log.Debug("the client has already joined a namespace, nothing to do")
		}
	}, c.server._connectTimeout))
}

// connect connects a client to a namespace with optional auth parameters.
func (c *Client) connect(name string, auth map[string]any) {
	if _, ok := c.server._nsps.Load(name); ok {
		client_log.Debug("connecting to namespace %s", name)
		c.doConnect(name, auth)
		return
	}
	c.server._checkNamespace(name, auth, func(dynamicNspName Namespace) {
		if dynamicNspName != nil {
			c.doConnect(name, auth)
		} else {
			client_log.Debug("creation of namespace %s was denied", name)
			c._packet(&parser.Packet{
				Type: parser.CONNECT_ERROR,
				Nsp:  name,
				Data: map[string]any{
					"message": "Invalid namespace",
				},
			}, nil)
		}
	})
}

// doConnect connects a client to a namespace and adds the socket to the client.
func (c *Client) doConnect(name string, auth map[string]any) {
	nsp := c.server.Of(name, nil)
	nsp.Add(c, auth, func(socket *Socket) {
		c.sockets.Store(socket.Id(), socket)
		c.nsps.Store(nsp.Name(), socket)
		if connectTimeout := c.connectTimeout.Load(); connectTimeout != nil {
			utils.ClearTimeout(connectTimeout)
			c.connectTimeout.Store(nil)
		}
	})
}

// _disconnect disconnects from all namespaces and closes the transport.
func (c *Client) _disconnect() {
	c.sockets.Range(func(id SocketId, socket *Socket) bool {
		socket.Disconnect(false)
		return true
	})
	c.sockets.Clear()
	c.close()
}

// _remove removes a socket from the client. Called by each Socket.
func (c *Client) _remove(socket *Socket) {
	if nsp, ok := c.sockets.Load(socket.Id()); ok {
		c.sockets.Delete(socket.Id())
		c.nsps.Delete(nsp.Nsp().Name())
	} else {
		client_log.Debug("ignoring remove for %s", socket.Id())
	}
}

// close closes the underlying connection.
func (c *Client) close() {
	if c.conn.ReadyState() == "open" {
		client_log.Debug("forcing transport close")
		c.conn.Close(false)
		c.onclose("forced server close")
	}
}

// _packet writes a packet to the transport.
func (c *Client) _packet(packet *parser.Packet, opts *WriteOptions) {
	if c.conn.ReadyState() != "open" {
		client_log.Debug("ignoring packet write %v", packet)
		return
	}

	if opts == nil {
		opts = &WriteOptions{}
	}

	c.WriteToEngine(c.encoder.Encode(packet), opts)
}

// WriteToEngine writes encoded packets to the Engine.IO transport.
func (c *Client) WriteToEngine(encodedPackets []types.BufferInterface, opts *WriteOptions) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if opts.Volatile && !c.conn.Transport().Writable() {
		client_log.Debug("volatile packet is discarded since the transport is not currently writable")
		return
	}

	for _, encodedPacket := range encodedPackets {
		c.conn.Write(encodedPacket.Clone(), &opts.Options, nil)
	}
}

// ondata is called with incoming transport data.
func (c *Client) ondata(args ...any) {
	// error is needed for protocol violations (GH-1880)
	if err := c.decoder.Add(args[0]); err != nil {
		client_log.Debug("invalid packet format")
		c.onerror(err)
	}
}

// ondecoded is called when the parser fully decodes a packet.
func (c *Client) ondecoded(args ...any) {
	packet, _ := args[0].(*parser.Packet)
	var namespace string
	var authPayload map[string]any
	if c.conn.Protocol() == 3 {
		if parsed, err := url.Parse(packet.Nsp); err == nil {
			namespace = parsed.Path
			authPayload = utils.MapValues(parsed.Query(), func(value []string) any {
				return value
			})
		}
	} else {
		namespace = packet.Nsp
		authPayload, _ = packet.Data.(map[string]any)
	}
	socket, ok := c.nsps.Load(namespace)
	if !ok && packet.Type == parser.CONNECT {
		c.connect(namespace, authPayload)
	} else if ok && packet.Type != parser.CONNECT && packet.Type != parser.CONNECT_ERROR {
		// Needs further investigation
		go socket._onpacket(packet)
	} else {
		client_log.Debug("invalid state (packet type: %s)", packet.Type.String())
		c.close()
	}
}

// onerror handles an error from the transport or parser.
func (c *Client) onerror(args ...any) {
	c.sockets.Range(func(_ SocketId, socket *Socket) bool {
		socket._onerror(args[0])
		return true
	})
	c.conn.Close(false)
}

// onclose is called upon transport close.
func (c *Client) onclose(args ...any) {
	client_log.Debug("client close with reason %v", args[0])

	// ignore a potential subsequent `close` event
	c.destroy()

	// `nsps` and `sockets` are cleaned up seamlessly
	c.sockets.Range(func(id SocketId, socket *Socket) bool {
		socket._onclose(args...)
		return true
	})
	c.sockets.Clear()

	c.decoder.Destroy() // clean up decoder
}

// destroy cleans up event listeners and timers for the client.
func (c *Client) destroy() {
	c.conn.RemoveListener("data", c.ondata)
	c.conn.RemoveListener("error", c.onerror)
	c.conn.RemoveListener("close", c.onclose)
	c.decoder.RemoveListener("decoded", c.ondecoded)

	if connectTimeout := c.connectTimeout.Load(); connectTimeout != nil {
		utils.ClearTimeout(connectTimeout)
		c.connectTimeout.Store(nil)
	}
}
