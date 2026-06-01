// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import "context"

// fixVerifyingStale is informational only; no auto-fix is applied.
func (f *fixer) fixVerifyingStale(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, failed []FailedFix) {
	// CategoryVerifyingStale is always skipped; the detail message explains the manual action.
	return
}
