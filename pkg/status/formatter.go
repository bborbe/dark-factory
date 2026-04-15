// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

//counterfeiter:generate -o ../../mocks/status-formatter.go --fake-name Formatter . Formatter

// Formatter formats status for display.
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

	// Project directory
	if st.ProjectDir != "" {
		fmt.Fprintf(&b, "  Project:    %s\n", st.ProjectDir)
	}

	// Container count (only when limit is configured OR the check was skipped)
	if st.ContainerMax > 0 {
		fmt.Fprintf(&b, "  Containers: %d/%d (system-wide)\n", st.ContainerCount, st.ContainerMax)
	} else if st.ContainerCountSkipped {
		b.WriteString("  Containers: (skipped — docker ps timed out)\n")
	}

	// Daemon status
	if st.DaemonPID > 0 {
		fmt.Fprintf(&b, "  Daemon:     %s (pid %d)\n", st.Daemon, st.DaemonPID)
	} else {
		fmt.Fprintf(&b, "  Daemon:     %s\n", st.Daemon)
	}

	// Current prompt
	if st.CurrentPrompt != "" {
		f.formatCurrentPrompt(&b, st)
	} else if st.GeneratingSpec != "" {
		f.formatGeneratingSpec(&b, st)
	} else {
		b.WriteString("  Current:    idle\n")
		if st.GeneratingSpecSkipped {
			b.WriteString("  (generating-spec check skipped — docker ps timed out)\n")
		}
	}

	// Queue
	if st.QueueCount > 0 {
		fmt.Fprintf(&b, "  Queue:      %d prompts\n", st.QueueCount)
		for _, p := range st.QueuedPrompts {
			fmt.Fprintf(&b, "    - %s\n", p)
		}
	} else {
		b.WriteString("  Queue:      0 prompts\n")
	}

	// Completed
	fmt.Fprintf(&b, "  Completed:  %d prompts\n", st.CompletedCount)

	// Last log
	if st.LastLogFile != "" {
		logInfo := st.LastLogFile
		if st.LastLogSize > 0 {
			logInfo += fmt.Sprintf(" (%s)", formatBytes(st.LastLogSize))
		}
		fmt.Fprintf(&b, "  Last log:   %s\n", logInfo)
	}

	// Warnings
	f.formatWarnings(&b, st)

	return b.String()
}

// formatWarnings formats the warnings section when git health issues are detected.
func (f *formatter) formatWarnings(b *strings.Builder, st *Status) {
	if !st.GitIndexLock && st.DirtyFileCount == 0 && !st.DirtyFileCheckSkipped {
		return
	}
	b.WriteString("  Warnings:\n")
	if st.GitIndexLock {
		b.WriteString("    \u26a0 .git/index.lock exists \u2014 daemon will skip prompts\n")
	}
	if st.DirtyFileCheckSkipped {
		b.WriteString("    \u26a0 dirty files: (skipped — git status timed out)\n")
	} else if st.DirtyFileCount > 0 {
		f.formatDirtyFileWarning(b, st)
	}
}

// formatDirtyFileWarning formats the dirty file count warning line.
func (f *formatter) formatDirtyFileWarning(b *strings.Builder, st *Status) {
	if st.DirtyFileThreshold > 0 {
		fmt.Fprintf(
			b,
			"    \u26a0 %d dirty files (threshold: %d)\n",
			st.DirtyFileCount,
			st.DirtyFileThreshold,
		)
	} else {
		fmt.Fprintf(b, "    \u26a0 %d dirty files\n", st.DirtyFileCount)
	}
}

// formatGeneratingSpec formats the spec generation section.
func (f *formatter) formatGeneratingSpec(b *strings.Builder, st *Status) {
	fmt.Fprintf(b, "  Current:    generating spec %s\n", st.GeneratingSpec)
	fmt.Fprintf(b, "  Container:  %s (running)\n", st.GeneratingContainer)
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
		switch {
		case st.ContainerRunningSkipped:
			containerStatus += " (status unknown — docker ps skipped)"
		case st.ContainerRunning:
			containerStatus += " (running)"
		default:
			containerStatus += " (not running)"
		}
		fmt.Fprintf(b, "  Container:  %s\n", containerStatus)
	}
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
		result += strconv.Itoa(h) + "h"
	}
	if m > 0 {
		result += strconv.Itoa(m) + "m"
	}
	if s > 0 {
		result += strconv.Itoa(s) + "s"
	}
	return result
}

// formatMS formats minutes:seconds.
func formatMS(m, s int) string {
	result := strconv.Itoa(m) + "m"
	if s > 0 {
		result += strconv.Itoa(s) + "s"
	}
	return result
}

// formatS formats seconds only.
func formatS(s int) string {
	return strconv.Itoa(s) + "s"
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
