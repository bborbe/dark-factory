// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

// ProjectName is the name identifying a dark-factory project.
type ProjectName string

// String returns the underlying string value.
func (p ProjectName) String() string { return string(p) }

// MaxContainers is the maximum number of concurrently running containers allowed.
type MaxContainers int

// VerificationGate controls whether execution pauses for manual verification before git operations.
type VerificationGate bool

// DirtyFileThreshold is the maximum number of dirty (modified) files before execution is blocked.
type DirtyFileThreshold int

// AutoRetryLimit is the maximum number of automatic retries for a failed prompt (0 = disabled).
type AutoRetryLimit int

// Dirs groups the three prompt directory paths used by the processor.
type Dirs struct {
	Queue, Completed, Log string
}
