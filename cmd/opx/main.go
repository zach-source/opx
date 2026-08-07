package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zach-source/opx/internal/audit"
	"github.com/zach-source/opx/internal/client"
	"github.com/zach-source/opx/internal/policy"
	"github.com/zach-source/opx/internal/security"
	"github.com/zach-source/opx/internal/util"
)

// Version information (set via ldflags during build)
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type parsedArgs struct {
	command string
	args    []string
	opFlags []string
}

func parseArgs(args []string) parsedArgs {
	var result parsedArgs

	i := 0
	for i < len(args) {
		arg := args[i]

		if strings.HasPrefix(arg, "--account=") {
			account := strings.TrimPrefix(arg, "--account=")
			if account != "" {
				result.opFlags = append(result.opFlags, "--account="+account)
			}
			i++
		} else if arg == "--account" && i+1 < len(args) {
			account := args[i+1]
			if account != "" {
				result.opFlags = append(result.opFlags, "--account="+account)
			}
			i += 2
		} else if !strings.HasPrefix(arg, "--") {
			result.command = arg
			i++
			break
		} else {
			i++
		}
	}

	for i < len(args) {
		arg := args[i]

		if strings.HasPrefix(arg, "--account=") {
			account := strings.TrimPrefix(arg, "--account=")
			if account != "" {
				result.opFlags = append(result.opFlags, "--account="+account)
			}
			i++
		} else if arg == "--account" && i+1 < len(args) {
			account := args[i+1]
			if account != "" {
				result.opFlags = append(result.opFlags, "--account="+account)
			}
			i += 2
		} else {
			result.args = append(result.args, arg)
			i++
		}
	}

	return result
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, hours)
}

func usage() {
	fmt.Fprintf(os.Stderr, `opx - client for opx-authd

Usage:
  opx [--account=ACCOUNT] read REF [REF...]
  opx [--account=ACCOUNT] resolve NAME=REF [NAME=REF ...]
  opx [--account=ACCOUNT] run --env NAME=REF [--env NAME=REF ...] -- CMD [ARGS...]
  opx invalidate REF [REF...] | --all
  opx status
  opx version
  opx audit [--since=24h|2d|1w] [--interactive]
  opx audit failures [--since=24h] [--process=PATH] [--reference=REF]
  opx verify-audit [--file=path] [--since=7d|1w|1M] [--all]
  opx policy <subcommand> [options]
  opx login <backend> [options]
  opx login 1password [--account=ACCOUNT]
  opx login vault [--address=URL] [--method=userpass]
  opx login bao [--address=URL] [--method=token]

Commands:
  read                  # Read secret references (op://, vault://, bao://)
  resolve              # Resolve environment variables  
  run                  # Run command with resolved env vars
  invalidate           # Drop cached entries after rotating a secret
  status               # Check daemon status
  version              # Show client and server version information
  audit                # Manage access control policies
  verify-audit         # Verify audit log integrity
  policy               # Advanced policy management (list, add, remove, test)
  login                # Login to backends (1password, vault, bao)

Global Flags:
  --account=ACCOUNT     # 1Password account to use

Audit Flags:
  --since=24h|2d|1w|1M # Show denials from last duration (supports h/d/w/M/y)
  --interactive        # Interactive policy management

Environment:
  OPX_AUTOSTART=0       # disable daemon autostart

Examples:
  opx --account=YOPUYSOQIRHYVGIV3IQ5CS627Y read op://Private/ClaudeCodeLongLiveCreds/credential
  opx read op://vault/item/password
  opx resolve DB_PASSWORD=op://vault/database/password

`)
	os.Exit(2)
}

func main() {
	parsed := parseArgs(os.Args[1:])

	if parsed.command == "" {
		usage()
	}

	cmd := parsed.command
	cmdArgs := parsed.args
	opFlags := parsed.opFlags

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cli, err := client.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "client init:", err)
		os.Exit(1)
	}
	// Handle commands that don't need daemon connection
	switch cmd {
	case "audit":
		handleAuditCommand(cmdArgs)
		return
	case "login":
		handlePluggableLoginCommand(cmdArgs, opFlags)
		return
	case "vault-login":
		// Deprecated - redirect to new structure
		fmt.Println("⚠️  'vault-login' is deprecated. Use: opx login vault [options]")
		handleVaultLoginCommand(cmdArgs)
		return
	case "verify-audit":
		handleVerifyAuditCommand(cmdArgs)
		return
	case "version":
		handleVersionCommand()
		return
	case "policy":
		handlePolicyCommand(cmdArgs)
		return
	}

	if err := cli.EnsureReady(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "daemon:", err)
		os.Exit(1)
	}

	switch cmd {
	case "invalidate":
		all := false
		refs := []string{}
		for _, a := range cmdArgs {
			if a == "--all" {
				all = true
				continue
			}
			refs = append(refs, a)
		}
		if !all && len(refs) == 0 {
			fmt.Fprintln(os.Stderr, "usage: opx invalidate REF [REF...] | opx invalidate --all")
			os.Exit(2)
		}

		resp, err := cli.Invalidate(ctx, refs, all)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalidate:", err)
			os.Exit(1)
		}
		fmt.Printf("invalidated %d cache entries\n", resp.Removed)

	case "status":
		status, err := cli.Status(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "status:", err)
			os.Exit(1)
		}

		fmt.Printf("opx-authd status: running\n")
		fmt.Printf("  Version:      %s\n", status.Version)
		fmt.Printf("  Backend:      %s\n", status.Backend)
		fmt.Printf("  Socket:       %s\n", status.SocketPath)
		fmt.Printf("  Uptime:       %s\n", formatDuration(time.Duration(status.Uptime)*time.Second))
		fmt.Printf("\n")
		fmt.Printf("Cache:\n")
		fmt.Printf("  Size:         %d items\n", status.CacheSize)
		fmt.Printf("  TTL:          %ds\n", status.TTLSeconds)
		fmt.Printf("  Hits:         %d\n", status.Hits)
		fmt.Printf("  Misses:       %d\n", status.Misses)
		fmt.Printf("  In-flight:    %d\n", status.InFlight)
		if status.Hits+status.Misses > 0 {
			hitRate := float64(status.Hits) / float64(status.Hits+status.Misses) * 100
			fmt.Printf("  Hit rate:     %.1f%%\n", hitRate)
		}
		fmt.Printf("\n")
		if status.PolicyPath != "" {
			fmt.Printf("Policy:\n")
			fmt.Printf("  Path:         %s\n", status.PolicyPath)
			fmt.Printf("  Rules:        %d\n", status.PolicyRuleCount)
			fmt.Printf("\n")
		}
		if status.Session != nil {
			fmt.Printf("Session:\n")
			fmt.Printf("  State:        %s\n", status.Session.State)
			if status.Session.Enabled {
				fmt.Printf("  Idle timeout: %s\n", formatDuration(time.Duration(status.Session.IdleTimeout)*time.Second))
				if status.Session.TimeUntilLock > 0 {
					fmt.Printf("  Lock in:      %s\n", formatDuration(time.Duration(status.Session.TimeUntilLock)*time.Second))
				}
			} else {
				fmt.Printf("  Idle timeout: disabled\n")
			}
		}
	case "read":
		if len(cmdArgs) < 1 {
			usage()
		}
		refs := cmdArgs
		if len(refs) == 1 {
			rr, err := cli.ReadWithFlags(ctx, refs[0], opFlags)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Print(rr.Value)
			if !strings.HasSuffix(rr.Value, "\n") {
				fmt.Print("\n")
			}
			return
		}
		rrs, err := cli.ReadsWithFlags(ctx, refs, opFlags)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, ref := range refs {
			rr := rrs.Results[ref]
			fmt.Println(rr.Value)
		}
	case "resolve":
		if len(cmdArgs) < 1 {
			usage()
		}
		envmap := map[string]string{}
		for _, kv := range cmdArgs {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "bad mapping: %s\n", kv)
				os.Exit(1)
			}
			envmap[parts[0]] = parts[1]
		}
		resp, err := cli.ResolveWithFlags(ctx, envmap, opFlags)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for k, v := range resp.Env {
			fmt.Printf("%s=%s\n", k, v)
		}
	case "run":
		// parse flags until --
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		var envs multiFlag
		fs.Var(&envs, "env", "NAME=REF mapping (repeatable)")
		// find -- in the remaining cmdArgs
		sep := -1
		for i, a := range cmdArgs {
			if a == "--" {
				sep = i
				break
			}
		}
		if sep == -1 {
			usage()
		}
		_ = fs.Parse(cmdArgs[:sep])
		execArgs := cmdArgs[sep+1:]
		if len(execArgs) == 0 {
			usage()
		}
		envmap := map[string]string{}
		for _, kv := range envs {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "bad mapping: %s\n", kv)
				os.Exit(1)
			}
			envmap[parts[0]] = parts[1]
		}
		resp, err := cli.ResolveWithFlags(ctx, envmap, opFlags)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// Exec locally with injected env
		cmdExec := exec.CommandContext(ctx, execArgs[0], execArgs[1:]...)
		cmdExec.Stdout = os.Stdout
		cmdExec.Stderr = os.Stderr
		cmdExec.Stdin = os.Stdin
		cmdExec.Env = os.Environ()
		for k, v := range resp.Env {
			cmdExec.Env = append(cmdExec.Env, fmt.Sprintf("%s=%s", k, v))
		}
		if err := cmdExec.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				os.Exit(ee.ExitCode())
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
	}
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func handleAuditCommand(args []string) {
	// Check for subcommands first
	if len(args) > 0 && args[0] == "failures" {
		handleAuditFailuresCommand(args[1:])
		return
	}

	var since string
	var interactive bool

	// Parse audit-specific flags
	auditFlags := flag.NewFlagSet("audit", flag.ExitOnError)
	auditFlags.StringVar(&since, "since", "24h", "show denials from last duration (e.g., 1h, 24h, 2d, 1w, 1M)")
	auditFlags.BoolVar(&interactive, "interactive", false, "interactive policy management")
	auditFlags.Parse(args)

	// Parse duration
	sinceData, err := util.ParseDuration(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid duration %s: %v\n", since, err)
		os.Exit(1)
	}

	// Scan for recent denials
	fmt.Printf("Scanning audit log for denials in the last %s...\n", since)
	denials, err := audit.ScanRecentDenials(sinceData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to scan audit log: %v\n", err)
		os.Exit(1)
	}

	if len(denials) == 0 {
		fmt.Printf("No access denials found in the last %s.\n", since)
		if interactive {
			fmt.Println("Your access control policy appears to be working correctly!")
		}
		return
	}

	fmt.Printf("\nFound %d unique access denials:\n\n", len(denials))

	// Display all denials
	for i, denial := range denials {
		fmt.Print(audit.FormatDenialForDisplay(i, denial))
	}

	if !interactive {
		fmt.Println("Use --interactive to manage policy rules for these denials.")
		return
	}

	// Interactive mode - let user select denials to allow
	fmt.Println("\nInteractive Policy Management")
	fmt.Println("Select denials to create allow rules for (comma-separated numbers, or 'q' to quit):")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
		os.Exit(1)
	}

	input = strings.TrimSpace(input)
	if input == "q" || input == "quit" {
		fmt.Println("Exiting without changes.")
		return
	}

	// Parse selection
	indices := parseSelection(input)
	if len(indices) == 0 {
		fmt.Println("No valid selections made.")
		return
	}

	// Process each selected denial
	for _, idx := range indices {
		if idx < 0 || idx >= len(denials) {
			fmt.Printf("Invalid selection: %d\n", idx+1)
			continue
		}

		denial := denials[idx]
		fmt.Printf("\nCreating allow rule for: %s -> %s\n", denial.Path, denial.Reference)

		// Suggest patterns
		patterns := audit.SuggestAllowPattern(denial.Reference)
		fmt.Println("Select permission level:")
		for i, pattern := range patterns {
			fmt.Printf("  [%d] %s\n", i+1, pattern)
		}
		fmt.Print("Choice (1-3): ")

		choiceInput, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Failed to read choice: %v\n", err)
			continue
		}

		choice, err := strconv.Atoi(strings.TrimSpace(choiceInput))
		if err != nil || choice < 1 || choice > len(patterns) {
			fmt.Printf("Invalid choice, skipping %s\n", denial.Reference)
			continue
		}

		selectedPattern := patterns[choice-1]
		rule := audit.CreatePolicyRuleFromDenial(denial, selectedPattern)

		// Add rule to policy
		if err := audit.AddRuleToPolicy(rule); err != nil {
			fmt.Printf("Failed to add rule: %v\n", err)
			continue
		}

		fmt.Printf("✅ Added rule: %s can access %s\n", denial.Path, selectedPattern)
	}

	fmt.Println("\n🎉 Policy updated! Restart opx-authd to apply changes:")
	fmt.Println("  sudo systemctl --user restart opx-authd")
	fmt.Println("  # or kill and restart manually")
}

func parseSelection(input string) []int {
	var indices []int
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Parse number (1-based) and convert to 0-based index
		num, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		if num > 0 {
			indices = append(indices, num-1)
		}
	}

	return indices
}

func handleVersionCommand() {
	fmt.Printf("opx client version: %s\n", version)
	if commit != "unknown" {
		fmt.Printf("  commit: %s\n", commit)
	}
	if date != "unknown" {
		fmt.Printf("  built: %s\n", date)
	}

	// Try to get server info from daemon
	ctx := context.Background()
	cli, err := client.New()
	if err != nil {
		fmt.Printf("opx-authd server: unavailable (%v)\n", err)
		return
	}

	if err := cli.EnsureReady(ctx); err != nil {
		fmt.Printf("opx-authd server: unavailable (%v)\n", err)
		return
	}

	// Test connection with ping
	if err := cli.Ping(ctx); err != nil {
		fmt.Printf("opx-authd server: error connecting (%v)\n", err)
		return
	}

	fmt.Printf("opx-authd server: connected\n")
	fmt.Printf("  status: ok\n")

	// Try to get the server process command line
	serverPath, serverCmd := getServerProcessInfo()
	if serverPath != "" {
		fmt.Printf("  executable: %s\n", serverPath)
	}
	if serverCmd != "" {
		fmt.Printf("  command: %s\n", serverCmd)
	}

	fmt.Printf("  note: use 'opx status' for detailed daemon information\n")
}

// getServerProcessInfo tries to find the running opx-authd process and get its command line
func getServerProcessInfo() (string, string) {
	// Use ps to find opx-authd processes
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "opx-authd") && !strings.Contains(line, "grep") && !strings.Contains(line, "OPX_AUTHD_PATH") {
			fields := strings.Fields(line)
			if len(fields) >= 11 {
				// Extract command line (everything from field 10 onwards in ps aux)
				cmdline := strings.Join(fields[10:], " ")

				// Only return if this looks like the actual daemon process (not shell wrapper)
				if strings.HasSuffix(fields[10], "opx-authd") {
					return fields[10], cmdline
				}
			}
		}
	}

	return "", ""
}

func handleAuditFailuresCommand(args []string) {
	var since string
	var processFilter string
	var referenceFilter string

	// Parse audit failures flags
	failuresFlags := flag.NewFlagSet("audit-failures", flag.ExitOnError)
	failuresFlags.StringVar(&since, "since", "24h", "show failures from last duration")
	failuresFlags.StringVar(&processFilter, "process", "", "filter by process path")
	failuresFlags.StringVar(&referenceFilter, "reference", "", "filter by secret reference")
	failuresFlags.Parse(args)

	sinceData, err := util.ParseDuration(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid duration %s: %v\n", since, err)
		os.Exit(1)
	}

	fmt.Printf("🔍 Scanning for access failures in the last %s...\n", since)

	// Get denials with filtering
	denials, err := audit.ScanRecentDenials(sinceData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to scan denials: %v\n", err)
		os.Exit(1)
	}

	// Apply filters
	var filteredDenials []audit.DenialEvent
	for _, denial := range denials {
		if processFilter != "" && !strings.Contains(denial.Path, processFilter) {
			continue
		}
		if referenceFilter != "" && !strings.Contains(denial.Reference, referenceFilter) {
			continue
		}
		filteredDenials = append(filteredDenials, denial)
	}

	if len(filteredDenials) == 0 {
		fmt.Println("✅ No access failures found with current filters.")
		if processFilter != "" || referenceFilter != "" {
			fmt.Println("Try removing filters or extending the time range.")
		}
		return
	}

	fmt.Printf("❌ Found %d access failures:\n\n", len(filteredDenials))

	for i, denial := range filteredDenials {
		fmt.Printf("🚫 Failure #%d:\n", i+1)
		fmt.Printf("   Process: %s\n", denial.Path)
		fmt.Printf("   Reference: %s\n", denial.Reference)
		fmt.Printf("   Count: %d failures\n", denial.Count)
		fmt.Printf("   Last: %s\n", denial.Timestamp.Format("2006-01-02 15:04:05"))

		// Try to determine why it failed using policy debug
		fmt.Printf("   💡 Quick Analysis:\n")

		// Create peer info for analysis
		peer := security.PeerInfo{
			PID:            denial.PID,
			ExecutablePath: denial.Path,
			Signed:         true, // Assume signed for analysis
			ValidSignature: true,
		}

		// Load policy and do quick evaluation
		pm, err := policy.NewPolicyManager()
		if err == nil {
			evaluation := pm.TestAccessDetailed(peer, denial.Reference)
			if len(evaluation.Steps) > 0 {
				// Show the most relevant failure reason
				for _, step := range evaluation.Steps {
					if step.Result == "FAIL" {
						fmt.Printf("      → %s: %s\n", step.Check, step.Reason)
						break
					}
				}
			} else {
				fmt.Printf("      → %s\n", evaluation.Reason)
			}
		}

		fmt.Println()
	}

	fmt.Println("💡 To fix these failures:")
	fmt.Println("   opx policy add --interactive    # Create rules from failures")
	fmt.Printf("   opx policy debug <process> <ref> # Detailed analysis\n")
}

// AccessAttempt represents an access attempt from audit logs
type AccessAttempt struct {
	ProcessPath      string
	Reference        string
	Decision         string
	Count            int
	LastAttempt      time.Time
	ParentPID        int
	ProcessHierarchy []security.ProcessInfo
	SigningInfo      string
}

// scanAllAccessAttempts scans audit logs for all access decisions (ALLOW and DENY)
func scanAllAccessAttempts(since time.Duration) ([]AccessAttempt, error) {
	// Create roller to find log files
	roller, err := audit.NewRoller(audit.DefaultRollerConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create roller: %w", err)
	}
	defer roller.Close()

	logFiles, err := roller.ListLogFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to list log files: %w", err)
	}

	if len(logFiles) == 0 {
		return []AccessAttempt{}, nil
	}

	// Parse all access attempts
	attempts := make(map[string]*AccessAttempt)
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

			var secureEvent audit.SecureAuditEvent
			if err := json.Unmarshal([]byte(line), &secureEvent); err != nil {
				continue // Skip malformed lines
			}

			event := secureEvent.Event
			if event.Event != "ACCESS_DECISION" || event.Timestamp.Before(cutoff) {
				continue
			}

			// Create unique key for this process+reference+decision combination
			key := fmt.Sprintf("%s|%s|%s", event.PeerInfo.ExecutablePath, event.Reference, event.Decision)

			if existing, exists := attempts[key]; exists {
				existing.Count++
				if event.Timestamp.After(existing.LastAttempt) {
					existing.LastAttempt = event.Timestamp
				}
			} else {
				signingInfo := ""
				if event.PeerInfo.Signed {
					signingInfo = fmt.Sprintf("Signed:%s Team:%s", event.PeerInfo.SigningID, event.PeerInfo.TeamID)
				} else {
					signingInfo = "Unsigned"
				}

				attempts[key] = &AccessAttempt{
					ProcessPath:      event.PeerInfo.ExecutablePath,
					Reference:        event.Reference,
					Decision:         event.Decision,
					Count:            1,
					LastAttempt:      event.Timestamp,
					ParentPID:        event.PeerInfo.ParentPID,
					ProcessHierarchy: event.PeerInfo.ProcessHierarchy,
					SigningInfo:      signingInfo,
				}
			}
		}

		file.Close()
	}

	// Convert to slice
	var result []AccessAttempt
	for _, attempt := range attempts {
		result = append(result, *attempt)
	}

	return result, nil
}

func handleLoginCommand(opFlags []string) {
	fmt.Println("Logging into 1Password...")

	// Build op signin command with optional account flag
	args := []string{"signin"}
	args = append(args, opFlags...)

	// Execute op signin interactively
	cmd := exec.Command("op", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "1Password signin failed with exit code %d\n", exitErr.ExitCode())
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Failed to execute 1Password signin: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Successfully logged into 1Password")
	fmt.Println("You can now use opx to read secrets:")
	fmt.Println("  opx read 'op://vault/item/field'")
	fmt.Println("")
	fmt.Println("If you have multiple accounts, set OP_ACCOUNT environment variable:")
	fmt.Println("  export OP_ACCOUNT=your-account-id")
}

func handleVaultLoginCommand(args []string) {
	var address string
	var method string
	var usernameRef string
	var passwordRef string
	var tokenRef string

	// Parse vault-login specific flags
	vaultFlags := flag.NewFlagSet("vault-login", flag.ExitOnError)
	vaultFlags.StringVar(&address, "address", "http://localhost:8200", "Vault server address")
	vaultFlags.StringVar(&method, "method", "userpass", "authentication method (token|userpass)")
	vaultFlags.StringVar(&usernameRef, "username-ref", "", "secret reference for username (e.g., op://vault/creds/username)")
	vaultFlags.StringVar(&passwordRef, "password-ref", "", "secret reference for password (e.g., op://vault/creds/password)")
	vaultFlags.StringVar(&tokenRef, "token-ref", "", "secret reference for token (e.g., op://vault/token/value)")
	vaultFlags.Parse(args)

	// Check for credential references (self-authentication)
	if tokenRef != "" || usernameRef != "" || passwordRef != "" {
		fmt.Printf("Using self-authentication with credential references...\n")
		err := performSelfAuthentication(address, method, tokenRef, usernameRef, passwordRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Self-authentication failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Self-authentication successful!")
		fmt.Printf("Vault credentials stored. Start daemon with: ./bin/opx-authd --backend=vault --verbose\n")
		return
	}

	fmt.Printf("Logging into Vault at %s using %s authentication...\n", address, method)

	switch method {
	case "token":
		fmt.Println("For token authentication, set the VAULT_TOKEN environment variable:")
		fmt.Println("  export VAULT_TOKEN=your-vault-token")
		fmt.Println("Or use credential reference:")
		fmt.Println("  opx login vault --token-ref=\"op://vault/vault-token/value\"")
		fmt.Println("Then start the daemon with:")
		fmt.Printf("  ./bin/opx-authd --backend=vault --verbose\n")

	case "userpass":
		fmt.Println("For userpass authentication:")
		fmt.Println("1. Set environment variables:")
		fmt.Println("   export VAULT_ADDR=" + address)
		fmt.Println("   export VAULT_USERNAME=your-username")
		fmt.Println("   export VAULT_PASSWORD=your-password")
		fmt.Println("")
		fmt.Println("2. Or use credential references:")
		fmt.Println("   opx login vault --username-ref=\"op://vault/vault-creds/username\" --password-ref=\"op://vault/vault-creds/password\"")
		fmt.Println("")
		fmt.Println("3. Start daemon:")
		fmt.Println("   ./bin/opx-authd --backend=vault --verbose")

	default:
		fmt.Fprintf(os.Stderr, "Unsupported authentication method: %s\n", method)
		fmt.Println("Supported methods: token, userpass")
		os.Exit(1)
	}

	fmt.Println("")
	fmt.Println("After authentication, you can read Vault secrets:")
	fmt.Println("  opx read 'vault://secret/myapp/config#password'")
	fmt.Println("  opx read 'bao://kv/production/api#key'")
}

func handleVerifyAuditCommand(args []string) {
	var logFile string
	var since string
	var verifyAll bool

	// Parse verify-audit specific flags
	verifyFlags := flag.NewFlagSet("verify-audit", flag.ExitOnError)
	verifyFlags.StringVar(&logFile, "file", "", "specific log file to verify")
	verifyFlags.StringVar(&since, "since", "7d", "verify logs from last duration")
	verifyFlags.BoolVar(&verifyAll, "all", false, "verify all available audit logs")
	verifyFlags.Parse(args)

	fmt.Println("Verifying audit log integrity...")

	// Create integrity manager for verification
	integrityManager, err := audit.NewIntegrityManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create integrity manager: %v\n", err)
		os.Exit(1)
	}

	if logFile != "" {
		// Verify specific file
		valid, errors, err := integrityManager.VerifyLogFile(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to verify log file %s: %v\n", logFile, err)
			os.Exit(1)
		}

		if valid {
			fmt.Printf("✅ Log file %s: INTEGRITY VERIFIED\n", logFile)
		} else {
			fmt.Printf("❌ Log file %s: INTEGRITY COMPROMISED\n", logFile)
			for _, errMsg := range errors {
				fmt.Printf("  - %s\n", errMsg)
			}
			os.Exit(1)
		}
		return
	}

	// Multi-file verification
	if verifyAll {
		verifyAllAuditFiles(integrityManager)
		return
	}

	if since != "7d" {
		verifySinceAuditFiles(integrityManager, since)
		return
	}

	fmt.Println("Use --file=path to verify specific log files")
	fmt.Println("Example: opx verify-audit --file=~/.local/share/opx-authd/audit-2025-01-15.log")
}

func handlePolicyCommand(args []string) {
	if len(args) < 1 {
		printPolicyUsage()
		return
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "list":
		handlePolicyList()
	case "add":
		handlePolicyAdd(subArgs)
	case "remove":
		handlePolicyRemove(subArgs)
	case "test":
		handlePolicyTest(subArgs)
	case "debug":
		handlePolicyDebug(subArgs)
	case "review":
		handlePolicyReview(subArgs)
	default:
		printPolicyUsage()
	}
}

func printPolicyUsage() {
	fmt.Fprintf(os.Stderr, `opx policy - Advanced policy management

Usage:
  opx policy list                           # List current policy rules
  opx policy add [--interactive]            # Add new policy rule
  opx policy remove <rule-index>            # Remove policy rule by index
  opx policy test <process-path> <ref>      # Test if access would be allowed
  opx policy debug <process-path> <ref>     # Detailed rule evaluation breakdown
  opx policy review [--since=24h]          # Review recent access and approve/deny

Examples:
  opx policy list                          # Show all rules
  opx policy add --interactive             # Interactive rule creation
  opx policy remove 2                      # Remove rule #2
  opx policy test /usr/bin/kubectl op://prod/k8s/token
  opx policy debug /usr/bin/kubectl op://prod/k8s/token # Detailed evaluation
  opx policy review --since=1h             # Review last hour's access attempts
`)
}

func handlePolicyList() {
	pm, err := policy.NewPolicyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create policy manager: %v\n", err)
		os.Exit(1)
	}

	rules, err := pm.ListRules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list rules: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Current Policy Rules:")
	fmt.Println()
	for _, rule := range rules {
		fmt.Println(rule)
		fmt.Println()
	}
}

func handlePolicyAdd(args []string) {
	interactive := false
	for _, arg := range args {
		if arg == "--interactive" {
			interactive = true
		}
	}

	if interactive {
		handleInteractivePolicyAdd()
		return
	}

	fmt.Println("Non-interactive policy add not yet implemented")
	fmt.Println("Use: opx policy add --interactive")
}

func handleInteractivePolicyAdd() {
	fmt.Println("Interactive Policy Rule Creation")
	fmt.Println("================================")

	reader := bufio.NewReader(os.Stdin)

	// Get process path
	fmt.Print("Process path (or press Enter to analyze recent denials): ")
	processPath, _ := reader.ReadString('\n')
	processPath = strings.TrimSpace(processPath)

	if processPath == "" {
		// Analyze recent denials
		fmt.Println("Analyzing recent access denials...")
		denials, err := audit.ScanRecentDenials(24 * time.Hour)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to scan denials: %v\n", err)
			os.Exit(1)
		}

		if len(denials) == 0 {
			fmt.Println("No recent denials found. Please specify a process path manually.")
			return
		}

		fmt.Printf("Found %d recent denials:\n\n", len(denials))
		for i, denial := range denials {
			fmt.Print(audit.FormatDenialForDisplay(i, denial))
		}

		fmt.Print("Select denial to create rule for (1-" + fmt.Sprintf("%d", len(denials)) + "): ")
		selectionStr, _ := reader.ReadString('\n')
		selection, err := strconv.Atoi(strings.TrimSpace(selectionStr))
		if err != nil || selection < 1 || selection > len(denials) {
			fmt.Println("Invalid selection")
			return
		}

		denial := denials[selection-1]
		processPath = denial.Path

		// Create rule from denial
		pm, err := policy.NewPolicyManager()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create policy manager: %v\n", err)
			os.Exit(1)
		}

		// Create mock peer info from denial (simplified)
		peer := security.PeerInfo{
			PID:            denial.PID,
			ExecutablePath: denial.Path,
		}

		fmt.Println("\nSelect permission scope:")
		fmt.Println("  [1] Exact reference only")
		fmt.Println("  [2] Entire vault")
		fmt.Println("  [3] All secrets")
		fmt.Print("Choice (1-3): ")

		scopeStr, _ := reader.ReadString('\n')
		scope, err := strconv.Atoi(strings.TrimSpace(scopeStr))
		if err != nil || scope < 1 || scope > 3 {
			fmt.Println("Invalid scope selection")
			return
		}

		scopeMap := map[int]string{1: "exact", 2: "vault", 3: "all"}
		rule := pm.CreateRuleFromPeerInfo(peer, denial.Reference, scopeMap[scope])

		if err := pm.AddRule(rule); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to add rule: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Added rule for %s\n", processPath)
		fmt.Println("Restart daemon to apply changes:")
		fmt.Println("  pkill opx-authd")
		return
	}

	fmt.Printf("Process path specified: %s\n", processPath)
	fmt.Println("Advanced rule creation not yet implemented")
}

func handlePolicyRemove(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: opx policy remove <rule-index>")
		return
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid rule index: %s\n", args[0])
		os.Exit(1)
	}

	pm, err := policy.NewPolicyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create policy manager: %v\n", err)
		os.Exit(1)
	}

	if err := pm.RemoveRule(index - 1); err != nil { // Convert 1-based to 0-based
		fmt.Fprintf(os.Stderr, "Failed to remove rule: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Removed rule %d\n", index)
	fmt.Println("Restart daemon to apply changes:")
	fmt.Println("  pkill opx-authd")
}

func handlePolicyTest(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: opx policy test <process-path> <reference>")
		fmt.Println("Example: opx policy test /usr/bin/kubectl op://prod/k8s/token")
		return
	}

	processPath := args[0]
	reference := args[1]

	// Create mock peer info for testing (with signing info for accurate testing)
	peer := security.PeerInfo{
		PID:            999999, // Mock PID
		ExecutablePath: processPath,
		Signed:         true,   // Assume signed for testing
		ValidSignature: true,   // Assume valid for testing
		SigningID:      "test", // Mock signing ID
	}

	pm, err := policy.NewPolicyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create policy manager: %v\n", err)
		os.Exit(1)
	}

	allowed, reason := pm.TestAccess(peer, reference)

	if allowed {
		fmt.Printf("✅ ALLOW: %s can access %s\n", processPath, reference)
		fmt.Printf("Reason: %s\n", reason)
	} else {
		fmt.Printf("❌ DENY: %s cannot access %s\n", processPath, reference)
		fmt.Printf("Reason: %s\n", reason)
	}
}

func handlePolicyDebug(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: opx policy debug <process-path> <reference>")
		fmt.Println("Example: opx policy debug /nix/store/.../opx op://Private/AnthropicAPI/credential")
		return
	}

	processPath := args[0]
	reference := args[1]

	// Create mock peer info for debugging
	peer := security.PeerInfo{
		PID:            999999, // Mock PID
		ExecutablePath: processPath,
		Signed:         true,    // Assume signed for debugging
		ValidSignature: true,    // Assume valid for debugging
		SigningID:      "debug", // Mock signing ID
	}

	pm, err := policy.NewPolicyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create policy manager: %v\n", err)
		os.Exit(1)
	}

	// Get detailed evaluation
	evaluation := pm.TestAccessDetailed(peer, reference)

	fmt.Printf("🔍 Policy Evaluation Debug for:\n")
	fmt.Printf("   Process: %s\n", processPath)
	fmt.Printf("   Reference: %s\n", reference)
	fmt.Printf("   Decision: %s\n", evaluation.Decision)
	fmt.Printf("   Reason: %s\n\n", evaluation.Reason)

	if len(evaluation.Steps) > 0 {
		fmt.Println("📋 Rule Evaluation Steps:")
		for _, step := range evaluation.Steps {
			status := "❌"
			if step.Result == "PASS" {
				status = "✅"
			} else if step.Result == "SKIP" {
				status = "⏭️"
			}

			fmt.Printf("   %s %s: %s\n", status, step.Check, step.Reason)
			if step.Expected != "" && step.Actual != "" {
				fmt.Printf("      Expected: %s\n", step.Expected)
				fmt.Printf("      Actual: %s\n", step.Actual)
			}
		}
	} else {
		fmt.Println("📋 No detailed steps available (using simplified evaluation)")
	}

	fmt.Println()
	if evaluation.Decision == "DENY" {
		fmt.Println("💡 Suggestions:")
		fmt.Println("   - Check if process path is correct")
		fmt.Println("   - Verify signing requirements")
		fmt.Println("   - Review reference patterns in matching rules")
		fmt.Println("   - Use: opx policy add --interactive to create a rule")
	}
}

func handlePolicyReview(args []string) {
	since := "24h"
	for i, arg := range args {
		if strings.HasPrefix(arg, "--since=") {
			since = strings.TrimPrefix(arg, "--since=")
		} else if arg == "--since" && i+1 < len(args) {
			since = args[i+1]
		}
	}

	// Parse duration
	sinceData, err := util.ParseDuration(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid duration %s: %v\n", since, err)
		os.Exit(1)
	}

	fmt.Printf("Reviewing access attempts from last %s...\n", since)

	// Get all access attempts (both ALLOW and DENY)
	attempts, err := scanAllAccessAttempts(sinceData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to scan access attempts: %v\n", err)
		os.Exit(1)
	}

	if len(attempts) == 0 {
		fmt.Printf("No access attempts found in the last %s.\n", since)
		return
	}

	fmt.Printf("Found %d access attempts:\n\n", len(attempts))

	// Group by decision type
	var allows, denies []AccessAttempt
	for _, attempt := range attempts {
		if attempt.Decision == "ALLOW" {
			allows = append(allows, attempt)
		} else {
			denies = append(denies, attempt)
		}
	}

	// Collect output for paging
	var output []string

	if len(denies) > 0 {
		output = append(output, fmt.Sprintf("❌ DENIED ACCESS (%d attempts):", len(denies)))
		for i, attempt := range denies {
			output = append(output, formatDetailedAccessAttempt(i+1, attempt))
		}
		output = append(output, "")
	}

	if len(allows) > 0 {
		output = append(output, fmt.Sprintf("✅ ALLOWED ACCESS (%d attempts):", len(allows)))
		for i, attempt := range allows {
			output = append(output, formatDetailedAccessAttempt(i+1, attempt))
		}
		output = append(output, "")
	}

	// Page output if long
	pageOutput(output)

	fmt.Println("Use: opx policy add --interactive to create rules from denied access")
	fmt.Println("Use: opx audit --interactive for detailed denial management")
}

// formatDetailedAccessAttempt formats an access attempt with rich hierarchy information
func formatDetailedAccessAttempt(index int, attempt AccessAttempt) string {
	result := fmt.Sprintf("  [%d] %s → %s (%s %d times)",
		index, filepath.Base(attempt.ProcessPath), attempt.Reference,
		strings.ToLower(attempt.Decision), attempt.Count)

	result += fmt.Sprintf("\n      Process: %s", attempt.ProcessPath)
	result += fmt.Sprintf("\n      Last: %s", attempt.LastAttempt.Format("2006-01-02 15:04:05"))

	if attempt.SigningInfo != "" {
		result += fmt.Sprintf("\n      Signing: %s", attempt.SigningInfo)
	}

	if attempt.ParentPID > 0 {
		result += fmt.Sprintf("\n      Parent PID: %d", attempt.ParentPID)
	}

	if len(attempt.ProcessHierarchy) > 0 {
		result += "\n      Call Chain: "
		for i, proc := range attempt.ProcessHierarchy {
			if i > 0 {
				result += " → "
			}
			processDesc := fmt.Sprintf("%s(%d)", proc.ProcessName, proc.PID)
			if proc.Verified {
				processDesc += "✓"
				if proc.ValidSignature {
					processDesc += "🔒"
				}
			} else {
				processDesc += "?"
			}
			result += processDesc
		}
	}

	return result
}

// pageOutput displays output with more/less style paging for long content
func pageOutput(lines []string) {
	if len(lines) <= 20 { // Show directly if short
		for _, line := range lines {
			fmt.Println(line)
		}
		return
	}

	// Use more/less style paging for long output
	fmt.Printf("Output has %d lines. Press Enter to continue, 'q' to quit, or number + Enter to jump:\n", len(lines))

	reader := bufio.NewReader(os.Stdin)
	currentLine := 0
	pageSize := 20

	for currentLine < len(lines) {
		// Show current page
		endLine := currentLine + pageSize
		if endLine > len(lines) {
			endLine = len(lines)
		}

		for i := currentLine; i < endLine; i++ {
			fmt.Println(lines[i])
		}

		// Check if we're done
		if endLine >= len(lines) {
			break
		}

		// Show pager prompt
		remaining := len(lines) - endLine
		fmt.Printf("--More-- (%d lines remaining) [Enter=next page, q=quit, number=jump]: ", remaining)

		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		input = strings.TrimSpace(input)

		if input == "q" || input == "quit" {
			fmt.Println("(quit)")
			break
		}

		if input == "" {
			// Continue to next page
			currentLine = endLine
			continue
		}

		// Try to parse as line number
		if lineNum, err := strconv.Atoi(input); err == nil {
			if lineNum > 0 && lineNum <= len(lines) {
				currentLine = lineNum - 1 // Convert to 0-based
				continue
			}
		}

		// Invalid input, continue normally
		currentLine = endLine
	}
}

func verifyAllAuditFiles(integrityManager *audit.IntegrityManager) {
	// Create roller to get list of log files
	roller, err := audit.NewRoller(audit.DefaultRollerConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create roller: %v\n", err)
		os.Exit(1)
	}
	defer roller.Close()

	logFiles, err := roller.ListLogFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list log files: %v\n", err)
		os.Exit(1)
	}

	if len(logFiles) == 0 {
		fmt.Println("No audit log files found.")
		return
	}

	fmt.Printf("Verifying %d audit log files...\n\n", len(logFiles))

	allValid := true
	for _, logFile := range logFiles {
		fmt.Printf("Checking %s...", filepath.Base(logFile))

		valid, errors, err := integrityManager.VerifyLogFile(logFile)
		if err != nil {
			fmt.Printf(" ERROR: %v\n", err)
			allValid = false
			continue
		}

		if valid {
			fmt.Println(" ✅ VERIFIED")
		} else {
			fmt.Println(" ❌ COMPROMISED")
			for _, errMsg := range errors {
				fmt.Printf("  - %s\n", errMsg)
			}
			allValid = false
		}
	}

	fmt.Println()
	if allValid {
		fmt.Println("🎉 All audit logs verified successfully!")
	} else {
		fmt.Println("⚠️  Some audit logs failed verification.")
		os.Exit(1)
	}
}

func verifySinceAuditFiles(integrityManager *audit.IntegrityManager, since string) {
	sinceData, err := util.ParseDuration(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid duration %s: %v\n", since, err)
		os.Exit(1)
	}

	// Create roller to get list of log files
	roller, err := audit.NewRoller(audit.DefaultRollerConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create roller: %v\n", err)
		os.Exit(1)
	}
	defer roller.Close()

	logFiles, err := roller.ListLogFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list log files: %v\n", err)
		os.Exit(1)
	}

	// Filter files by date
	cutoff := time.Now().Add(-sinceData)
	var relevantFiles []string

	for _, logFile := range logFiles {
		// Extract date from filename (audit-2025-01-15.log)
		base := filepath.Base(logFile)
		if !strings.HasPrefix(base, "audit-") || !strings.HasSuffix(base, ".log") {
			continue
		}

		dateStr := strings.TrimSuffix(strings.TrimPrefix(base, "audit-"), ".log")
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		if fileDate.After(cutoff) {
			relevantFiles = append(relevantFiles, logFile)
		}
	}

	if len(relevantFiles) == 0 {
		fmt.Printf("No audit log files found from the last %s.\n", since)
		return
	}

	fmt.Printf("Verifying %d audit log files from last %s...\n\n", len(relevantFiles), since)

	allValid := true
	for _, logFile := range relevantFiles {
		fmt.Printf("Checking %s...", filepath.Base(logFile))

		valid, errors, err := integrityManager.VerifyLogFile(logFile)
		if err != nil {
			fmt.Printf(" ERROR: %v\n", err)
			allValid = false
			continue
		}

		if valid {
			fmt.Println(" ✅ VERIFIED")
		} else {
			fmt.Println(" ❌ COMPROMISED")
			for _, errMsg := range errors {
				fmt.Printf("  - %s\n", errMsg)
			}
			allValid = false
		}
	}

	fmt.Println()
	if allValid {
		fmt.Printf("🎉 All audit logs from last %s verified successfully!\n", since)
	} else {
		fmt.Printf("⚠️  Some audit logs from last %s failed verification.\n", since)
		os.Exit(1)
	}
}

func handlePluggableLoginCommand(args []string, opFlags []string) {
	if len(args) < 1 {
		printLoginUsage()
		return
	}

	backend := args[0]
	backendArgs := args[1:]

	switch backend {
	case "1password":
		handleLoginCommand(opFlags)
	case "vault":
		handleVaultLoginCommand(backendArgs)
	case "bao":
		handleBaoLoginCommand(backendArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown backend: %s\n", backend)
		printLoginUsage()
		os.Exit(1)
	}
}

func printLoginUsage() {
	fmt.Fprintf(os.Stderr, `opx login - Backend authentication

Usage:
  opx login <backend> [options]

Backends:
  1password [--account=ACCOUNT]       # Login to 1Password account
  vault [--address=URL] [--method=M]  # Login to HashiCorp Vault  
  bao [--address=URL] [--method=M]    # Login to OpenBao

Self-Authentication (Credential Chaining):
  vault [--username-ref=REF] [--password-ref=REF] [--token-ref=REF]
  bao [--token-ref=REF] [--username-ref=REF] [--password-ref=REF]

Examples:
  opx login 1password --account=MYACCOUNT
  opx login vault --username-ref="op://vault/vault-creds/username" --password-ref="op://vault/vault-creds/password"
  opx login bao --token-ref="op://vault/bao-token/value"
`)
}

func handleBaoLoginCommand(args []string) {
	var address string
	var method string

	// Parse bao-login specific flags
	baoFlags := flag.NewFlagSet("login-bao", flag.ExitOnError)
	baoFlags.StringVar(&address, "address", "http://localhost:8300", "Bao server address")
	baoFlags.StringVar(&method, "method", "token", "authentication method (token|userpass)")
	baoFlags.Parse(args)

	fmt.Printf("Logging into Bao at %s using %s authentication...\n", address, method)

	switch method {
	case "token":
		fmt.Println("For token authentication, set the BAO_TOKEN environment variable:")
		fmt.Println("  export BAO_TOKEN=your-bao-token")
		fmt.Println("Then start the daemon with:")
		fmt.Printf("  ./bin/opx-authd --backend=bao --verbose\n")

	case "userpass":
		fmt.Println("For userpass authentication:")
		fmt.Println("1. Set environment variables:")
		fmt.Println("   export BAO_ADDR=" + address)
		fmt.Println("   export BAO_USERNAME=your-username")
		fmt.Println("   export BAO_PASSWORD=your-password")
		fmt.Println("")
		fmt.Println("2. Start daemon:")
		fmt.Println("   ./bin/opx-authd --backend=bao --verbose")

	default:
		fmt.Fprintf(os.Stderr, "Unsupported authentication method: %s\n", method)
		fmt.Println("Supported methods: token, userpass")
		os.Exit(1)
	}

	fmt.Println("")
	fmt.Println("After authentication, you can read Bao secrets:")
	fmt.Println("  opx read 'bao://kv/production/api#key'")
}

// performSelfAuthentication handles automated authentication using credential references
func performSelfAuthentication(address, method, tokenRef, usernameRef, passwordRef string) error {
	ctx := context.Background()

	// Create a client to read credential references
	cli, err := client.New()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	if err := cli.EnsureReady(ctx); err != nil {
		return fmt.Errorf("daemon not ready: %w", err)
	}

	switch method {
	case "token":
		if tokenRef == "" {
			return fmt.Errorf("token-ref required for token authentication")
		}

		fmt.Printf("Reading token from: %s\n", tokenRef)
		tokenResp, err := cli.Read(ctx, tokenRef)
		if err != nil {
			return fmt.Errorf("failed to read token reference: %w", err)
		}

		// Store token in environment file for daemon usage
		return storeVaultCredentials(map[string]string{
			"VAULT_ADDR":  address,
			"VAULT_TOKEN": tokenResp.Value,
		})

	case "userpass":
		if usernameRef == "" || passwordRef == "" {
			return fmt.Errorf("both username-ref and password-ref required for userpass authentication")
		}

		fmt.Printf("Reading credentials from: %s, %s\n", usernameRef, passwordRef)

		// Read username
		usernameResp, err := cli.Read(ctx, usernameRef)
		if err != nil {
			return fmt.Errorf("failed to read username reference: %w", err)
		}

		// Read password
		passwordResp, err := cli.Read(ctx, passwordRef)
		if err != nil {
			return fmt.Errorf("failed to read password reference: %w", err)
		}

		// Store credentials in environment file for daemon usage
		return storeVaultCredentials(map[string]string{
			"VAULT_ADDR":     address,
			"VAULT_USERNAME": usernameResp.Value,
			"VAULT_PASSWORD": passwordResp.Value,
		})

	default:
		return fmt.Errorf("unsupported authentication method: %s", method)
	}
}

// storeVaultCredentials stores Vault credentials in a secure environment file
func storeVaultCredentials(credentials map[string]string) error {
	// Get config directory
	configDir, err := getConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	credFile := filepath.Join(configDir, "vault.env")

	// Create credentials file content
	var content strings.Builder
	content.WriteString("# Vault credentials (auto-generated by opx self-authentication)\n")
	content.WriteString("# This file contains sensitive information - keep secure\n")
	content.WriteString("\n")

	for key, value := range credentials {
		content.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	// Write securely
	if err := os.WriteFile(credFile, []byte(content.String()), 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	fmt.Printf("Credentials stored in: %s\n", credFile)
	fmt.Println("Start daemon with: VAULT_ENV_FILE=" + credFile + " ./bin/opx-authd --backend=vault")

	return nil
}

// getConfigDir gets the XDG config directory (simplified version)
func getConfigDir() (string, error) {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		dir := filepath.Join(xdgConfig, "opx-authd")
		return dir, os.MkdirAll(dir, 0700)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(homeDir, ".config", "opx-authd")
	return dir, os.MkdirAll(dir, 0700)
}
