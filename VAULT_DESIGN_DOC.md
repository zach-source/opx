Design: Vault/Bao backend for secret retrieval

Overview

We will extend our existing secret retrieval infrastructure—currently implemented
as a local caching daemon and a CLI front‑end—to support HashiCorp Vault and
its open‑source fork OpenBao.  This design introduces new CLI front‑ends
(vaultx and baox) and a vault backend inside the daemon that can securely
fetch secrets from Vault/Bao.

Motivation

Our current system provides a local caching daemon that reduces the number of
interactive prompts when retrieving secrets from 1Password.  Many
organizations also use HashiCorp Vault or OpenBao for secret management.  By
integrating Vault/Bao into the same backend, developers can use one tool to
fetch secrets from multiple backends while still amortizing authentication and
maintaining a consistent approval workflow.

Goals
	•	Multi‑backend support: Read secrets from HashiCorp Vault and
OpenBao through the same caching daemon used for 1Password.
	•	New CLI front‑ends: Provide new binaries, vaultx and baox, that
integrate with the daemon and present a familiar command‑line interface for
Vault/Bao operations such as reading secrets or injecting environment
variables.
	•	Security and approval: Preserve security by requiring explicit user
approval for authentication and limiting token lifetimes.  Vault/Bao
credentials should never be used without the user’s knowledge.
	•	Efficiency: Maintain the ability to batch multiple secret requests and
cache results, similar to our existing 1Password backend.
	•	Portability: Keep the design portable across Linux and macOS; Windows
support can follow later.

Architecture

The system has three major components:
	1.	Daemon (op-authd extended with a vault backend):  The daemon runs as a
local service and exposes an HTTP API over a Unix domain socket.  It
supports multiple backends (“opcli”, “fake”, and now “vault”).  For the
vault backend it caches per‑host tokens and secret values, coalesces
concurrent reads, and handles authentication.
	2.	CLI Front‑Ends (opx, vaultx, baox):  These are lightweight
clients that users invoke from the command line.  opx is unchanged for
1Password; vaultx and baox are new wrappers that speak the same socket
protocol but expose different commands and default backends.
	3.	Configuration:  A JSON or YAML config file (e.g.
~/.op-authd/config.json) stores backend configuration such as Vault host,
namespace, login method and password references, TTL preferences, and user
approval settings.

Backend operation

The vault backend adds support for secrets defined by a URI scheme
vault:// or bao://.  For example:
