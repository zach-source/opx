package protocol

type ReadRequest struct {
	Ref   string   `json:"ref"`
	Flags []string `json:"flags,omitempty"`
}

type ReadsRequest struct {
	Refs  []string `json:"refs"`
	Flags []string `json:"flags,omitempty"`
}

type ReadResponse struct {
	Ref        string `json:"ref"`
	Value      string `json:"value"`
	FromCache  bool   `json:"from_cache"`
	ExpiresIn  int    `json:"expires_in_seconds"`
	ResolvedAt int64  `json:"resolved_at_unix"`
}

type ReadsResponse struct {
	Results map[string]ReadResponse `json:"results"`
}

type ResolveRequest struct {
	Env   map[string]string `json:"env"` // name -> ref
	Flags []string          `json:"flags,omitempty"`
}

type ResolveResponse struct {
	Env map[string]string `json:"env"` // name -> value
}

type Status struct {
	Backend    string         `json:"backend"`
	CacheSize  int            `json:"cache_size"`
	Hits       int64          `json:"hits"`
	Misses     int64          `json:"misses"`
	InFlight   int            `json:"in_flight"`
	TTLSeconds int            `json:"ttl_seconds"`
	SocketPath string         `json:"socket_path"`
	Session    *SessionStatus `json:"session,omitempty"`
}

type SessionStatus struct {
	State         string `json:"state"`
	IdleTimeout   int    `json:"idle_timeout_seconds"`
	TimeUntilLock int    `json:"time_until_lock_seconds"`
	Enabled       bool   `json:"enabled"`
}

type SessionUnlockRequest struct {
	// No fields needed - unlock is based on validating current CLI session
}

type SessionUnlockResponse struct {
	Success bool   `json:"success"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}
