# Socket.IO Contributing Guide (Go Version)

Thank you for your interest in contributing to the **Go implementation of Socket.IO** at [`github.com/zishang520/socket.io`](https://github.com/zishang520/socket.io)!

To ensure a smooth collaboration process, please read the following guidelines before you get started.

<!-- TOC -->
  * [Before You Start](#before-you-start)
  * [Reporting Bugs](#reporting-bugs)
  * [Requesting Features](#requesting-features)
  * [Creating Pull Requests](#creating-pull-requests)
    * [Bug Fixes](#bug-fixes)
    * [New Features](#new-features)
  * [Project Structure](#project-structure)
  * [Development Setup](#development-setup)
  * [Useful Commands](#useful-commands)
    * [Code Formatting](#code-formatting)
    * [Running Tests](#running-tests)
<!-- TOC -->

## Before You Start

- Our [issues list](https://github.com/zishang520/socket.io/issues) is reserved for **bug reports and feature requests**.
- For general usage questions, please refer to:
  - the [documentation](https://github.com/zishang520/socket.io/tree/v3/docs)
  - or open a [discussion](https://github.com/zishang520/socket.io/discussions/new?category=q-a)

## Reporting Bugs

- First, check if your issue already exists in the [bug label](https://github.com/zishang520/socket.io/issues?q=label%3Abug+).
- If it has been reported but closed and still persists, open a **new issue** instead of commenting on the old one.
- For security-related bugs, **do not** open a public issue. Refer to our [security policy](./SECURITY.md).

### When creating a bug report:
- Include Go module versions
- Specify the platform (OS, architecture)
- Provide a minimal reproducible example (see [examples/docs](https://github.com/zishang520/socket.io/tree/v3/docs))

## Requesting Features

- Check for similar feature requests in the [enhancement list](https://github.com/zishang520/socket.io/labels/enhancement).
- If none exist, [submit a new feature request](https://github.com/zishang520/socket.io/issues/new/choose).

Include:
- The problem you're trying to solve
- Your proposed solution
- Alternatives or workarounds considered

## Creating Pull Requests

We welcome PRs for bug fixes, new features, and improvements.

### Bug Fixes
- Reference the related issue (if any)
- Add test cases to prevent future regressions
- Ensure all existing tests pass

### New Features
- Please open a [feature request](#requesting-features) first to discuss it
- Include tests and documentation
- Make sure all tests pass before submitting

## Project Structure

This is a **Go monorepo**. Each submodule is organized as an independent Go module:

| Go Module                                                  | Description                                                                                                                           |
|------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| `github.com/zishang520/socket.io/v3`                       | Root module defining shared types, interfaces, and entry points.                                                                     |
| `github.com/zishang520/socket.io/servers/engine/v3`        | Engine.IO server: manages low-level communication via transports.                                                                    |
| `github.com/zishang520/socket.io/clients/engine/v3`        | Engine.IO client: low-level transport connection to server.                                                                          |
| `github.com/zishang520/socket.io/parsers/engine/v3`        | Engine.IO packet parser: encodes/decodes transport-level messages.                                                                  |
| `github.com/zishang520/socket.io/servers/socket/v3`        | Socket.IO server: built on Engine.IO server for real-time, bidirectional communication.                                              |
| `github.com/zishang520/socket.io/clients/socket/v3`        | Socket.IO client: built on Engine.IO client with rooms, namespaces, etc.                                                             |
| `github.com/zishang520/socket.io/parsers/socket/v3`        | Socket.IO packet parser: handles event-based message encoding/decoding.                                                              |
| `github.com/zishang520/socket.io/adapters/adapter/v3`      | Adapter interface: plug-and-play broadcast layer for multi-node communication.                                                       |
| `github.com/zishang520/socket.io/adapters/redis/v3`        | Redis-based adapter: enables pub/sub message broadcasting via Redis.                                                                 |
| `github.com/zishang520/socket.io/adapters/pgsql/v3`        | PostgreSQL-based adapter (not yet implemented).                                                                                       |

## Development Setup

- Install [Go](https://go.dev) **version 1.24.1+**
- Clone the repository and use Go Workspaces (`go work`) to link submodules if needed

## Useful Commands

### Code Formatting

Format all workspaces:

```bash
make fmt
```

Format a specific workspace:

**Windows:**
```bash
make fmt servers/socket
```

**Unix:**
```bash
make fmt MODULE=servers/socket
```

### Running Tests

Run all tests:

```bash
make test
```

Test a specific workspace:

**Windows:**
```bash
make test servers/socket
```

**Unix:**
```bash
make test MODULE=servers/socket
```

---

Feel free to open an issue or discussion if you have any questions. Happy hacking! ✨

