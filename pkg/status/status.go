// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// Status represents the current daemon status.
type Status struct {
	Daemon           string   `json:"daemon"`
	DaemonPID        int      `json:"daemon_pid,omitempty"`
	CurrentPrompt    string   `json:"current_prompt,omitempty"`
	ExecutingSince   string   `json:"executing_since,omitempty"`
	Container        string   `json:"container,omitempty"`
	ContainerRunning bool     `json:"container_running,omitempty"`
	QueueCount       int      `json:"queue_count"`
	QueuedPrompts    []string `json:"queued_prompts"`
	CompletedCount   int      `json:"completed_count"`
	IdeasCount       int      `json:"ideas_count"`
	LastLogFile      string   `json:"last_log_file,omitempty"`
	LastLogSize      int64    `json:"last_log_size,omitempty"`
}

// QueuedPrompt represents a prompt in the queue with metadata.
type QueuedPrompt struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Size  int64  `json:"size"`
}

// CompletedPrompt represents a completed prompt with metadata.
type CompletedPrompt struct {
	Name        string    `json:"name"`
	CompletedAt time.Time `json:"completed_at"`
}

// Checker checks the current status of the dark-factory daemon.
//
//counterfeiter:generate -o ../../mocks/status-checker.go --fake-name Checker . Checker
type Checker interface {
	GetStatus(ctx context.Context) (*Status, error)
	GetQueuedPrompts(ctx context.Context) ([]QueuedPrompt, error)
	GetCompletedPrompts(ctx context.Context, limit int) ([]CompletedPrompt, error)
}

// checker implements Checker.
type checker struct {
	queueDir     string
	completedDir string
	ideasDir     string
	logDir       string
	serverPort   int
	promptMgr    prompt.Manager
}

// NewChecker creates a new Checker.
func NewChecker(
	queueDir string,
	completedDir string,
	ideasDir string,
	promptMgr prompt.Manager,
) Checker {
	return &checker{
		queueDir:     queueDir,
		completedDir: completedDir,
		ideasDir:     ideasDir,
		logDir:       "prompts/log",
		serverPort:   8080,
		promptMgr:    promptMgr,
	}
}

// NewCheckerWithOptions creates a new Checker with additional options.
func NewCheckerWithOptions(
	queueDir string,
	completedDir string,
	ideasDir string,
	logDir string,
	serverPort int,
	promptMgr prompt.Manager,
) Checker {
	return &checker{
		queueDir:     queueDir,
		completedDir: completedDir,
		ideasDir:     ideasDir,
		logDir:       logDir,
		serverPort:   serverPort,
		promptMgr:    promptMgr,
	}
}

// GetStatus returns the current daemon status.
func (s *checker) GetStatus(ctx context.Context) (*Status, error) {
	status := &Status{
		Daemon:        "not running",
		QueuedPrompts: []string{},
	}

	// Check if daemon is running
	s.populateDaemonStatus(status)

	// Check for executing prompt
	if err := s.populateExecutingPrompt(ctx, status); err != nil {
		return nil, errors.Wrap(ctx, err, "populate executing prompt")
	}

	// Count queued prompts
	queued, err := s.promptMgr.ListQueued(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "list queued prompts")
	}

	for _, p := range queued {
		status.QueuedPrompts = append(status.QueuedPrompts, filepath.Base(p.Path))
	}
	status.QueueCount = len(queued)

	// Count completed prompts
	completedCount, err := s.countMarkdownFiles(s.completedDir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "count completed prompts")
	}
	status.CompletedCount = completedCount

	// Count ideas (if ideas directory exists)
	ideasCount, err := s.countMarkdownFiles(s.ideasDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(ctx, err, "count ideas")
	}
	status.IdeasCount = ideasCount

	// Find latest log file
	if err := s.populateLogInfo(ctx, status); err != nil {
		return nil, errors.Wrap(ctx, err, "populate log info")
	}

	return status, nil
}

// GetQueuedPrompts returns detailed information about queued prompts.
func (s *checker) GetQueuedPrompts(ctx context.Context) ([]QueuedPrompt, error) {
	queued, err := s.promptMgr.ListQueued(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "list queued prompts")
	}

	result := make([]QueuedPrompt, 0, len(queued))
	for _, p := range queued {
		title, err := s.promptMgr.Title(ctx, p.Path)
		if err != nil {
			title = filepath.Base(p.Path)
		}

		info, err := os.Stat(p.Path)
		size := int64(0)
		if err == nil {
			size = info.Size()
		}

		result = append(result, QueuedPrompt{
			Name:  filepath.Base(p.Path),
			Title: title,
			Size:  size,
		})
	}

	return result, nil
}

// GetCompletedPrompts returns recent completed prompts.
func (s *checker) GetCompletedPrompts(
	ctx context.Context,
	limit int,
) ([]CompletedPrompt, error) {
	entries, err := os.ReadDir(s.completedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []CompletedPrompt{}, nil
		}
		return nil, errors.Wrap(ctx, err, "read completed directory")
	}

	prompts := s.collectCompletedPrompts(ctx, entries)
	sortPromptsByTimeDescending(prompts)
	prompts = applyLimit(prompts, limit)

	return convertToCompletedPrompts(prompts), nil
}

// promptWithTime holds prompt name and completion time.
type promptWithTime struct {
	name          string
	completedTime time.Time
}

// collectCompletedPrompts collects completed prompts with their completion times.
func (s *checker) collectCompletedPrompts(
	ctx context.Context,
	entries []os.DirEntry,
) []promptWithTime {
	prompts := make([]promptWithTime, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		completedTime := s.getCompletionTime(ctx, entry)
		if completedTime.IsZero() {
			continue
		}

		prompts = append(prompts, promptWithTime{
			name:          entry.Name(),
			completedTime: completedTime,
		})
	}
	return prompts
}

// getCompletionTime extracts completion time from frontmatter or file mod time.
func (s *checker) getCompletionTime(ctx context.Context, entry os.DirEntry) time.Time {
	path := filepath.Join(s.completedDir, entry.Name())
	fm, err := s.promptMgr.ReadFrontmatter(ctx, path)

	// Try frontmatter timestamp first
	if err == nil && fm != nil && fm.Completed != "" {
		if t, err := time.Parse(time.RFC3339, fm.Completed); err == nil {
			return t
		}
	}

	// Fall back to file mod time
	info, err := entry.Info()
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// sortPromptsByTimeDescending sorts prompts by completion time (most recent first).
func sortPromptsByTimeDescending(prompts []promptWithTime) {
	for i := 0; i < len(prompts)-1; i++ {
		for j := i + 1; j < len(prompts); j++ {
			if prompts[j].completedTime.After(prompts[i].completedTime) {
				prompts[i], prompts[j] = prompts[j], prompts[i]
			}
		}
	}
}

// applyLimit limits the number of prompts.
func applyLimit(prompts []promptWithTime, limit int) []promptWithTime {
	if limit > 0 && len(prompts) > limit {
		return prompts[:limit]
	}
	return prompts
}

// convertToCompletedPrompts converts promptWithTime to CompletedPrompt.
func convertToCompletedPrompts(prompts []promptWithTime) []CompletedPrompt {
	result := make([]CompletedPrompt, len(prompts))
	for i, p := range prompts {
		result[i] = CompletedPrompt{
			Name:        p.name,
			CompletedAt: p.completedTime,
		}
	}
	return result
}

// executingPrompt contains info about the executing prompt.
type executingPrompt struct {
	Path        string
	Container   string
	StartedTime time.Time
}

// findExecutingPrompt finds the currently executing prompt.
func (s *checker) findExecutingPrompt(ctx context.Context) (*executingPrompt, error) {
	entries, err := os.ReadDir(s.queueDir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read queue directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(s.queueDir, entry.Name())
		fm, err := s.promptMgr.ReadFrontmatter(ctx, path)
		if err != nil {
			continue
		}

		if fm.Status == string(prompt.StatusExecuting) {
			startedTime := time.Time{}
			if fm.Started != "" {
				// Parse the started timestamp from frontmatter
				if t, err := time.Parse(time.RFC3339, fm.Started); err == nil {
					startedTime = t
				}
			}

			return &executingPrompt{
				Path:        path,
				Container:   fm.Container,
				StartedTime: startedTime,
			}, nil
		}
	}

	return nil, nil
}

// countMarkdownFiles counts .md files in a directory.
func (s *checker) countMarkdownFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			count++
		}
	}

	return count, nil
}

// formatDuration formats a duration in a human-readable format.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	hours := d / time.Hour
	d -= hours * time.Hour

	minutes := d / time.Minute
	d -= minutes * time.Minute

	seconds := d / time.Second

	if hours > 0 {
		return formatTime(int(hours), int(minutes), int(seconds))
	}
	if minutes > 0 {
		return formatTime(0, int(minutes), int(seconds))
	}
	return formatTime(0, 0, int(seconds))
}

// formatTime formats hours, minutes, and seconds.
func formatTime(h, m, s int) string {
	if h > 0 {
		return formatHMS(h, m, s)
	}
	if m > 0 {
		return formatMS(m, s)
	}
	return formatS(s)
}

// formatHMS formats hours:minutes:seconds.
func formatHMS(h, m, s int) string {
	result := ""
	if h > 0 {
		result += formatInt(h) + "h"
	}
	if m > 0 {
		result += formatInt(m) + "m"
	}
	if s > 0 {
		result += formatInt(s) + "s"
	}
	return result
}

// formatMS formats minutes:seconds.
func formatMS(m, s int) string {
	result := formatInt(m) + "m"
	if s > 0 {
		result += formatInt(s) + "s"
	}
	return result
}

// formatS formats seconds only.
func formatS(s int) string {
	return formatInt(s) + "s"
}

// formatInt converts int to string.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}

// populateExecutingPrompt populates executing prompt info in the status.
func (s *checker) populateExecutingPrompt(ctx context.Context, st *Status) error {
	if !s.promptMgr.HasExecuting(ctx) {
		return nil
	}

	executing, err := s.findExecutingPrompt(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "find executing prompt")
	}

	if executing == nil {
		return nil
	}

	st.CurrentPrompt = filepath.Base(executing.Path)
	st.Container = executing.Container

	if !executing.StartedTime.IsZero() {
		duration := time.Since(executing.StartedTime)
		st.ExecutingSince = formatDuration(duration)
	}

	// Check if container is running
	st.ContainerRunning = s.isContainerRunning(executing.Container)

	return nil
}

// populateDaemonStatus checks if daemon is running.
func (s *checker) populateDaemonStatus(st *Status) {
	// Check if server port is listening
	if s.isDaemonRunning() {
		st.Daemon = "running"
		// TODO: get PID from daemon when available
	}
}

// populateLogInfo finds the latest log file.
func (s *checker) populateLogInfo(ctx context.Context, st *Status) error {
	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(ctx, err, "read log directory")
	}

	var latestLog string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if latestLog == "" || info.ModTime().After(latestTime) {
			latestLog = entry.Name()
			latestTime = info.ModTime()
		}
	}

	if latestLog != "" {
		logPath := filepath.Join(s.logDir, latestLog)
		st.LastLogFile = logPath

		info, err := os.Stat(logPath)
		if err == nil {
			st.LastLogSize = info.Size()
		}
	}

	return nil
}

// isDaemonRunning checks if the daemon is running by attempting to connect to the server port.
func (s *checker) isDaemonRunning() bool {
	// Try to connect to the server port
	// For now, we check if the port is open by attempting a TCP connection
	// This is a simple check - in production you might want to check a PID file or use a health endpoint
	return false // TODO: implement port check or PID file check
}

// isContainerRunning checks if a Docker container is running.
//
//nolint:unparam // Will be implemented in future iteration
func (s *checker) isContainerRunning(containerName string) bool {
	if containerName == "" {
		return false
	}
	// TODO: implement docker ps check
	return false
}
