//go:build darwin

package security

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

	// Get parent PID and build hierarchy
	parentPID := getParentPIDMacOS(pid)
	hierarchy := buildProcessHierarchyMacOS(pid)

	// Fill out the object
	return &PeerInfo{
		PID:              pid,
		ExecutablePath:   info.ExecutablePath,
		ParentPID:        parentPID,
		ProcessHierarchy: hierarchy,
		SigningID:        info.SigningID,
		TeamID:           info.TeamID,
		CDHashHex:        info.CDHashHex,
		Flags:            info.Flags,
		Signed:           info.Signed,
		ValidSignature:   info.ValidSignature,
		HasEntitlements:  info.HasEntitlements,
	}, nil
}

// Optional helper if you want to sanity check the process still exists.
func ProcExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}

// getParentPIDMacOS gets the parent PID for a process on macOS
func getParentPIDMacOS(pid int) int {
	cmd := exec.Command("/bin/ps", "-o", "ppid=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	ppidStr := strings.TrimSpace(string(output))
	ppid, err := strconv.Atoi(ppidStr)
	if err != nil {
		return 0
	}

	return ppid
}

// buildProcessHierarchyMacOS builds the parent process hierarchy on macOS
func buildProcessHierarchyMacOS(startPID int) []ProcessInfo {
	var hierarchy []ProcessInfo
	currentPID := startPID
	visited := make(map[int]bool)

	// Walk up the parent chain
	for depth := 0; depth < 10 && currentPID > 1; depth++ { // Limit depth
		if visited[currentPID] {
			break // Circular reference protection
		}
		visited[currentPID] = true

		// Get parent PID using ps command
		parentPID := getParentPIDMacOS(currentPID)
		if parentPID <= 1 {
			break
		}

		// Get parent process name and path
		var parentPath, processName string

		// Try to get executable path
		if pathCmd := exec.Command("/bin/ps", "-o", "comm=", "-p", strconv.Itoa(parentPID)); pathCmd != nil {
			if output, err := pathCmd.Output(); err == nil {
				parentPath = strings.TrimSpace(string(output))
				processName = filepath.Base(parentPath)
			}
		}

		// If we couldn't get full path, get process name
		if processName == "" {
			if nameCmd := exec.Command("/bin/ps", "-o", "command=", "-p", strconv.Itoa(parentPID)); nameCmd != nil {
				if output, err := nameCmd.Output(); err == nil {
					cmdline := strings.TrimSpace(string(output))
					if cmdline != "" {
						parts := strings.Fields(cmdline)
						if len(parts) > 0 {
							processName = filepath.Base(parts[0])
							if parentPath == "" {
								parentPath = parts[0]
							}
						}
					}
				}
			}
		}

		hierarchy = append(hierarchy, ProcessInfo{
			PID:            parentPID,
			ExecutablePath: parentPath,
			ProcessName:    processName,
		})

		currentPID = parentPID
	}

	return hierarchy
}

// String method removed - using unified String() method in peer.go
