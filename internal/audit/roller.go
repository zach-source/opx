package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zach-source/opx/internal/util"
)

// RollerConfig configures audit log rotation behavior
type RollerConfig struct {
	MaxDays       int           `json:"max_days"`        // Maximum days to keep logs (0 = keep all)
	CompressOld   bool          `json:"compress_old"`    // Whether to compress old log files
	RotateOnStart bool          `json:"rotate_on_start"` // Whether to rotate logs on startup
	FlushInterval time.Duration `json:"-"`               // How often to flush logs to disk
}

// DefaultRollerConfig returns sensible defaults for log rotation
func DefaultRollerConfig() RollerConfig {
	return RollerConfig{
		MaxDays:       30,              // Keep 30 days of logs
		CompressOld:   false,           // Don't compress for now
		RotateOnStart: true,            // Rotate on startup
		FlushInterval: 5 * time.Second, // Flush every 5 seconds
	}
}

// Roller manages audit log rotation and retention
type Roller struct {
	config      RollerConfig
	baseDir     string
	currentFile *os.File
	currentDate string
	mu          sync.Mutex
	flushTimer  *time.Timer
}

// NewRoller creates a new audit log roller
func NewRoller(config RollerConfig) (*Roller, error) {
	dataDir, err := util.DataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get data directory: %w", err)
	}

	roller := &Roller{
		config:  config,
		baseDir: dataDir,
	}

	// Rotate on startup if configured
	if config.RotateOnStart {
		if err := roller.rotateIfNeeded(); err != nil {
			return nil, fmt.Errorf("failed initial log rotation: %w", err)
		}
	}

	// Start flush timer
	if config.FlushInterval > 0 {
		roller.flushTimer = time.AfterFunc(config.FlushInterval, roller.scheduleFlush)
	}

	return roller, nil
}

// Write writes data to the current log file, rotating if necessary
func (r *Roller) Write(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if we need to rotate (new day)
	if err := r.rotateIfNeeded(); err != nil {
		return fmt.Errorf("log rotation failed: %w", err)
	}

	// Write to current file
	if r.currentFile == nil {
		return fmt.Errorf("no current log file available")
	}

	_, err := r.currentFile.Write(data)
	return err
}

// rotateIfNeeded rotates the log if we're on a new day
func (r *Roller) rotateIfNeeded() error {
	currentDate := time.Now().Format("2006-01-02")

	// If we're still on the same day and have a file, nothing to do
	if currentDate == r.currentDate && r.currentFile != nil {
		return nil
	}

	// Close current file if open
	if r.currentFile != nil {
		r.currentFile.Close()
		r.currentFile = nil
	}

	// Open new log file for current date
	logPath := filepath.Join(r.baseDir, fmt.Sprintf("audit-%s.log", currentDate))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}

	r.currentFile = file
	r.currentDate = currentDate

	// Clean up old log files if retention is configured
	if r.config.MaxDays > 0 {
		go r.cleanupOldLogs() // Run in background to avoid blocking
	}

	return nil
}

// cleanupOldLogs removes log files older than MaxDays
func (r *Roller) cleanupOldLogs() {
	cutoffDate := time.Now().AddDate(0, 0, -r.config.MaxDays)

	// Find all audit log files
	files, err := filepath.Glob(filepath.Join(r.baseDir, "audit-*.log"))
	if err != nil {
		return // Silently fail cleanup
	}

	for _, file := range files {
		// Extract date from filename (audit-2006-01-02.log)
		base := filepath.Base(file)
		if !strings.HasPrefix(base, "audit-") || !strings.HasSuffix(base, ".log") {
			continue
		}

		// Extract date part
		dateStr := strings.TrimSuffix(strings.TrimPrefix(base, "audit-"), ".log")
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue // Skip files with invalid date format
		}

		// Remove if older than cutoff
		if fileDate.Before(cutoffDate) {
			os.Remove(file) // Ignore errors
		}
	}
}

// scheduleFlush flushes the current log file and reschedules
func (r *Roller) scheduleFlush() {
	r.mu.Lock()
	if r.currentFile != nil {
		r.currentFile.Sync()
	}
	r.mu.Unlock()

	// Reschedule if interval is configured
	if r.config.FlushInterval > 0 {
		r.flushTimer = time.AfterFunc(r.config.FlushInterval, r.scheduleFlush)
	}
}

// Close closes the current log file and stops the flush timer
func (r *Roller) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.flushTimer != nil {
		r.flushTimer.Stop()
		r.flushTimer = nil
	}

	if r.currentFile != nil {
		err := r.currentFile.Close()
		r.currentFile = nil
		return err
	}

	return nil
}

// GetCurrentLogPath returns the path to the current log file
func (r *Roller) GetCurrentLogPath() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentDate == "" {
		r.currentDate = time.Now().Format("2006-01-02")
	}

	return filepath.Join(r.baseDir, fmt.Sprintf("audit-%s.log", r.currentDate))
}

// ListLogFiles returns all available audit log files sorted by date (newest first)
func (r *Roller) ListLogFiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(r.baseDir, "audit-*.log"))
	if err != nil {
		return nil, err
	}

	// Sort by filename (which contains date) in reverse order (newest first)
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	return files, nil
}

// GetLogForDate returns the log file path for a specific date
func (r *Roller) GetLogForDate(date time.Time) string {
	dateStr := date.Format("2006-01-02")
	return filepath.Join(r.baseDir, fmt.Sprintf("audit-%s.log", dateStr))
}
