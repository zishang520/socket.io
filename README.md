# Socket.IO for Golang

[![Go](https://github.com/zishang520/socket.io/actions/workflows/go.yml/badge.svg)](https://github.com/zishang520/socket.io/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/zishang520/socket.io/servers/socket/v3.svg)](https://pkg.go.dev/github.com/zishang520/socket.io/servers/socket/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/zishang520/socket.io/v3)](https://goreportcard.com/report/github.com/zishang520/socket.io/v3)

A modern, idiomatic Go implementation of [Socket.IO](https://socket.io/), designed for real-time, bidirectional communication over WebSockets and other transports.

---

## 🚀 Getting Started

Check out the [official documentation](https://github.com/zishang520/socket.io/tree/v3/docs) to get started, including examples, API references, and guides.

Install a specific module:

```bash
go get github.com/zishang520/socket.io/servers/socket/v3
```

---

## ❓ Questions & Support

The [Issues](https://github.com/zishang520/socket.io/issues) section is only for confirmed bugs or feature requests.

For general help or implementation questions:

- Read the [documentation](https://github.com/zishang520/socket.io/tree/v3/docs)
- Ask in [Discussions → Q&A](https://github.com/zishang520/socket.io/discussions/new?category=q-a)

---

## 🔒 Security

If you discover a vulnerability or security issue, **do not file a public issue**. Instead, please follow the steps in our [Security Policy](./SECURITY.md).

---

## 🛠 Contributing

We welcome contributions of all kinds! To report bugs, suggest features, or submit pull requests:

- Please read our [Contributing Guide](./CONTRIBUTING.md) for best practices
- Ensure your changes are well-tested and formatted with `make fmt`
- Open an issue or discussion before starting major changes

Thanks to all [contributors](https://github.com/zishang520/socket.io/graphs/contributors) who make this project better ❤️

---

## 📦 Modules

This project is a monorepo containing the following Go modules:

| Go Module                                                  | Description                                                                                      |
|------------------------------------------------------------|--------------------------------------------------------------------------------------------------|
| `github.com/zishang520/socket.io/v3`                       | Root module with shared interfaces, types, and base definitions                                 |
| `github.com/zishang520/socket.io/servers/engine/v3`        | Engine.IO server implementation for low-level transport handling                                |
| `github.com/zishang520/socket.io/clients/engine/v3`        | Engine.IO client implementation                                                                 |
| `github.com/zishang520/socket.io/parsers/engine/v3`        | Packet parser for Engine.IO protocol                                                            |
| `github.com/zishang520/socket.io/servers/socket/v3`        | Socket.IO server implementation built atop the Engine.IO server                                 |
| `github.com/zishang520/socket.io/clients/socket/v3`        | Socket.IO client implementation built atop the Engine.IO client                                 |
| `github.com/zishang520/socket.io/parsers/socket/v3`        | Packet parser for Socket.IO protocol                                                            |
| `github.com/zishang520/socket.io/adapters/adapter/v3`      | Base adapter interface for implementing broadcast mechanisms                                    |
| `github.com/zishang520/socket.io/adapters/redis/v3`        | Redis-based adapter for broadcasting messages across distributed servers using Redis Pub/Sub    |

---

## 🧾 License

This project is licensed under the [MIT License](https://opensource.org/licenses/MIT).

