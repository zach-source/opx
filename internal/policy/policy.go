package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/zach-source/opx/internal/security"
	"github.com/zach-source/opx/internal/util"
)

type Rule struct {
	// Process identification
	Path       string `json:"path,omitempty"`        // absolute binary path
	PathSHA256 string `json:"path_sha256,omitempty"` // sha256 of the path string
	PID        int    `json:"pid,omitempty"`         // optional exact PID match

	// Parent process requirements
	ParentPID        int      `json:"parent_pid,omitempty"`        // required parent PID
	RequiredParents  []string `json:"required_parents,omitempty"`  // required parent process names in hierarchy
	ForbiddenParents []string `json:"forbidden_parents,omitempty"` // forbidden parent process names

	// Code signing requirements (macOS)
	RequireSigned     bool     `json:"require_signed,omitempty"`      // require valid code signature
	AllowedSigningIDs []string `json:"allowed_signing_ids,omitempty"` // allowed signing identities
	AllowedTeamIDs    []string `json:"allowed_team_ids,omitempty"`    // allowed team identifiers
	RequiredCDHash    string   `json:"required_cd_hash,omitempty"`    // specific code directory hash

	// Unix credentials (Linux)
	RequiredUID *int `json:"required_uid,omitempty"` // required user ID
	RequiredGID *int `json:"required_gid,omitempty"` // required group ID

	// Process metadata
	AllowedCommands []string `json:"allowed_commands,omitempty"` // allowed command lines
	RequiredCaps    []string `json:"required_caps,omitempty"`    // required capabilities (Linux)

	// Secret access patterns
	Refs []string `json:"refs"` // allowed refs; supports "*" and prefix wildcards

	// Rule metadata
	Description string `json:"description,omitempty"` // human-readable rule description
	CreatedAt   string `json:"created_at,omitempty"`  // when rule was created
	CreatedBy   string `json:"created_by,omitempty"`  // who/what created the rule
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
	PeerInfo security.PeerInfo
}

// Allowed answers whether the Subject may read the given ref under Policy using rich peer information.
func Allowed(pol Policy, subj Subject, ref string) bool {
	if len(pol.Allow) == 0 && !pol.DefaultDeny {
		return true
	}

	for _, r := range pol.Allow {
		if !ruleMatches(r, subj.PeerInfo, ref) {
			continue
		}
		return true
	}
	return !pol.DefaultDeny
}

// ruleMatches checks if a rule matches the given peer info and reference
func ruleMatches(rule Rule, peer security.PeerInfo, ref string) bool {
	// Process identification checks
	if rule.PID != 0 && rule.PID != peer.PID {
		return false
	}

	if rule.Path != "" && !samePath(rule.Path, peer.ExecutablePath) {
		return false
	}

	if rule.PathSHA256 != "" && rule.PathSHA256 != sha256Hex(peer.ExecutablePath) {
		return false
	}

	// Parent process checks
	if rule.ParentPID != 0 && rule.ParentPID != peer.ParentPID {
		return false
	}

	// Required parents check - at least one required parent must be in hierarchy
	if len(rule.RequiredParents) > 0 {
		found := false
		for _, requiredParent := range rule.RequiredParents {
			for _, proc := range peer.ProcessHierarchy {
				if proc.ProcessName == requiredParent {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	// Forbidden parents check - none of the forbidden parents should be in hierarchy
	for _, forbiddenParent := range rule.ForbiddenParents {
		for _, proc := range peer.ProcessHierarchy {
			if proc.ProcessName == forbiddenParent {
				return false
			}
		}
	}

	// Code signing checks (macOS)
	if rule.RequireSigned && !peer.ValidSignature {
		return false
	}

	if len(rule.AllowedSigningIDs) > 0 {
		found := false
		for _, allowedID := range rule.AllowedSigningIDs {
			if peer.SigningID == allowedID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(rule.AllowedTeamIDs) > 0 {
		found := false
		for _, allowedTeam := range rule.AllowedTeamIDs {
			if peer.TeamID == allowedTeam {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if rule.RequiredCDHash != "" && rule.RequiredCDHash != peer.CDHashHex {
		return false
	}

	// Unix credentials checks (Linux)
	if rule.RequiredUID != nil && *rule.RequiredUID != peer.UID {
		return false
	}

	if rule.RequiredGID != nil && *rule.RequiredGID != peer.GID {
		return false
	}

	// TODO: Add command line and capabilities checking

	// Reference pattern matching
	return matchRef(rule.Refs, ref)
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ap := filepath.Clean(a)
	bp := filepath.Clean(b)
	return ap == bp
}
