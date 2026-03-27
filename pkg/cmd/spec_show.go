// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-show-command.go --fake-name SpecShowCommand . SpecShowCommand

// SpecShowCommand executes the spec show subcommand.
type SpecShowCommand interface {
	Run(ctx context.Context, args []string) error
}

// specShowCommand implements SpecShowCommand.
type specShowCommand struct {
	inboxDir              string
	inProgressDir         string
	completedDir          string
	counter               prompt.Counter
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewSpecShowCommand creates a new SpecShowCommand.
func NewSpecShowCommand(
	inboxDir, inProgressDir, completedDir string,
	counter prompt.Counter,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) SpecShowCommand {
	return &specShowCommand{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		counter:               counter,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// SpecShowOutput holds all fields for JSON output.
type SpecShowOutput struct {
	File             string `json:"file"`
	Status           string `json:"status"`
	Approved         string `json:"approved,omitempty"`
	Prompted         string `json:"prompted,omitempty"`
	Verifying        string `json:"verifying,omitempty"`
	Completed        string `json:"completed,omitempty"`
	PromptsCompleted int    `json:"prompts_completed"`
	PromptsTotal     int    `json:"prompts_total"`
}

// Run executes the spec show command.
func (s *specShowCommand) Run(ctx context.Context, args []string) error {
	jsonOutput := false
	id := ""
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		} else if id == "" {
			id = arg
		}
	}

	if id == "" {
		return errors.Errorf(ctx, "spec identifier required")
	}

	path, err := FindSpecFileInDirs(ctx, id, s.inboxDir, s.inProgressDir, s.completedDir)
	if err != nil {
		return errors.Wrap(ctx, err, "find spec file")
	}

	sf, err := spec.Load(ctx, path, s.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	specName := sf.Name
	completed, total, err := s.counter.CountBySpec(ctx, specName)
	if err != nil {
		return errors.Wrap(ctx, err, "count prompts for spec")
	}

	out := SpecShowOutput{
		File:             filepath.Base(path),
		Status:           sf.Frontmatter.Status,
		Approved:         sf.Frontmatter.Approved,
		Prompted:         sf.Frontmatter.Prompted,
		Verifying:        sf.Frontmatter.Verifying,
		Completed:        sf.Frontmatter.Completed,
		PromptsCompleted: completed,
		PromptsTotal:     total,
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}

	fmt.Printf("File:    %s\n", out.File)
	fmt.Printf("Status:  %s\n", out.Status)
	if out.Approved != "" {
		fmt.Printf("Approved:  %s\n", out.Approved)
	}
	if out.Prompted != "" {
		fmt.Printf("Prompted:  %s\n", out.Prompted)
	}
	if out.Verifying != "" {
		fmt.Printf("Verifying: %s\n", out.Verifying)
	}
	if out.Completed != "" {
		fmt.Printf("Completed: %s\n", out.Completed)
	}
	fmt.Printf("Linked Prompts: %d/%d\n", out.PromptsCompleted, out.PromptsTotal)
	return nil
}
