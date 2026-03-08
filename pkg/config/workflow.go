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
	WorkflowDirect Workflow = "direct"
	WorkflowPR     Workflow = "pr"
)

// AvailableWorkflows contains all valid workflow values.
var AvailableWorkflows = Workflows{WorkflowDirect, WorkflowPR}

// Workflow is a string-based enum for workflow types.
type Workflow string

// String returns the string representation of the Workflow.
func (w Workflow) String() string {
	return string(w)
}

// Validate checks that the Workflow is a known value.
func (w Workflow) Validate(ctx context.Context) error {
	if w == "worktree" {
		return errors.Wrapf(ctx, validation.Error,
			"workflow 'worktree' removed — use 'pr' instead")
	}
	if !AvailableWorkflows.Contains(w) {
		return errors.Wrapf(ctx, validation.Error, "unknown workflow %q", w)
	}
	return nil
}

// Ptr returns a pointer to the Workflow value.
func (w Workflow) Ptr() *Workflow {
	return &w
}

// Workflows is a collection of Workflow values.
type Workflows []Workflow

func (w Workflows) Contains(workflow Workflow) bool {
	return collection.Contains(w, workflow)
}
