// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"fmt"
	"strings"
)

// Formatter formats status for display.
//
//counterfeiter:generate -o ../../mocks/status-formatter.go --fake-name Formatter . Formatter
type Formatter interface {
	Format(st *Status) string
}

// formatter implements Formatter.
type formatter struct{}

// NewFormatter creates a new Formatter.
func NewFormatter() Formatter {
	return &formatter{}
}

// Format formats status in human-readable format.
func (f *formatter) Format(st *Status) string {
	var b strings.Builder

	b.WriteString("Dark Factory Status\n")

	// Daemon status
	if st.DaemonPID > 0 {
		b.WriteString(fmt.Sprintf("  Daemon:     %s (pid %d)\n", st.Daemon, st.DaemonPID))
	} else {
		b.WriteString(fmt.Sprintf("  Daemon:     %s\n", st.Daemon))
	}

	// Current prompt
	if st.CurrentPrompt == "" {
		b.WriteString("  Current:    idle\n")
	} else {
		f.formatCurrentPrompt(&b, st)
	}

	// Queue
	if st.QueueCount > 0 {
		b.WriteString(fmt.Sprintf("  Queue:      %d prompts\n", st.QueueCount))
		for _, p := range st.QueuedPrompts {
			b.WriteString(fmt.Sprintf("    - %s\n", p))
		}
	} else {
		b.WriteString("  Queue:      0 prompts\n")
	}

	// Completed
	b.WriteString(fmt.Sprintf("  Completed:  %d prompts\n", st.CompletedCount))

	// Ideas
	if st.IdeasCount > 0 {
		b.WriteString(fmt.Sprintf("  Ideas:      %d prompts\n", st.IdeasCount))
	}

	// Last log
	if st.LastLogFile != "" {
		logInfo := st.LastLogFile
		if st.LastLogSize > 0 {
			logInfo += fmt.Sprintf(" (%s)", formatBytes(st.LastLogSize))
		}
		b.WriteString(fmt.Sprintf("  Last log:   %s\n", logInfo))
	}

	return b.String()
}

// formatCurrentPrompt formats the current prompt section.
func (f *formatter) formatCurrentPrompt(b *strings.Builder, st *Status) {
	currentLine := fmt.Sprintf("  Current:    %s", st.CurrentPrompt)
	if st.ExecutingSince != "" {
		currentLine += fmt.Sprintf(" (executing since %s)", st.ExecutingSince)
	}
	b.WriteString(currentLine + "\n")

	// Container info
	if st.Container != "" {
		containerStatus := st.Container
		if st.ContainerRunning {
			containerStatus += " (running)"
		} else {
			containerStatus += " (not running)"
		}
		b.WriteString(fmt.Sprintf("  Container:  %s\n", containerStatus))
	}
}

// formatBytes formats bytes in human-readable format.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}

	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}

	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}
