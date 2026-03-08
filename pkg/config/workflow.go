// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"

	"github.com/bborbe/collection"
	"github.com/bborbe/errors"
	"github.com/bborbe/validation"
)

// Workflow defines how prompts are processed.
const (
	WorkflowDirect   Workflow = "direct"
	WorkflowPR       Workflow = "pr"
	WorkflowWorktree Workflow = "worktree"
)

// AvailableWorkflows contains all valid workflow values.
var AvailableWorkflows = Workflows{WorkflowDirect, WorkflowPR}

// Workflow is a string-based enum for workflow types.
type Workflow string

func (w Workflow) String() string {
	return string(w)
}

func (w Workflow) Validate(ctx context.Context) error {
	if w == WorkflowWorktree {
		return errors.Wrapf(ctx, validation.Error,
			"workflow 'worktree' removed — use 'pr' instead")
	}
	if !AvailableWorkflows.Contains(w) {
		return errors.Wrapf(ctx, validation.Error, "unknown workflow '%s'", w)
	}
	return nil
}

func (w Workflow) Ptr() *Workflow {
	return &w
}

// Workflows is a collection of Workflow values.
type Workflows []Workflow

func (w Workflows) Contains(workflow Workflow) bool {
	return collection.Contains(w, workflow)
}
