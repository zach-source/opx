//go:build darwin

package security

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// InspectPeerProcess extracts the peer PID from a *Unix* domain socket connection,
// verifies its code signature, and returns details about the signed binary.
//
// LIMITATION: macOS exposes peer PID only for Unix domain sockets (not TCP).
func InspectPeerProcess(conn net.Conn) (*PeerInfo, error) {
	// Ensure we’re on a Unix domain socket
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("InspectPeerProcess: connection is not a Unix domain socket (macOS cannot expose peer PID for TCP)")
	}

	// Get the raw file descriptor
	rc, err := uc.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("SyscallConn: %w", err)
	}

	var fd int
	var ctlErr error
	if err := rc.Control(func(x uintptr) {
		fd = int(x)
	}); err != nil {
		return nil, fmt.Errorf("control: %w", err)
	}
	if ctlErr != nil {
		return nil, ctlErr
	}

	// Ask kernel for the peer PID
	pid, err := unix.GetsockoptInt(fd, unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	if err != nil {
		// If the socket is not connected yet or not a Unix socket, this will fail.
		return nil, fmt.Errorf("getsockopt(LOCAL_PEERPID): %w", err)
	}

	// Query macOS Security framework about that PID/code
	info, err := getCodeSignatureInfoForPID(pid)
	if err != nil {
		return nil, fmt.Errorf("codesign query failed for pid %d: %w", pid, err)
	}

	// Fill out the object
	return &PeerInfo{
		PID:             pid,
		ExecutablePath:  info.ExecutablePath,
		SigningID:       info.SigningID,
		TeamID:          info.TeamID,
		CDHashHex:       info.CDHashHex,
		Flags:           info.Flags,
		Signed:          info.Signed,
		ValidSignature:  info.ValidSignature,
		HasEntitlements: info.HasEntitlements,
	}, nil
}

// Optional helper if you want to sanity check the process still exists.
func ProcExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}

// String method removed - using unified String() method in peer.go
