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

		// Verify parent process using Security framework
		verified, signingInfo := verifyParentProcessMacOS(parentPID, parentPath)

		procInfo := ProcessInfo{
			PID:            parentPID,
			ExecutablePath: parentPath,
			ProcessName:    processName,
			Verified:       verified,
		}

		// Add signing information if verification succeeded
		if verified && signingInfo != nil {
			procInfo.SigningID = signingInfo.SigningID
			procInfo.TeamID = signingInfo.TeamID
			procInfo.CDHashHex = signingInfo.CDHashHex
			procInfo.ValidSignature = signingInfo.ValidSignature
		}

		hierarchy = append(hierarchy, procInfo)

		currentPID = parentPID
	}

	return hierarchy
}

// verifyParentProcessMacOS verifies a parent process using Security framework with debug logging
func verifyParentProcessMacOS(pid int, expectedPath string) (bool, *csInfo) {
	fmt.Printf("[DEBUG] Verifying parent process PID:%d ExpectedPath:%s\n", pid, expectedPath)

	// Check if the process still exists first (fast check)
	if !ProcExists(pid) {
		fmt.Printf("[DEBUG] PID:%d does not exist\n", pid)
		return false, nil
	}

	fmt.Printf("[DEBUG] PID:%d exists, proceeding with Security framework verification\n", pid)

	// Get code signature info for the PID (this might fail for some processes)
	info, err := getCodeSignatureInfoForPID(pid)
	if err != nil {
		fmt.Printf("[DEBUG] Security framework failed for PID:%d: %v\n", pid, err)

		// Security framework failed - this can happen for system processes or unsigned binaries
		// We still consider it "verified" if the process exists and path matches
		if expectedPath != "" {
			// Do a simple path verification without signature validation
			actualPath := getExecutablePathMacOS(pid)
			fmt.Printf("[DEBUG] Fallback path check for PID:%d: actual=%s expected=%s\n",
				pid, actualPath, expectedPath)

			if pathsMatch(expectedPath, actualPath) {
				fmt.Printf("[DEBUG] PID:%d path verification succeeded (unsigned)\n", pid)
				return true, &csInfo{
					ExecutablePath: actualPath,
					Signed:         false,
					ValidSignature: false,
				}
			} else {
				fmt.Printf("[DEBUG] PID:%d path verification failed: mismatch\n", pid)
			}
		}

		fmt.Printf("[DEBUG] PID:%d verification failed: no valid path or signature\n", pid)
		return false, nil
	}

	fmt.Printf("[DEBUG] Security framework succeeded for PID:%d: path=%s signed=%t valid=%t\n",
		pid, info.ExecutablePath, info.Signed, info.ValidSignature)

	// Verify that the PID's executable path matches what we expect
	if expectedPath != "" && !pathsMatch(expectedPath, info.ExecutablePath) {
		fmt.Printf("[DEBUG] PID:%d path mismatch: expected=%s actual=%s (possible spoofing)\n",
			pid, expectedPath, info.ExecutablePath)
		return false, nil
	}

	fmt.Printf("[DEBUG] PID:%d fully verified with Security framework\n", pid)
	return true, info
}

// getExecutablePathMacOS gets executable path for a PID using ps command
func getExecutablePathMacOS(pid int) string {
	cmd := exec.Command("/bin/ps", "-o", "comm=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// pathsMatch checks if two paths refer to the same executable, handling symlinks and Nix store paths
func pathsMatch(expected, actual string) bool {
	// Direct match
	if expected == actual {
		return true
	}

	// Resolve symlinks for comparison
	expectedResolved, err1 := filepath.EvalSymlinks(expected)
	actualResolved, err2 := filepath.EvalSymlinks(actual)

	// If both resolve successfully, compare resolved paths
	if err1 == nil && err2 == nil {
		if expectedResolved == actualResolved {
			return true
		}
	}

	// Check if the actual path is a Nix store path for the expected binary
	if strings.HasPrefix(actual, "/nix/store/") {
		expectedBase := filepath.Base(expected)
		actualBase := filepath.Base(actual)

		// If base names match, consider it a match (handles Nix store paths)
		if expectedBase == actualBase {
			return true
		}

		// Handle versioned binaries (e.g., zsh vs zsh-5.9)
		if strings.HasPrefix(actualBase, expectedBase+"-") {
			return true
		}
	}

	// Special case for "claude" -> node mapping
	if expected == "claude" && strings.Contains(actual, "node") {
		return true
	}

	return false
}

// String method removed - using unified String() method in peer.go
