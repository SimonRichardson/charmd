//go:build linux

package daemon

import (
	"context"
	"net"
	"syscall"
)

// PeerContext authenticates Unix-domain connections using kernel peer credentials.
// Connections without available credentials remain unauthenticated.
func PeerContext(ctx context.Context, conn net.Conn) context.Context {
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		return ctx
	}
	raw, err := unix.SyscallConn()
	if err != nil {
		return ctx
	}
	var credentials *syscall.Ucred
	var credentialErr error
	if err = raw.Control(func(fd uintptr) {
		credentials, credentialErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || credentialErr != nil || credentials == nil {
		return ctx
	}
	uid := credentials.Uid
	return WithUser(ctx, &UserState{UID: &uid, Admin: uid == 0})
}
