package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zach-source/opx/internal/policy"
	"github.com/zach-source/opx/internal/util"
)

// DenialEvent represents a parsed denial event from audit logs
type DenialEvent struct {
	Timestamp time.Time `json:"timestamp"`
	PID       int       `json:"pid"`
	Path      string    `json:"path"`
	Reference string    `json:"reference"`
	Count     int       `json:"count"` // How many times this combination was denied
}

// ScanRecentDenials reads audit logs and returns recent denial events
func ScanRecentDenials(since time.Duration) ([]DenialEvent, error) {
	// Create a roller to find log files
	roller, err := NewRoller(DefaultRollerConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create roller: %w", err)
	}
	defer roller.Close()

	// Get list of log files to scan
	logFiles, err := roller.ListLogFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to list log files: %w", err)
	}

	if len(logFiles) == 0 {
		return []DenialEvent{}, nil // No audit logs exist yet
	}

	// Parse denial events from all relevant log files
	denials := make(map[string]*DenialEvent)
	cutoff := time.Now().Add(-since)

	for _, logFile := range logFiles {
		file, err := os.Open(logFile)
		if err != nil {
			continue // Skip files we can't open
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var event AuditEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue // Skip malformed lines
			}

			// Only interested in recent access denials
			if event.Event != "ACCESS_DECISION" || event.Decision != "DENY" || event.Timestamp.Before(cutoff) {
				continue
			}

			// Create unique key for this process+reference combination
			key := fmt.Sprintf("%s|%s", event.PeerInfo.Path, event.Reference)

			if existing, exists := denials[key]; exists {
				existing.Count++
				// Keep the most recent timestamp
				if event.Timestamp.After(existing.Timestamp) {
					existing.Timestamp = event.Timestamp
				}
			} else {
				denials[key] = &DenialEvent{
					Timestamp: event.Timestamp,
					PID:       event.PeerInfo.PID,
					Path:      event.PeerInfo.Path,
					Reference: event.Reference,
					Count:     1,
				}
			}
		}

		if err := scanner.Err(); err != nil {
			// Log error but continue with other files
			continue
		}

		file.Close()
	}

	// Convert to slice and sort by count (most frequent first)
	var result []DenialEvent
	for _, denial := range denials {
		result = append(result, *denial)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result, nil
}

// CreatePolicyRuleFromDenial creates a policy rule that would allow the denied access
func CreatePolicyRuleFromDenial(denial DenialEvent, allowPattern string) policy.Rule {
	return policy.Rule{
		Path: denial.Path,
		Refs: []string{allowPattern},
	}
}

// SuggestAllowPattern suggests appropriate allow patterns for a reference
func SuggestAllowPattern(reference string) []string {
	suggestions := []string{
		reference, // Exact match
	}

	// Extract vault and suggest vault-level access
	parts := strings.Split(reference, "/")
	if len(parts) >= 3 && strings.HasPrefix(reference, "op://") {
		vault := parts[2]
		suggestions = append(suggestions, fmt.Sprintf("op://%s/*", vault))
	}

	// Add wildcard option
	suggestions = append(suggestions, "*")

	return suggestions
}

// AddRuleToPolicy adds a rule to an existing policy and saves it
func AddRuleToPolicy(rule policy.Rule) error {
	// Load current policy
	pol, _, err := policy.Load()
	if err != nil {
		return fmt.Errorf("failed to load current policy: %w", err)
	}

	// Add the new rule
	pol.Allow = append(pol.Allow, rule)

	// If this is the first rule and default_deny isn't set, set it to true
	// to ensure the policy actually takes effect
	if len(pol.Allow) == 1 && !pol.DefaultDeny {
		pol.DefaultDeny = true
	}

	// Save the updated policy
	configDir, err := util.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	policyFile := filepath.Join(configDir, "policy.json")
	data, err := json.MarshalIndent(pol, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	if err := os.WriteFile(policyFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write policy file: %w", err)
	}

	return nil
}

// FormatDenialForDisplay formats a denial event for user display
func FormatDenialForDisplay(i int, denial DenialEvent) string {
	return fmt.Sprintf("[%d] Process: %s\n    Reference: %s\n    Denied: %d times, Last: %s\n",
		i+1,
		denial.Path,
		denial.Reference,
		denial.Count,
		denial.Timestamp.Format("2006-01-02 15:04:05"))
}

// FilterDenialsByPath filters denials for a specific executable path using generics
func FilterDenialsByPath(denials []DenialEvent, path string) []DenialEvent {
	return util.Filter(denials, func(d DenialEvent) bool {
		return d.Path == path
	})
}

// GroupDenialsByPath groups denials by executable path using generics
func GroupDenialsByPath(denials []DenialEvent) map[string][]DenialEvent {
	return util.GroupBy(denials, func(d DenialEvent) string {
		return d.Path
	})
}

// FindMostFrequentDenial finds the denial with the highest count using generics
func FindMostFrequentDenial(denials []DenialEvent) (DenialEvent, bool) {
	return util.FindFirst(denials, func(d DenialEvent) bool {
		// Since denials are sorted by count desc, the first one is most frequent
		return true
	})
}
