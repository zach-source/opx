
# op-authd-proto

A prototype **1Password op CLI batching daemon** (`op-authd`) with a companion client (`opx`).
It coalesces concurrent secret reads across processes, caches results briefly, and provides a simple
local API over a Unix domain socket.

> **Status:** Prototype. Linux/macOS (Unix socket). Windows named-pipe support would be a follow-up.

## Why?
Toolchains that shell out to `op read ...` many times end up spamming auth prompts and duplicate API calls.
This daemon centralizes those reads, **coalesces identical in-flight requests**, and **short-caches** results.

## Features
- Unix domain socket server at `~/.op-authd/socket.sock`
- Bearer token in `~/.op-authd/token` (0600) and directory perms 0700
- In-memory TTL cache (default 120s) with single-flight coalescing
- Backends:
  - `opcli` (default): shells out to `op read <ref>` and relies on 1Password's built-in auth/daemon
  - `fake`: returns deterministic dummy values for testing (`export OP_AUTHD_BACKEND=fake`)
- Endpoints:
  - `POST /v1/read` – read a single ref
  - `POST /v1/reads` – batch read multiple refs
  - `POST /v1/resolve` – resolve env var mapping `{ENV: ref}`
  - `GET  /v1/status` – health/counters

## Install
```bash
make build
# Binaries in ./bin: op-authd, opx
```

## Run Daemon
```bash
./bin/op-authd --ttl 120 --verbose
```

## Client Usage
```bash
# Single read
./bin/opx read "op://Engineering/DB/password"

# Batch read (multiple args)
./bin/opx read op://Vault/A/secret1 op://Vault/B/secret2

# Resolve env vars then run a command locally
./bin/opx run --env DB_PASS=op://Engineering/DB/password -- -- bash -lc 'echo "db pass: $DB_PASS"'
```

The client will attempt to autostart the daemon if it can't connect. You can disable this via `OPX_AUTOSTART=0`.

## Security Notes
- The socket directory is `0700`, token is `0600`. Only your user should be able to talk to the daemon.
- Values are kept in-memory only and zeroized on replacement/eviction to the extent Go allows.
- This is a prototype: **do not** expose the socket to other users or mount it across trust boundaries.

## Systemd (user) example
```ini
# ~/.config/systemd/user/op-authd.service
[Unit]
Description=op-authd prototype
After=default.target

[Service]
ExecStart=%h/op-authd-proto/bin/op-authd --ttl 120 --verbose
Restart=on-failure

[Install]
WantedBy=default.target
```
```bash
systemctl --user daemon-reload
systemctl --user enable --now op-authd
```

## Implementation sketch
- HTTP over Unix socket with custom `http.Transport` dialing `unix` (client) and `http.Serve` (server)
- `singleflight.Group` to coalesce identical `ref` lookups
- Small TTL cache keyed by `ref`
- Backend interface:
  ```go
  type Backend interface {
    ReadRef(ctx context.Context, ref string) (string, error)
  }
  ```

## Caveats
- Assumes the `op` CLI is installed and signed-in (for `opcli` backend).
- Windows not yet implemented (named pipes TBD).

## License
MIT
