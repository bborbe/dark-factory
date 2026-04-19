// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package preflight provides the baseline check that runs before each prompt execution.
// It verifies that the project's CI command passes on the clean main-branch tree,
// caching results per git commit SHA to avoid redundant container runs.
package preflight
