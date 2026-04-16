// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package formatter implements a Go port of the claude-yolo Python v2 stream-json formatter.
// It reads newline-delimited Claude Code stream-json from an io.Reader, writes every raw line
// verbatim to a raw writer, and renders a human-readable formatted line to a formatted writer.
package formatter
