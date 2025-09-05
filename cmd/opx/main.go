package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zach-source/opx/internal/client"
)

func usage() {
	fmt.Fprintf(os.Stderr, `opx - client for op-authd

Usage:
  opx [--account=ACCOUNT] read REF [REF...]
  opx [--account=ACCOUNT] resolve NAME=REF [NAME=REF ...]
  opx [--account=ACCOUNT] run --env NAME=REF [--env NAME=REF ...] -- CMD [ARGS...]
  opx status

Global Flags:
  --account=ACCOUNT     # 1Password account to use

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
