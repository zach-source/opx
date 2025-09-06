package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/zach-source/opx/internal/util"
)

type Rule struct {
	Path       string   `json:"path,omitempty"`        // absolute binary path
	PathSHA256 string   `json:"path_sha256,omitempty"` // sha256 of the path string
	PID        int      `json:"pid,omitempty"`         // optional exact PID match
	Refs       []string `json:"refs"`                  // allowed refs; supports "*" and prefix wildcards
}

type Policy struct {
	Allow       []Rule `json:"allow"`
	DefaultDeny bool   `json:"default_deny"`
}

func defaultPolicy() Policy {
	return Policy{
		Allow:       []Rule{},
		DefaultDeny: false,
	}
}

// Load reads policy.json from XDG config directory if present; otherwise returns default.
func Load() (Policy, string, error) {
	configDir, err := util.ConfigDir()
	if err != nil {
		return Policy{}, "", err
	}
	p := filepath.Join(configDir, "policy.json")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultPolicy(), p, nil
		}
		return Policy{}, p, err
	}
	var pol Policy
	if err := json.Unmarshal(b, &pol); err != nil {
		return Policy{}, p, err
	}
	return pol, p, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func matchRef(allowed []string, ref string) bool {
	for _, a := range allowed {
		if a == "*" {
			return true
		}
		if strings.HasSuffix(a, "*") {
			if strings.HasPrefix(ref, strings.TrimSuffix(a, "*")) {
				return true
			}
		} else if ref == a {
			return true
		}
	}
	return false
}

type Subject struct {
	PID  int
	Path string
}

// Allowed answers whether the Subject may read the given ref under Policy.
func Allowed(pol Policy, subj Subject, ref string) bool {
	if len(pol.Allow) == 0 && !pol.DefaultDeny {
		return true
	}
	for _, r := range pol.Allow {
		if r.PID != 0 && r.PID != subj.PID {
			continue
		}
		if r.Path != "" && !samePath(r.Path, subj.Path) {
			continue
		}
		if r.PathSHA256 != "" && r.PathSHA256 != sha256Hex(subj.Path) {
			continue
		}
		if matchRef(r.Refs, ref) {
			return true
		}
	}
	return !pol.DefaultDeny
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ap := filepath.Clean(a)
	bp := filepath.Clean(b)
	return ap == bp
}
