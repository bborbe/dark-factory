// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notifier

import (
	"context"
	"log/slog"
)

type multiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier returns a Notifier that fans out to all provided notifiers.
// If notifiers is empty, returns a no-op Notifier.
func NewMultiNotifier(notifiers ...Notifier) Notifier {
	if len(notifiers) == 0 {
		return &noopNotifier{}
	}
	return &multiNotifier{notifiers: notifiers}
}

func (m *multiNotifier) Notify(ctx context.Context, event Event) error {
	for _, n := range m.notifiers {
		if err := n.Notify(ctx, event); err != nil {
			slog.Warn("notification delivery failed", "eventType", event.EventType, "error", err)
		}
	}
	return nil
}
