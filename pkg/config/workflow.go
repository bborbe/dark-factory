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

// Workflow defines how prompts are processed.
const (
	WorkflowDirect   Workflow = "direct"
	WorkflowBranch   Workflow = "branch"
	WorkflowWorktree Workflow = "worktree"
	WorkflowClone    Workflow = "clone"

	// WorkflowPR is the legacy enum value kept for parsing only.
	// The loader maps it to WorkflowClone + pr: true before validation.
	// Do not use this constant in new code.
	WorkflowPR Workflow = "pr"
)

// AvailableWorkflows contains the four valid workflow values for new configs.
// WorkflowPR ("pr") is intentionally excluded — it is legacy and mapped at load time.
var AvailableWorkflows = Workflows{WorkflowDirect, WorkflowBranch, WorkflowWorktree, WorkflowClone}

// Workflow is a string-based enum for workflow types.
type Workflow string

// String returns the string representation of the Workflow.
func (w Workflow) String() string {
	return string(w)
}

// Validate checks that the Workflow is a known value.
func (w Workflow) Validate(ctx context.Context) error {
	if !AvailableWorkflows.Contains(w) {
		validValues := make([]string, len(AvailableWorkflows))
		for i, v := range AvailableWorkflows {
			validValues[i] = string(v)
		}
		return errors.Wrapf(
			ctx,
			validation.Error,
			"unknown workflow %q, valid values: %s",
			w,
			strings.Join(validValues, ", "),
		)
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
