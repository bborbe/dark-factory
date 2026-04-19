// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/preflight"
)

// SetPreflightChecker injects a preflight.Checker for internal processor tests.
func (p *processor) SetPreflightChecker(c preflight.Checker) {
	p.preflightChecker = c
}

// CheckPreflightConditions exposes checkPreflightConditions for internal tests.
func (p *processor) CheckPreflightConditions(ctx context.Context) (bool, error) {
	return p.checkPreflightConditions(ctx)
}

// ErrPreflightSkip exposes errPreflightSkip for tests to assert sentinel identity.
var ErrPreflightSkip = errPreflightSkip
