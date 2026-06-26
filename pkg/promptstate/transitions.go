// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptstate

// stateTransitions is the single source of truth for allowed in-memory state moves.
// It is consistent with pkg/prompt.promptTransitions; add one row here to enable a
// new transition. Every State in AvailableStates MUST appear as a source key or as a
// sink in at least one row (enforced by TestTransitionTableCoversAllStates).
var stateTransitions = map[State][]State{
	StateApproved:            {StateExecuting, StateCancelled},
	StateExecuting:           {StateCommitting, StateCancelled, StateAborted},
	StateCommitting:          {StateCompleted},
	StateAborted:             {StateApproved}, // recovery: container gone → reset to approved
	StatePendingVerification: {StateCompleted},
	// StateCompleted and StateCancelled are terminal sinks: no outgoing rows.
}

// IsValidTransition reports whether moving from -> to is an allowed state transition.
// Transitions not in the table (including any involving StateUnknown) return false.
func IsValidTransition(from, to State) bool {
	for _, allowed := range stateTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
