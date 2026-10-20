//go:build !linux

package daemon

import (
	"context"
	"net"
)

// PeerContext leaves connections unauthenticated when peer credentials are unavailable.
func PeerContext(ctx context.Context, _ net.Conn) context.Context { return ctx }
