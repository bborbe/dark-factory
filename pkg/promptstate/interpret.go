// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptstate

import (
	prompt "github.com/bborbe/dark-factory/pkg/prompt"
)

// Location is where the prompt file currently lives — the half-state discriminator.
type Location string

const (
	// LocationInProgress means the file lives in prompts/in-progress/.
	LocationInProgress Location = "in_progress"
	// LocationCompleted means the file lives in prompts/completed/.
	LocationCompleted Location = "completed"
)

// DockerState is the liveness of the prompt's container as reported by Docker.
type DockerState string

const (
	// DockerStateRunning means the container is alive and executing.
	DockerStateRunning DockerState = "running"
	// DockerStateStopped means the container has exited or does not exist.
	DockerStateStopped DockerState = "stopped"
	// DockerStateUnavailable means the Docker daemon is unreachable or the liveness probe errored.
	DockerStateUnavailable DockerState = "unavailable"
)

// InterpretTuple is the ONLY function in the codebase allowed to decide the
// authoritative current State from the four observable inputs. It is pure: same
// inputs always yield the same State; it has no shared mutable state and never
// blocks on docker. status is the raw on-disk frontmatter status string.
//
// Rules (each locked by a regression test in interpret_test.go):
//   - executing + container running         -> StateExecuting   (resume keeps it executing)
//   - executing + container gone (stopped)  -> StateAborted     (reset-to-approved path)
//   - executing + docker unavailable        -> StateExecuting   (refuse to coerce; file truth wins)
//   - committing + file in completed dir    -> StateCompleted   (location wins; PR #30 half-state)
//   - committing + file in in-progress dir  -> StateCommitting
//   - cancelled                              -> StateCancelled
//   - pending_verification                   -> StatePendingVerification
//   - approved                               -> StateApproved
//   - completed                              -> StateCompleted
//   - any unrecognised status string         -> StateUnknown
//
// Pre-execution states (idea, draft) and terminal-failure states (failed, in_review,
// rejected) are not in the seven canonical execution-lifecycle states and map to
// StateUnknown here — this is intentional, not a bug.
func InterpretTuple(
	location Location,
	status prompt.PromptStatus,
	container string,
	dockerState DockerState,
) State {
	_ = container // reserved: callers pass the frontmatter container name

	switch status {
	case prompt.ApprovedPromptStatus:
		return StateApproved
	case prompt.ExecutingPromptStatus:
		if dockerState == DockerStateStopped {
			return StateAborted
		}
		// DockerStateRunning or DockerStateUnavailable: refuse to coerce — file truth wins.
		return StateExecuting
	case prompt.CommittingPromptStatus:
		if location == LocationCompleted {
			// Half-state: file already moved to completed dir but status not yet updated (PR #30).
			// Location wins over the stale on-disk status.
			return StateCompleted
		}
		return StateCommitting
	case prompt.CompletedPromptStatus:
		return StateCompleted
	case prompt.CancelledPromptStatus:
		return StateCancelled
	case prompt.PendingVerificationPromptStatus:
		return StatePendingVerification
	default:
		// idea, draft, failed, in_review, rejected, and any future/unknown status
		// are outside the seven canonical execution-lifecycle states.
		return StateUnknown
	}
}

// InterpretRawTuple is InterpretTuple with the raw on-disk status string as input.
// Consumers pass pf.Frontmatter.Status directly; the PromptStatus conversion lives
// here so consumer files contain no prompt.PromptStatus token (spec AC-3 gate).
func InterpretRawTuple(
	location Location,
	rawStatus string,
	container string,
	dockerState DockerState,
) State {
	return InterpretTuple(location, prompt.PromptStatus(rawStatus), container, dockerState)
}

// IsPreExecutionStatus reports whether the raw status is a pre-execution status
// (idea/draft/approved). It preserves the queuescanner pre-lock gate semantics,
// which are broader than the seven canonical InterpretTuple states.
func IsPreExecutionStatus(rawStatus string) bool {
	return prompt.PromptStatus(rawStatus).IsPreExecution()
}

// StatusFromRaw converts a raw on-disk status string to a prompt.PromptStatus,
// keeping the conversion inside the allow-listed owner package.
func StatusFromRaw(rawStatus string) prompt.PromptStatus {
	return prompt.PromptStatus(rawStatus)
}
