package audit

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/zach-source/opx/internal/security"
)

// AuditEvent represents a security audit event
type AuditEvent struct {
	Timestamp  time.Time         `json:"timestamp"`
	Event      string            `json:"event"`
	PeerInfo   security.PeerInfo `json:"peer_info"`
	Reference  string            `json:"reference,omitempty"`
	Decision   string            `json:"decision"`
	PolicyPath string            `json:"policy_path,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
}

// Logger handles audit event logging with rotation
type Logger struct {
	enabled bool
	roller  *Roller
}

// NewLogger creates a new audit logger with configurable rotation
func NewLogger(enabled bool) (*Logger, error) {
	return NewLoggerWithConfig(enabled, DefaultRollerConfig())
}

// NewLoggerWithConfig creates a new audit logger with custom rotation config
func NewLoggerWithConfig(enabled bool, config RollerConfig) (*Logger, error) {
	if !enabled {
		return &Logger{enabled: false}, nil
	}

	roller, err := NewRoller(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create log roller: %w", err)
	}

	return &Logger{
		enabled: true,
		roller:  roller,
	}, nil
}

// LogEvent records an audit event
func (l *Logger) LogEvent(event AuditEvent) {
	if !l.enabled {
		return
	}

	event.Timestamp = time.Now()

	// Log to structured audit file with rotation
	if l.roller != nil {
		data, err := json.Marshal(event)
		if err == nil {
			l.roller.Write(append(data, '\n'))
		}
	}

	// Also log to standard logger for immediate visibility
	log.Printf("[AUDIT] %s: %s (PID:%d Path:%s) -> %s: %s",
		event.Event,
		event.Decision,
		event.PeerInfo.PID,
		event.PeerInfo.Path,
		event.Reference,
		formatDetails(event.Details))
}

// LogAccessDecision records a policy access decision
func (l *Logger) LogAccessDecision(peerInfo security.PeerInfo, reference string, allowed bool, policyPath string, details map[string]string) {
	decision := "ALLOW"
	if !allowed {
		decision = "DENY"
	}

	event := AuditEvent{
		Event:      "ACCESS_DECISION",
		PeerInfo:   peerInfo,
		Reference:  reference,
		Decision:   decision,
		PolicyPath: policyPath,
		Details:    details,
	}

	l.LogEvent(event)
}

// LogSessionEvent records session-related security events
func (l *Logger) LogSessionEvent(eventType string, peerInfo security.PeerInfo, decision string, details map[string]string) {
	event := AuditEvent{
		Event:    eventType,
		PeerInfo: peerInfo,
		Decision: decision,
		Details:  details,
	}

	l.LogEvent(event)
}

// LogAuthenticationEvent records authentication attempts
func (l *Logger) LogAuthenticationEvent(peerInfo security.PeerInfo, success bool, reason string) {
	decision := "SUCCESS"
	details := map[string]string{"reason": reason}

	if !success {
		decision = "FAILURE"
	}

	event := AuditEvent{
		Event:    "AUTHENTICATION",
		PeerInfo: peerInfo,
		Decision: decision,
		Details:  details,
	}

	l.LogEvent(event)
}

// Close closes the audit logger
func (l *Logger) Close() error {
	if l.roller != nil {
		return l.roller.Close()
	}
	return nil
}

func formatDetails(details map[string]string) string {
	if len(details) == 0 {
		return ""
	}

	var result []string
	for k, v := range details {
		result = append(result, k+"="+v)
	}

	return "[" + strings.Join(result, ", ") + "]"
}
