// Package postgres provides PostgreSQL LISTEN/NOTIFY support for Socket.IO adapters.
package postgres

import "time"

// DefaultOperationTimeout bounds individual PostgreSQL I/O operations.
const DefaultOperationTimeout = 5 * time.Second
