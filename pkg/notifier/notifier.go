// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notifier

import "context"

// Event holds the data for a single notification.
type Event struct {
	ProjectName string
	EventType   string // "prompt_failed", "prompt_permanently_failed", "prompt_partial", "spec_verifying", "review_limit", "stuck_container"
	PromptName  string // filename without path, empty if not applicable
	PRURL       string // empty if not applicable
}

//counterfeiter:generate -o ../../mocks/notifier.go --fake-name Notifier . Notifier

// Notifier sends a notification for a lifecycle event.
type Notifier interface {
	Notify(ctx context.Context, event Event) error
}

type noopNotifier struct{}

func (n *noopNotifier) Notify(_ context.Context, _ Event) error { return nil }
