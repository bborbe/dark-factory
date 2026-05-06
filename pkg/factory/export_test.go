// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"time"
)

// BuildIdleLoggerForTest exposes buildIdleLogger for unit testing.
var BuildIdleLoggerForTest = func(
	idleLogInterval time.Duration,
	queueInterval time.Duration,
	emit func(),
) func(context.Context, context.CancelFunc) {
	return buildIdleLogger(idleLogInterval, queueInterval, emit)
}
