//go:build linux

package security

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Linux-specific peer information is defined in the main peer.go file

// InspectPeerProcess extracts the peer credentials (PID/UID/GID) for a *Unix* domain
// socket, reads process metadata from /proc, and gathers IMA/EVM evidence if present.
//
// LIMITATION: Peer PID is only available for UNIX domain sockets (not TCP).
func InspectPeerProcess(conn net.Conn) (*PeerInfo, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("connection is not a Unix domain socket (Linux exposes SO_PEERCRED only for Unix sockets)")
	}

	raw, err := uc.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("SyscallConn: %w", err)
	}

	var fd int
	if err := raw.Control(func(x uintptr) { fd = int(x) }); err != nil {
		return nil, fmt.Errorf("control: %w", err)
	}

	// SO_PEERCRED -> ucred { pid, uid, gid }
	ucred, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return nil, fmt.Errorf("getsockopt(SO_PEERCRED): %w", err)
	}

	info := &PeerInfo{
		PID: int(ucred.Pid),
		UID: int(ucred.Uid),
		GID: int(ucred.Gid),
	}

	// /proc/<pid>/exe -> executable path (symlink)
	exePath, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", info.PID))
	info.ExecutablePath = exePath

	// /proc/<pid>/stat -> parent PID (third field)
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", info.PID)); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) >= 4 {
			if ppid, err := strconv.Atoi(fields[3]); err == nil {
				info.ParentPID = ppid
			}
		}
	}

	// Build process hierarchy
	info.ProcessHierarchy = buildProcessHierarchyLinux(info.PID)

	// /proc/<pid>/cmdline (NUL-separated)
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", info.PID)); err == nil {
		parts := bytes.Split(bytes.TrimRight(b, "\x00"), []byte{0})
		for _, p := range parts {
			info.Cmdline = append(info.Cmdline, string(p))
		}
	}

	// /proc/<pid>/status (grab CapEff)
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", info.PID)); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "CapEff:") {
				info.CapEff = strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
				break
			}
		}
	}

	// /proc/<pid>/cgroup (first line is fine; adjust to your needs)
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", info.PID)); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) > 0 {
			info.Cgroup = lines[0]
		}
	}

	// Compute SHA-256 of executable (if we could resolve it)
	if info.ExecutablePath != "" {
		if h, err := sha256File(info.ExecutablePath); err == nil {
			info.SHA256 = h
		}
		// IMA/EVM evidence (best-effort)
		imaHex, hasIMA := readXattrHex(info.ExecutablePath, "security.ima")
		evmHex, hasEVM := readXattrHex(info.ExecutablePath, "security.evm")
		info.IMA.HasSecurityIMA = hasIMA
		info.IMA.HasSecurityEVM = hasEVM
		info.IMA.SecurityIMAHex = trimHex(imaHex, 128)
		info.IMA.SecurityEVMHex = trimHex(evmHex, 128)
	}

	return info, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readXattrHex(path, name string) (hexStr string, ok bool) {
	// Determine size
	sz, err := unix.Getxattr(path, name, nil)
	if err != nil || sz <= 0 {
		return "", false
	}
	buf := make([]byte, sz)
	n, err := unix.Getxattr(path, name, buf)
	if err != nil || n <= 0 {
		return "", false
	}
	return hex.EncodeToString(buf[:n]), true
}

func trimHex(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(" + strconv.Itoa(len(s)) + " hex chars)"
}

// buildProcessHierarchyLinux builds the parent process hierarchy up to init (PID 1)
func buildProcessHierarchyLinux(startPID int) []ProcessInfo {
	var hierarchy []ProcessInfo
	currentPID := startPID
	visited := make(map[int]bool) // Prevent infinite loops

	// Walk up the parent chain
	for depth := 0; depth < 10 && currentPID > 1; depth++ { // Limit depth to prevent issues
		if visited[currentPID] {
			break // Circular reference protection
		}
		visited[currentPID] = true

		// Get parent PID from /proc/<pid>/stat
		statPath := fmt.Sprintf("/proc/%d/stat", currentPID)
		b, err := os.ReadFile(statPath)
		if err != nil {
			break // Process might have exited
		}

		fields := strings.Fields(string(b))
		if len(fields) < 4 {
			break
		}

		parentPID, err := strconv.Atoi(fields[3])
		if err != nil || parentPID <= 1 {
			break
		}

		// Get parent process info
		parentExePath, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", parentPID))
		processName := filepath.Base(parentExePath)
		if processName == "" {
			processName = fields[1] // comm field from stat
			processName = strings.Trim(processName, "()")
		}

		hierarchy = append(hierarchy, ProcessInfo{
			PID:            parentPID,
			ExecutablePath: parentExePath,
			ProcessName:    processName,
		})

		currentPID = parentPID
	}

	return hierarchy
}

// String method removed - using unified String() method in peer.go
