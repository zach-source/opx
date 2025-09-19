package policy

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zach-source/opx/internal/security"
	"github.com/zach-source/opx/internal/util"
)

// PolicyManager handles advanced policy operations
type PolicyManager struct {
	policyPath string
}

// NewPolicyManager creates a new policy manager
func NewPolicyManager() (*PolicyManager, error) {
	configDir, err := util.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	return &PolicyManager{
		policyPath: filepath.Join(configDir, "policy.json"),
	}, nil
}

// AddRule adds a new rule to the policy
func (pm *PolicyManager) AddRule(rule Rule) error {
	pol, _, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load policy: %w", err)
	}

	// Set creation metadata
	rule.CreatedAt = time.Now().Format(time.RFC3339)
	rule.CreatedBy = "opx-cli"

	pol.Allow = append(pol.Allow, rule)

	return pm.savePolicy(pol)
}

// RemoveRule removes a rule by index
func (pm *PolicyManager) RemoveRule(index int) error {
	pol, _, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load policy: %w", err)
	}

	if index < 0 || index >= len(pol.Allow) {
		return fmt.Errorf("invalid rule index: %d", index)
	}

	// Remove rule at index
	pol.Allow = append(pol.Allow[:index], pol.Allow[index+1:]...)

	return pm.savePolicy(pol)
}

// UpdateRule updates a rule by index
func (pm *PolicyManager) UpdateRule(index int, rule Rule) error {
	pol, _, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load policy: %w", err)
	}

	if index < 0 || index >= len(pol.Allow) {
		return fmt.Errorf("invalid rule index: %d", index)
	}

	// Preserve original creation data
	rule.CreatedAt = pol.Allow[index].CreatedAt
	rule.CreatedBy = pol.Allow[index].CreatedBy

	pol.Allow[index] = rule

	return pm.savePolicy(pol)
}

// TestAccess tests if given peer info would be allowed access to reference
func (pm *PolicyManager) TestAccess(peer security.PeerInfo, ref string) (bool, string) {
	pol, _, err := Load()
	if err != nil {
		return false, fmt.Sprintf("Failed to load policy: %v", err)
	}

	subject := Subject{PeerInfo: peer}
	allowed := Allowed(pol, subject, ref)

	if allowed {
		// Find which rule matched
		for i, rule := range pol.Allow {
			if ruleMatches(rule, peer, ref) {
				return true, fmt.Sprintf("Allowed by rule %d: %s", i+1, rule.Description)
			}
		}
		return true, "Allowed by default policy"
	}

	return false, "Denied by policy"
}

// TestAccessDetailed provides comprehensive policy evaluation with step-by-step breakdown
func (pm *PolicyManager) TestAccessDetailed(peer security.PeerInfo, ref string) PolicyEvaluationResult {
	pol, _, err := Load()
	if err != nil {
		return PolicyEvaluationResult{
			Decision: "ERROR",
			Reason:   fmt.Sprintf("Failed to load policy: %v", err),
			Steps:    []PolicyEvaluationStep{},
		}
	}

	return EvaluatePolicy(pol, peer, ref)
}

// CreateRuleFromPeerInfo creates a rule suggestion based on peer information
func (pm *PolicyManager) CreateRuleFromPeerInfo(peer security.PeerInfo, ref string, scope string) Rule {
	rule := Rule{
		Path:        peer.ExecutablePath,
		Description: fmt.Sprintf("Auto-generated rule for %s", filepath.Base(peer.ExecutablePath)),
		CreatedAt:   time.Now().Format(time.RFC3339),
		CreatedBy:   "opx-audit-review",
	}

	// Set reference scope
	switch scope {
	case "exact":
		rule.Refs = []string{ref}
	case "vault":
		// Extract vault and allow all from that vault
		parts := strings.Split(ref, "/")
		if len(parts) >= 3 && strings.HasPrefix(ref, "op://") {
			rule.Refs = []string{fmt.Sprintf("op://%s/*", parts[2])}
		} else {
			rule.Refs = []string{ref}
		}
	case "all":
		rule.Refs = []string{"*"}
	default:
		rule.Refs = []string{ref}
	}

	// Add parent requirements if available
	if peer.ParentPID > 0 && len(peer.ProcessHierarchy) > 0 {
		// Require immediate parent
		if len(peer.ProcessHierarchy) > 0 {
			rule.RequiredParents = []string{peer.ProcessHierarchy[0].ProcessName}
		}
	}

	// Add code signing requirements on macOS
	if peer.Signed && peer.ValidSignature {
		rule.RequireSigned = true
		if peer.SigningID != "" {
			rule.AllowedSigningIDs = []string{peer.SigningID}
		}
		if peer.TeamID != "" {
			rule.AllowedTeamIDs = []string{peer.TeamID}
		}
	}

	return rule
}

// ListRules returns formatted list of current policy rules
func (pm *PolicyManager) ListRules() ([]string, error) {
	pol, _, err := Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load policy: %w", err)
	}

	var rules []string
	for i, rule := range pol.Allow {
		desc := rule.Description
		if desc == "" {
			desc = fmt.Sprintf("Rule for %s", filepath.Base(rule.Path))
		}

		ruleStr := fmt.Sprintf("[%d] %s", i+1, desc)

		// Add process info
		if rule.Path != "" {
			ruleStr += fmt.Sprintf("\n    Process: %s", rule.Path)
		}

		// Add parent requirements
		if len(rule.RequiredParents) > 0 {
			ruleStr += fmt.Sprintf("\n    Required Parents: %v", rule.RequiredParents)
		}

		// Add signing info
		if rule.RequireSigned {
			ruleStr += "\n    Requires: Valid code signature"
		}

		// Add refs
		ruleStr += fmt.Sprintf("\n    Secrets: %v", rule.Refs)

		rules = append(rules, ruleStr)
	}

	if len(rules) == 0 {
		rules = append(rules, "No policy rules defined (allow all by default)")
	}

	return rules, nil
}

// savePolicy saves the policy to disk
func (pm *PolicyManager) savePolicy(pol Policy) error {
	data, err := json.MarshalIndent(pol, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	return util.AtomicWriteFile(pm.policyPath, data, 0600)
}

// BackupPolicy creates a timestamped backup of the current policy
func (pm *PolicyManager) BackupPolicy() (string, error) {
	pol, _, err := Load()
	if err != nil {
		return "", fmt.Errorf("failed to load policy: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := pm.policyPath + ".backup." + timestamp

	data, err := json.MarshalIndent(pol, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy: %w", err)
	}

	if err := util.AtomicWriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}
