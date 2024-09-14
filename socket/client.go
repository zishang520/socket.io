package socket

import (
	"net/url"
	"sync/atomic"

	_types "github.com/zishang520/engine.io-go-parser/types"
	"github.com/zishang520/engine.io/v2/engine"
	"github.com/zishang520/engine.io/v2/log"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/engine.io/v2/utils"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

var client_log = log.NewLog("socket.io:client")

type Client struct {
	conn engine.Socket

	id             string
	server         *Server
	encoder        parser.Encoder
	decoder        parser.Decoder
	sockets        *types.Map[SocketId, *Socket]
	nsps           *types.Map[string, *Socket]
	connectTimeout atomic.Pointer[utils.Timer]
}

func MakeClient() *Client {
	c := &Client{
		sockets: &types.Map[SocketId, *Socket]{},
		nsps:    &types.Map[string, *Socket]{},
	}

	return c
}

func NewClient(server *Server, conn engine.Socket) *Client {
	c := MakeClient()

	c.Construct(server, conn)

	return c
}

func (c *Client) Conn() engine.Socket {
	return c.conn
}

// Client constructor.
//
// Param: server instance
//
// Param: conn
func (c *Client) Construct(server *Server, conn engine.Socket) {
	c.server = server
	c.conn = conn
	c.encoder = server.Encoder()
	c.decoder = server._parser.NewDecoder()
	c.id = conn.Id()
	c.setup()
}

// Return: the reference to the request that originated the Engine.IO connection
func (c *Client) Request() *types.HttpContext {
	return c.conn.Request()
}

// Sets up event listeners.
func (c *Client) setup() {
	c.decoder.On("decoded", c.ondecoded)
	c.conn.On("data", c.ondata)
	c.conn.On("error", c.onerror)
	c.conn.On("close", c.onclose)

	c.connectTimeout.Store(utils.SetTimeout(func() {
		if c.nsps.Len() == 0 {
			client_log.Debug("no namespace joined yet, close the client")
			c.close()
		} else {
			client_log.Debug("the client has already joined a namespace, nothing to do")
		}
	}, c.server._connectTimeout))
}

// Connects a client to a namespace.
//
// Param: name - the namespace
//
// Param: auth - the auth parameters
func (c *Client) connect(name string, auth any) {
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

// Connects a client to a namespace.
//
// Param: name - the namespace
//
// Param: auth - the auth parameters
func (c *Client) doConnect(name string, auth any) {
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

// Disconnects from all namespaces and closes transport.
func (c *Client) _disconnect() {
	c.sockets.Range(func(id SocketId, socket *Socket) bool {
		socket.Disconnect(false)
		return true
	})
	c.sockets.Clear()
	c.close()
}

// Removes a socket. Called by each `Socket`.
func (c *Client) _remove(socket *Socket) {
	if nsp, ok := c.sockets.Load(socket.Id()); ok {
		c.sockets.Delete(socket.Id())
		c.nsps.Delete(nsp.Nsp().Name())
	} else {
		client_log.Debug("ignoring remove for %s", socket.Id())
	}
}

// Closes the underlying connection.
func (c *Client) close() {
	if "open" == c.conn.ReadyState() {
		client_log.Debug("forcing transport close")
		c.conn.Close(false)
		c.onclose("forced server close")
	}
}

// Writes a packet to the transport.
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

func (c *Client) WriteToEngine(encodedPackets []_types.BufferInterface, opts *WriteOptions) {
	if opts.Volatile && !c.conn.Transport().Writable() {
		client_log.Debug("volatile packet is discarded since the transport is not currently writable")
		return
	}

	for _, encodedPacket := range encodedPackets {
		c.conn.Write(encodedPacket.Clone(), &opts.Options, nil)
	}
}

// Called with incoming transport data.
func (c *Client) ondata(args ...any) {
	// error is needed for protocol violations (GH-1880)
	if err := c.decoder.Add(args[0]); err != nil {
		client_log.Debug("invalid packet format")
		c.onerror(err)
	}
}

// Called when parser fully decodes a packet.
func (c *Client) ondecoded(args ...any) {
	packet, _ := args[0].(*parser.Packet)
	var namespace string
	var authPayload any
	if c.conn.Protocol() == 3 {
		if parsed, err := url.Parse(packet.Nsp); err == nil {
			namespace = parsed.Path
			authPayload = parsed.Query()
		}
	} else {
		namespace = packet.Nsp
		authPayload = packet.Data
	}
	socket, ok := c.nsps.Load(namespace)
	if !ok && packet.Type == parser.CONNECT {
		c.connect(namespace, authPayload)
	} else if ok && packet.Type != parser.CONNECT && packet.Type != parser.CONNECT_ERROR {
		go socket._onpacket(packet)
	} else {
		client_log.Debug("invalid state (packet type: %s)", packet.Type.String())
		c.close()
	}
}

// Handles an error.
func (c *Client) onerror(args ...any) {
	c.sockets.Range(func(_ SocketId, socket *Socket) bool {
		socket._onerror(args[0])
		return true
	})
	c.conn.Close(false)
}

// Called upon transport close.
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

// Cleans up event listeners.
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
