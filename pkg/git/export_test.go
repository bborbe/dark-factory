// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

// DecideMergeActionForTest exposes the internal decideMergeAction function for external tests.
func DecideMergeActionForTest(mergeStateStatus string) (shouldMerge bool, err error) {
	return decideMergeAction(mergeStateStatus)
}
