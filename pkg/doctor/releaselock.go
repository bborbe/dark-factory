// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"log/slog"

	"github.com/bborbe/dark-factory/pkg/lock"
)

// releaseLock releases fl and logs a warning on failure. Intended for use in
// defer statements where the caller cannot meaningfully propagate the error.
func releaseLock(ctx context.Context, fl lock.FileLock, path string) {
	if err := fl.Release(ctx); err != nil {
		slog.Warn("doctor: file lock release failed", "path", path, "error", err.Error())
	}
}
