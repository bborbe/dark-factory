// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// Status represents the current daemon status.
type Status struct {
	Daemon              string   `json:"daemon"`
	DaemonPID           int      `json:"daemon_pid,omitempty"`
	CurrentPrompt       string   `json:"current_prompt,omitempty"`
	ExecutingSince      string   `json:"executing_since,omitempty"`
	Container           string   `json:"container,omitempty"`
	ContainerRunning    bool     `json:"container_running,omitempty"`
	GeneratingSpec      string   `json:"generating_spec,omitempty"`
	GeneratingContainer string   `json:"generating_container,omitempty"`
	QueueCount          int      `json:"queue_count"`
	QueuedPrompts       []string `json:"queued_prompts"`
	CompletedCount      int      `json:"completed_count"`
	IdeasCount          int      `json:"ideas_count"`
	LastLogFile         string   `json:"last_log_file,omitempty"`
	LastLogSize         int64    `json:"last_log_size,omitempty"`
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

//counterfeiter:generate -o ../../mocks/status-checker.go --fake-name Checker . Checker

// Checker checks the current status of the dark-factory daemon.
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
	lockFilePath string
	serverPort   int
	promptMgr    prompt.Manager
}

// NewChecker creates a new Checker with additional options.
func NewChecker(
	queueDir string,
	completedDir string,
	ideasDir string,
	logDir string,
	lockFilePath string,
	serverPort int,
	promptMgr prompt.Manager,
) Checker {
	return &checker{
		queueDir:     queueDir,
		completedDir: completedDir,
		ideasDir:     ideasDir,
		logDir:       logDir,
		lockFilePath: lockFilePath,
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

	// Check for spec generation containers (only when no prompt is executing)
	if status.CurrentPrompt == "" {
		s.populateGeneratingSpec(ctx, status)
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
	sort.Slice(prompts, func(i, j int) bool {
		return prompts[i].completedTime.After(prompts[j].completedTime)
	})
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

		if fm.Status == string(prompt.ExecutingPromptStatus) {
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

// populateDaemonStatus checks if daemon is running via lock file PID.
func (s *checker) populateDaemonStatus(st *Status) {
	pid, err := s.readLockFilePID()
	if err != nil || pid <= 0 {
		return
	}
	if s.isProcessAlive(pid) {
		st.Daemon = "running"
		st.DaemonPID = pid
	}
}

// readLockFilePID reads the PID from the lock file.
func (s *checker) readLockFilePID() (int, error) {
	data, err := os.ReadFile(s.lockFilePath)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	return strconv.Atoi(pidStr)
}

// isProcessAlive checks if a process with the given PID is alive.
func (s *checker) isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
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

// isContainerRunning checks if a Docker container is running.
func (s *checker) isContainerRunning(containerName string) bool {
	if containerName == "" {
		return false
	}
	ctx := context.Background()
	// #nosec G204 -- containerName is derived from trusted frontmatter, not user input
	cmd := exec.CommandContext(
		ctx,
		"docker",
		"ps",
		"--filter",
		"name="+containerName,
		"--format",
		"{{.Names}}",
	)
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("docker ps failed", "container", containerName, "err", err)
		return false
	}
	return strings.Contains(string(out), containerName)
}

// populateGeneratingSpec checks for running spec generation containers and populates status.
func (s *checker) populateGeneratingSpec(ctx context.Context, st *Status) {
	const genPrefix = "dark-factory-gen-"
	// #nosec G204 -- filter value is a hardcoded prefix, not user input
	cmd := exec.CommandContext(
		ctx,
		"docker",
		"ps",
		"--filter",
		"name="+genPrefix,
		"--format",
		"{{.Names}}",
	)
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("docker ps for spec generation failed", "err", err)
		return
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return
	}
	// Take the first matching container line
	var containerName string
	for _, line := range bytes.Split([]byte(output), []byte("\n")) {
		name := strings.TrimSpace(string(line))
		if strings.HasPrefix(name, genPrefix) {
			containerName = name
			break
		}
	}
	if containerName == "" {
		return
	}
	specName := strings.TrimPrefix(containerName, genPrefix)
	st.GeneratingSpec = specName
	st.GeneratingContainer = containerName
}
