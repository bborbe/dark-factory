// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"
)

// AuditEntry records a single audit log event.
type AuditEntry struct {
	Timestamp   time.Time
	Category    Category
	Action      string // "applied", "skipped", "failed"
	TargetPaths []string
	Before      string
	After       string
}

// WriteAuditEntry appends one tab-separated line to the file at path.
// The directory containing path is created with mode 0755 if missing.
// The file is created with mode 0644 if missing and appended to otherwise.
func WriteAuditEntry(ctx context.Context, path string, entry AuditEntry) error {
	// Ensure directory exists.
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errors.Wrap(ctx, err, "create audit log directory")
		}
	}

	// Format: <rfc3339>\t<Category>\t<action>\t<targets>\t<before>\t<after>\n
	line := fmt.Sprintf(
		"%s\t%s\t%s\t%s\t%s\t%s\n",
		entry.Timestamp.Format(time.RFC3339),
		entry.Category,
		entry.Action,
		strings.Join(entry.TargetPaths, " "),
		entry.Before,
		entry.After,
	)

	// #nosec G304 -- path is operator-controlled audit log path
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return errors.Wrap(ctx, err, "open audit log")
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		return errors.Wrap(ctx, err, "write audit entry")
	}

	return nil
}
