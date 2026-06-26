// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log provides context-aware structured logging helpers for dark-factory.
// It allows a *slog.Logger to be bound to a context.Context (via NewContext) and
// retrieved from it (via From), falling back to slog.Default() when no logger is bound.
package log
