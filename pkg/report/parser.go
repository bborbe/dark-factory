// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report

import (
	"context"
)

//counterfeiter:generate -o ../../mocks/report-parser.go --fake-name ReportParser . Parser

// Parser parses a completion report out of a prompt's execution log.
type Parser interface {
	// ParseFromLog reads the last N bytes of a log file and extracts the CompletionReport.
	// Returns nil if no report found (graceful — old prompts won't have one).
	ParseFromLog(ctx context.Context, logFile string) (*CompletionReport, error)
}

// NewParser creates a new Parser.
func NewParser() Parser {
	return &parser{}
}

type parser struct{}

// ParseFromLog reads the last N bytes of a log file and extracts the CompletionReport.
func (p *parser) ParseFromLog(ctx context.Context, logFile string) (*CompletionReport, error) {
	return ParseFromLog(ctx, logFile)
}
