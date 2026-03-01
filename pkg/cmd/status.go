// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/status"
)

//counterfeiter:generate -o ../../mocks/status-command.go --fake-name StatusCommand . StatusCommand

// StatusCommand executes the status subcommand.
type StatusCommand interface {
	Run(ctx context.Context, args []string) error
}

// statusCommand implements StatusCommand.
type statusCommand struct {
	checker   status.Checker
	formatter status.Formatter
}

// NewStatusCommand creates a new StatusCommand.
func NewStatusCommand(checker status.Checker, formatter status.Formatter) StatusCommand {
	return &statusCommand{
		checker:   checker,
		formatter: formatter,
	}
}

// Run executes the status command.
func (s *statusCommand) Run(ctx context.Context, args []string) error {
	// Check for --json flag
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			break
		}
	}

	// Get status
	st, err := s.checker.GetStatus(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get status")
	}

	// Output
	if jsonOutput {
		return s.outputJSON(st)
	}
	return s.outputHuman(st)
}

// outputJSON outputs status as JSON.
func (s *statusCommand) outputJSON(st *status.Status) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(st)
}

// outputHuman outputs status in human-readable format.
func (s *statusCommand) outputHuman(st *status.Status) error {
	output := s.formatter.Format(st)
	fmt.Print(output)
	return nil
}
