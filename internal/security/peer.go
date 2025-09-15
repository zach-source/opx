package security

import (
	"fmt"
	"net"
	"runtime"
)

// PeerInfo represents peer process information with platform-specific fields
type PeerInfo struct {
	// Common fields (all platforms)
	PID            int
	ExecutablePath string

	// Unix credentials (Linux)
	UID int
	GID int

	// Process details (Linux)
	Cmdline []string
	Cgroup  string
	CapEff  string // hex string (from /proc/<pid>/status)

	// IMA/EVM evidence (Linux)
	IMA struct {
		HasSecurityIMA bool   // xattr present
		SecurityIMAHex string // raw xattr hex (first 128 hex chars shown if large)
		HasSecurityEVM bool
		SecurityEVMHex string // raw xattr hex (first 128 hex chars shown if large)
	}

	SHA256 string // sha256 of the executable file

	// macOS code signing fields
	SigningID       string // macOS code signing identity
	TeamID          string // macOS team identifier
	CDHashHex       string // macOS code directory hash
	Flags           uint32 // macOS codesign flags
	Signed          bool   // whether binary is code signed
	ValidSignature  bool   // whether signature is valid
	HasEntitlements bool   // whether binary has entitlements
}

// String returns a human-readable representation of PeerInfo
func (pi PeerInfo) String() string {
	if pi.ExecutablePath != "" {
		switch runtime.GOOS {
		case "linux":
			return fmt.Sprintf("PID:%d Path:%s UID:%d GID:%d", pi.PID, pi.ExecutablePath, pi.UID, pi.GID)
		case "darwin":
			if pi.Signed {
				return fmt.Sprintf("PID:%d Path:%s Signed:%s Team:%s", pi.PID, pi.ExecutablePath, pi.SigningID, pi.TeamID)
			}
			return fmt.Sprintf("PID:%d Path:%s Unsigned", pi.PID, pi.ExecutablePath)
		default:
			return fmt.Sprintf("PID:%d Path:%s", pi.PID, pi.ExecutablePath)
		}
	}
	return fmt.Sprintf("PID:%d", pi.PID)
}

// PeerFromUnixConn extracts peer credentials from a *net.UnixConn using platform-specific implementation
func PeerFromUnixConn(conn *net.UnixConn) (*PeerInfo, error) {
	return InspectPeerProcess(conn)
}

// exePathForPID is now handled by platform-specific implementations
