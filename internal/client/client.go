package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/zach-source/opx/internal/protocol"
	"github.com/zach-source/opx/internal/util"
)

type Client struct {
	http  *http.Client
	base  string
	token string
	sock  string
}

func New() (*Client, error) {
	sock, err := util.SocketPath()
	if err != nil {
		return nil, err
	}
	tokPath, err := util.TokenPath()
	if err != nil {
		return nil, err
	}
	tok, _ := os.ReadFile(tokPath) // may not exist yet; daemon will create

	// Get TLS configuration for client
	tlsConfig, err := util.ClientTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to setup client TLS: %w", err)
	}

	tr := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			conn, err := d.DialContext(ctx, "unix", sock)
			if err != nil {
				return nil, err
			}
			// Wrap the Unix socket connection with TLS
			tlsConn := tls.Client(conn, tlsConfig)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, fmt.Errorf("TLS handshake failed: %w", err)
			}
			return tlsConn, nil
		},
	}
	return &Client{
		http:  &http.Client{Transport: tr, Timeout: 30 * time.Second},
		base:  "https://unix",
		token: string(tok),
		sock:  sock,
	}, nil
}

func (c *Client) ensureDaemon(ctx context.Context) error {
	// Try quick ping
	if err := c.Ping(ctx); err == nil {
		return nil
	}
	if os.Getenv("OPX_AUTOSTART") == "0" {
		return errors.New("daemon not reachable and autostart disabled (OPX_AUTOSTART=0)")
	}
	// Attempt to start: call opx-authd binary from configured path or PATH
	exe := getDaemonPath()
	if exe == "" {
		var err error
		exe, err = exec.LookPath("opx-authd")
		if err != nil {
			return fmt.Errorf("opx-authd not found in PATH: %w", err)
		}
	}
	cmd := exec.CommandContext(ctx, exe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch opx-authd: %w", err)
	}
	// Give it a moment
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Ping(ctx); err == nil {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("failed to connect to opx-authd after autostart")
}

func (c *Client) doJSON(ctx context.Context, method, path string, req any, resp any) error {
	var body *bytes.Reader
	if req != nil {
		b, _ := json.Marshal(req)
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}
	httpReq, _ := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if req != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		httpReq.Header.Set("X-OpAuthd-Token", c.token)
	}
	r, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode == 401 {
		return errors.New("unauthorized (token mismatch). Remove ~/.op-authd/token and restart daemon if needed")
	}
	if r.StatusCode >= 400 {
		b, _ := io.ReadAll(r.Body)
		return fmt.Errorf("server error: %s: %s", r.Status, string(b))
	}
	if resp != nil {
		return json.NewDecoder(r.Body).Decode(resp)
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.base+"/v1/status", nil)
	if c.token != "" {
		req.Header.Set("X-OpAuthd-Token", c.token)
	}
	r, err := c.http.Do(req)
	if err != nil {
		return err
	}
	r.Body.Close()
	if r.StatusCode == 401 {
		return errors.New("unauthorized")
	}
	if r.StatusCode >= 400 {
		return fmt.Errorf("status %s", r.Status)
	}
	return nil
}

// getDaemonPath returns the configured path to the opx-authd binary
func getDaemonPath() string {
	// Check environment variable first
	if path := os.Getenv("OPX_AUTHD_PATH"); path != "" {
		return path
	}

	// TODO: Could also check config file here if we add client-side config
	// configDir, err := util.ConfigDir()
	// if err == nil {
	//     configPath := filepath.Join(configDir, "client.json")
	//     // Load client config and check daemon_path field
	// }

	return "" // Use PATH lookup
}

func (c *Client) Read(ctx context.Context, ref string) (protocol.ReadResponse, error) {
	return c.ReadWithFlags(ctx, ref, nil)
}

func (c *Client) ReadWithFlags(ctx context.Context, ref string, flags []string) (protocol.ReadResponse, error) {
	var resp protocol.ReadResponse
	if err := c.doJSON(ctx, "POST", "/v1/read", protocol.ReadRequest{Ref: ref, Flags: flags}, &resp); err != nil {
		return protocol.ReadResponse{}, err
	}
	return resp, nil
}

func (c *Client) Reads(ctx context.Context, refs []string) (protocol.ReadsResponse, error) {
	return c.ReadsWithFlags(ctx, refs, nil)
}

func (c *Client) ReadsWithFlags(ctx context.Context, refs []string, flags []string) (protocol.ReadsResponse, error) {
	var resp protocol.ReadsResponse
	if err := c.doJSON(ctx, "POST", "/v1/reads", protocol.ReadsRequest{Refs: refs, Flags: flags}, &resp); err != nil {
		return protocol.ReadsResponse{}, err
	}
	return resp, nil
}

func (c *Client) Resolve(ctx context.Context, env map[string]string) (protocol.ResolveResponse, error) {
	return c.ResolveWithFlags(ctx, env, nil)
}

func (c *Client) ResolveWithFlags(ctx context.Context, env map[string]string, flags []string) (protocol.ResolveResponse, error) {
	var resp protocol.ResolveResponse
	if err := c.doJSON(ctx, "POST", "/v1/resolve", protocol.ResolveRequest{Env: env, Flags: flags}, &resp); err != nil {
		return protocol.ResolveResponse{}, err
	}
	return resp, nil
}

func (c *Client) EnsureReady(ctx context.Context) error {
	return c.ensureDaemon(ctx)
}
