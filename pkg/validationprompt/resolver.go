// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package validationprompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/validationprompt-resolver.go --fake-name Resolver . Resolver

// Resolver resolves a validationPrompt config value into criteria text.
//   - empty value          → ("", false, nil)
//   - file path (exists)   → (file contents, true, nil)
//   - path-shaped, missing → ("", false, nil) — log warning at caller
//   - inline text          → (value, true, nil)
//   - file read error      → ("", false, err)
type Resolver interface {
	Resolve(ctx context.Context, value string) (string, bool, error)
}

// NewResolver creates a filesystem-backed Resolver.
func NewResolver() Resolver {
	return &resolver{}
}

type resolver struct{}

// Resolve resolves the validationPrompt config value into criteria text.
func (r *resolver) Resolve(ctx context.Context, value string) (string, bool, error) {
	if value == "" {
		return "", false, nil
	}
	if _, err := os.Stat(value); err == nil {
		data, readErr := os.ReadFile(
			value,
		) // #nosec G304 -- path is validated by config (no absolute path, no .. traversal)
		if readErr != nil {
			return "", false, errors.Wrap(ctx, readErr, "read validationPrompt file")
		}
		return string(data), true, nil
	}
	// path-shaped but missing — caller logs warning
	if strings.Contains(value, string(filepath.Separator)) || strings.HasSuffix(value, ".md") {
		return "", false, nil
	}
	return value, true, nil
}
