// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/scenario"
)

//counterfeiter:generate -o ../../mocks/scenario-show-command.go --fake-name ScenarioShowCommand . ScenarioShowCommand

// ScenarioShowCommand executes the scenario show subcommand.
type ScenarioShowCommand interface {
	Run(ctx context.Context, args []string) error
}

// scenarioShowCommand implements ScenarioShowCommand.
type scenarioShowCommand struct {
	lister scenario.Lister
}

// NewScenarioShowCommand creates a new ScenarioShowCommand.
func NewScenarioShowCommand(lister scenario.Lister) ScenarioShowCommand {
	return &scenarioShowCommand{lister: lister}
}

// Run executes the scenario show command.
func (s *scenarioShowCommand) Run(ctx context.Context, args []string) error {
	id := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && id == "" {
			id = arg
		}
	}
	if id == "" {
		return errors.Errorf(ctx, "scenario identifier required")
	}

	matches, err := s.lister.Find(ctx, id)
	if err != nil {
		return errors.Wrap(ctx, err, "find scenario")
	}

	switch len(matches) {
	case 0:
		return errors.Errorf(ctx, "no scenario matching %q", id)
	case 1:
		_, err := os.Stdout.Write(matches[0].RawContent)
		return err
	default:
		fmt.Fprintf(os.Stderr, "scenario %q matches multiple files:\n", id)
		for _, sf := range matches {
			fmt.Fprintf(os.Stderr, "  %s\n", sf.Name+".md")
		}
		return errors.Errorf(ctx, "ambiguous scenario identifier %q: %d matches", id, len(matches))
	}
}
