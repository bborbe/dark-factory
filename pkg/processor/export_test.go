// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import "context"

// SetupInPlaceBranchForTest exposes branchWorkflowExecutor.setupInPlaceBranch for external tests.
// Placing this here (package processor) avoids an import cycle: the external test package
// imports mocks, which imports pkg/processor — importing mocks from within pkg/processor itself
// would create a forbidden cycle.
func SetupInPlaceBranchForTest(deps WorkflowDeps, ctx context.Context, branch string) error {
	e := &branchWorkflowExecutor{deps: deps}
	return e.setupInPlaceBranch(ctx, branch)
}
