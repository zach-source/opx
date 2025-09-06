package security

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type PeerInfo struct {
	PID  int
	UID  uint32
	GID  uint32
	Path string // best-effort executable path
}

// PeerFromUnixConn extracts peer credentials from a *net.UnixConn.
func PeerFromUnixConn(conn *net.UnixConn) (PeerInfo, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return PeerInfo{}, err
	}
	var pi PeerInfo
	var serr error

	err = raw.Control(func(fd uintptr) {
		switch runtime.GOOS {
		case "linux":
			// Get peer PID using SO_PEERCRED on Linux
			// For now, just get PID - UID/GID can be added later with more complex syscalls
			const SO_PEERCRED = 17
			pid, e := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, SO_PEERCRED)
			if e != nil {
				serr = e
				return
			}
			pi = PeerInfo{PID: pid}
		case "darwin":
			pid, e := unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
			if e != nil {
				serr = e
				return
			}
			pi = PeerInfo{PID: pid}
		default:
			serr = fmt.Errorf("peer creds unsupported on %s", runtime.GOOS)
		}
	})
	if err != nil {
		return PeerInfo{}, err
	}
	if serr != nil {
		return PeerInfo{}, serr
	}

	// Best-effort executable path
	pi.Path = exePathForPID(pi.PID)
	return pi, nil
}

func exePathForPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	switch runtime.GOOS {
	case "linux":
		p := fmt.Sprintf("/proc/%d/exe", pid)
		if target, err := os.Readlink(p); err == nil {
			return target
		}
	case "darwin":
		out, err := exec.Command("/bin/ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
		if err == nil {
			s := string(out)
			return filepath.Clean(strings.TrimSpace(s))
		}
	}
	return ""
}

// String returns a human-readable representation of PeerInfo
func (pi PeerInfo) String() string {
	if pi.Path != "" {
		return fmt.Sprintf("PID:%d Path:%s UID:%d GID:%d", pi.PID, pi.Path, pi.UID, pi.GID)
	}
	return fmt.Sprintf("PID:%d UID:%d GID:%d", pi.PID, pi.UID, pi.GID)
}
