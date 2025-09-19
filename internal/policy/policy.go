package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// PolicyEvaluationStep represents a step in policy evaluation
type PolicyEvaluationStep struct {
	RuleIndex int    `json:"rule_index"`
	RuleName  string `json:"rule_name"`
	Check     string `json:"check"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual"`
	Result    string `json:"result"` // "PASS", "FAIL", "SKIP"
	Reason    string `json:"reason"`
}

// PolicyEvaluationResult contains the complete evaluation breakdown
type PolicyEvaluationResult struct {
	Decision string                 `json:"decision"` // "ALLOW", "DENY"
	Reason   string                 `json:"reason"`
	Steps    []PolicyEvaluationStep `json:"steps"`
}

// EvaluatePolicy provides detailed policy evaluation with step-by-step breakdown
func EvaluatePolicy(pol Policy, peer security.PeerInfo, ref string) PolicyEvaluationResult {
	result := PolicyEvaluationResult{
		Decision: "DENY",
		Reason:   "Default deny policy",
		Steps:    []PolicyEvaluationStep{},
	}

	if len(pol.Allow) == 0 && !pol.DefaultDeny {
		result.Decision = "ALLOW"
		result.Reason = "No rules defined and default allow"
		return result
	}

	for i, rule := range pol.Allow {
		ruleName := rule.Description
		if ruleName == "" {
			ruleName = fmt.Sprintf("Rule %d", i+1)
		}

		ruleResult := evaluateRuleDetailed(rule, peer, ref, i, ruleName)
		result.Steps = append(result.Steps, ruleResult.Steps...)

		if ruleResult.Decision == "ALLOW" {
			result.Decision = "ALLOW"
			result.Reason = fmt.Sprintf("Allowed by %s", ruleName)
			return result
		}
	}

	// No rules matched
	if pol.DefaultDeny {
		result.Reason = "No matching rules and default deny policy"
	} else {
		result.Decision = "ALLOW"
		result.Reason = "No matching rules but default allow policy"
	}

	return result
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

// ruleMatches checks if a rule matches the given peer info and reference (simplified for backward compatibility)
func ruleMatches(rule Rule, peer security.PeerInfo, ref string) bool {
	evaluation := evaluateRuleDetailed(rule, peer, ref, -1, "")
	return evaluation.Decision == "ALLOW"
}

// evaluateRuleDetailed provides step-by-step rule evaluation with detailed logging
func evaluateRuleDetailed(rule Rule, peer security.PeerInfo, ref string, ruleIndex int, ruleName string) PolicyEvaluationResult {
	result := PolicyEvaluationResult{
		Decision: "DENY",
		Reason:   "Rule conditions not met",
		Steps:    []PolicyEvaluationStep{},
	}

	// Process identification checks
	if rule.PID != 0 {
		step := PolicyEvaluationStep{
			RuleIndex: ruleIndex,
			RuleName:  ruleName,
			Check:     "PID",
			Expected:  fmt.Sprintf("%d", rule.PID),
			Actual:    fmt.Sprintf("%d", peer.PID),
		}
		if rule.PID == peer.PID {
			step.Result = "PASS"
		} else {
			step.Result = "FAIL"
			step.Reason = "PID mismatch"
			result.Steps = append(result.Steps, step)
			return result
		}
		result.Steps = append(result.Steps, step)
	}

	if rule.Path != "" {
		step := PolicyEvaluationStep{
			RuleIndex: ruleIndex,
			RuleName:  ruleName,
			Check:     "Path",
			Expected:  rule.Path,
			Actual:    peer.ExecutablePath,
		}
		if samePath(rule.Path, peer.ExecutablePath) {
			step.Result = "PASS"
		} else {
			step.Result = "FAIL"
			step.Reason = "Executable path mismatch"
			result.Steps = append(result.Steps, step)
			return result
		}
		result.Steps = append(result.Steps, step)
	}

	if rule.PathSHA256 != "" {
		expectedSHA := rule.PathSHA256
		actualSHA := sha256Hex(peer.ExecutablePath)
		step := PolicyEvaluationStep{
			RuleIndex: ruleIndex,
			RuleName:  ruleName,
			Check:     "PathSHA256",
			Expected:  expectedSHA,
			Actual:    actualSHA,
		}
		if expectedSHA == actualSHA {
			step.Result = "PASS"
		} else {
			step.Result = "FAIL"
			step.Reason = "Path SHA256 mismatch"
			result.Steps = append(result.Steps, step)
			return result
		}
		result.Steps = append(result.Steps, step)
	}

	// Code signing checks (macOS)
	if rule.RequireSigned {
		step := PolicyEvaluationStep{
			RuleIndex: ruleIndex,
			RuleName:  ruleName,
			Check:     "RequireSigned",
			Expected:  "true",
			Actual:    fmt.Sprintf("signed=%t valid=%t", peer.Signed, peer.ValidSignature),
		}
		if peer.ValidSignature {
			step.Result = "PASS"
		} else {
			step.Result = "FAIL"
			step.Reason = "Binary not signed or signature invalid"
			result.Steps = append(result.Steps, step)
			return result
		}
		result.Steps = append(result.Steps, step)
	}

	// Reference pattern matching - final check
	step := PolicyEvaluationStep{
		RuleIndex: ruleIndex,
		RuleName:  ruleName,
		Check:     "RefPattern",
		Expected:  fmt.Sprintf("refs=%v", rule.Refs),
		Actual:    ref,
	}
	if matchRef(rule.Refs, ref) {
		step.Result = "PASS"
		step.Reason = "Reference pattern matched"
		result.Steps = append(result.Steps, step)
		result.Decision = "ALLOW"
		result.Reason = "All rule conditions passed"
		return result
	} else {
		step.Result = "FAIL"
		step.Reason = "Reference pattern did not match"
		result.Steps = append(result.Steps, step)
		return result
	}
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ap := filepath.Clean(a)
	bp := filepath.Clean(b)
	return ap == bp
}
