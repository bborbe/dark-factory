// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"context"
	"log/slog"
)

// contextKey is an unexported type for the logger context key.
// Using a private named type prevents collisions with keys from other packages.
type contextKey struct{}

// loggerKey is the sentinel value used to store/retrieve the bound *slog.Logger.
var loggerKey = contextKey{}

// NewContext returns a copy of ctx carrying logger. Downstream code retrieves it
// via From. Re-binding (e.g. after a container is assigned) is done by calling
// NewContext again with a logger derived via logger.With(...).
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// From returns the *slog.Logger bound to ctx by NewContext. When no logger is
// bound (boot path, tests, pre-bind hot-path lines), it returns slog.Default()
// — never nil. Callers can therefore always call log.From(ctx).Info(...) without
// a nil check. The fallback emits to whatever handler slog.SetDefault installed
// (see main.go / pkg/runner/runner.go).
func From(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}
