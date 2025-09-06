package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/zach-source/opx/internal/audit"
	"github.com/zach-source/opx/internal/client"
)

func usage() {
	fmt.Fprintf(os.Stderr, `opx - client for opx-authd

Usage:
  opx [--account=ACCOUNT] read REF [REF...]
  opx [--account=ACCOUNT] resolve NAME=REF [NAME=REF ...]
  opx [--account=ACCOUNT] run --env NAME=REF [--env NAME=REF ...] -- CMD [ARGS...]
  opx status
  opx audit [--since=24h] [--interactive]

Commands:
  read                  # Read secret references
  resolve              # Resolve environment variables  
  run                  # Run command with resolved env vars
  status               # Check daemon status
  audit                # Manage access control policies

Global Flags:
  --account=ACCOUNT     # 1Password account to use

Audit Flags:
  --since=24h          # Show denials from last 24 hours (default)
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
	// Parse global flags
	var account string
	var opFlags []string

	// Find the subcommand position (first non-flag argument)
	cmdPos := -1
	for i, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--account=") {
			account = strings.TrimPrefix(arg, "--account=")
			if account != "" {
				opFlags = append(opFlags, "--account="+account)
			}
		} else if arg == "--account" && i+1 < len(os.Args[1:]) {
			account = os.Args[i+2] // i is 0-based from os.Args[1:], so i+2 for full args
			if account != "" {
				opFlags = append(opFlags, "--account="+account)
			}
			i++ // skip the next argument
		} else if !strings.HasPrefix(arg, "--") {
			cmdPos = i + 1 // +1 because we're iterating over os.Args[1:]
			break
		}
	}

	if cmdPos == -1 || cmdPos >= len(os.Args) {
		usage()
	}

	cmd := os.Args[cmdPos]
	cmdArgs := os.Args[cmdPos+1:]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cli, err := client.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "client init:", err)
		os.Exit(1)
	}
	// Handle audit command separately (doesn't need daemon connection)
	if cmd == "audit" {
		handleAuditCommand(cmdArgs)
		return
	}

	if err := cli.EnsureReady(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "daemon:", err)
		os.Exit(1)
	}

	switch cmd {
	case "status":
		if err := cli.Ping(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "status:", err)
			os.Exit(1)
		}
		fmt.Println("ok")
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
	var since string
	var interactive bool

	// Parse audit-specific flags
	auditFlags := flag.NewFlagSet("audit", flag.ExitOnError)
	auditFlags.StringVar(&since, "since", "24h", "show denials from last duration (e.g., 1h, 24h, 7d)")
	auditFlags.BoolVar(&interactive, "interactive", false, "interactive policy management")
	auditFlags.Parse(args)

	// Parse duration
	sinceData, err := time.ParseDuration(since)
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
