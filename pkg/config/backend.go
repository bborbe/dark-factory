// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"strings"

	"github.com/bborbe/collection"
	"github.com/bborbe/errors"
	"github.com/bborbe/validation"
)

const (
	BackendDocker Backend = "docker"
	BackendLocal  Backend = "local"
)

// AvailableBackends contains the two valid backend values.
var AvailableBackends = Backends{BackendDocker, BackendLocal}

// Backend selects how LLM steps (prompt execution and generation) are launched.
type Backend string

// String returns the string representation of the Backend.
func (b Backend) String() string {
	return string(b)
}

// Validate checks that the Backend is a known value.
func (b Backend) Validate(ctx context.Context) error {
	// Empty string is valid — means the field was not set in yaml (Backend has omitempty).
	if b == "" {
		return nil
	}
	if !AvailableBackends.Contains(b) {
		validValues := make([]string, len(AvailableBackends))
		for i, v := range AvailableBackends {
			validValues[i] = string(v)
		}
		return errors.Wrapf(
			ctx,
			validation.Error,
			"unknown backend %q, valid values: %s",
			b,
			strings.Join(validValues, ", "),
		)
	}
	return nil
}

// Ptr returns a pointer to the Backend value.
func (b Backend) Ptr() *Backend {
	return &b
}

// Backends is a collection of Backend values.
type Backends []Backend

func (b Backends) Contains(backend Backend) bool {
	return collection.Contains(b, backend)
}
