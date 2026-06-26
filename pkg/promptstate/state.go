// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptstate

import (
	"context"

	"github.com/bborbe/collection"
	"github.com/bborbe/errors"
	"github.com/bborbe/validation"
)

// State is the in-memory authoritative state of a prompt, derived by InterpretTuple
// from the four observable inputs. It is DISTINCT from pkg/prompt.PromptStatus, which
// is the on-disk storage type; the 1:1 mapping between them lives in this package.
type State string

const (
	// StateApproved — prompt is queued, not yet executing.
	StateApproved State = "approved"
	// StateExecuting — prompt is running in a container (or being resumed into one).
	StateExecuting State = "executing"
	// StateCommitting — container succeeded; git commit pending.
	StateCommitting State = "committing"
	// StateCompleted — prompt finished and (re)located in the completed dir.
	StateCompleted State = "completed"
	// StateCancelled — prompt was cancelled before or during execution.
	StateCancelled State = "cancelled"
	// StatePendingVerification — prompt awaits post-review verification.
	StatePendingVerification State = "pending_verification"
	// StateAborted is the INTERPRETED state for "frontmatter says executing but the
	// container is gone" — the case the daemon resolves by resetting to approved.
	// It has NO on-disk status string; it is a transient/interpreted state only.
	// (See OPEN QUESTION in prompt context: there is no "aborted" frontmatter value.)
	StateAborted State = "aborted"
	// StateUnknown is the error-only sentinel returned by InterpretTuple when the
	// frontmatter status string is not one InterpretTuple recognises
	// (spec Failure Mode row 2). Callers log ERROR unknown_prompt_status and surface
	// the prompt as "unknown"; the daemon does NOT silently coerce.
	StateUnknown State = "unknown"
)

// States is a slice of State values.
type States []State

// AvailableStates lists every canonical State value the system accepts.
// StateUnknown is the error-only sentinel and is deliberately NOT in AvailableStates.
var AvailableStates = States{
	StateApproved,
	StateExecuting,
	StateCommitting,
	StateCompleted,
	StateCancelled,
	StatePendingVerification,
	StateAborted,
}

// String returns the string representation of the State.
func (s State) String() string {
	return string(s)
}

// Contains returns true if the given state is in the collection.
func (ss States) Contains(s State) bool {
	return collection.Contains(ss, s)
}

// Validate returns an error if s is not a canonical State value.
// StateUnknown is invalid per Validate — it is a sentinel, not a canonical state.
func (s State) Validate(ctx context.Context) error {
	if !AvailableStates.Contains(s) {
		return errors.Wrapf(ctx, validation.Error, "unknown state '%s'", s)
	}
	return nil
}
