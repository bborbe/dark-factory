// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/requeue-command.go --fake-name RequeueCommand . RequeueCommand

// RequeueCommand executes the requeue subcommand.
type RequeueCommand interface {
	Run(ctx context.Context, args []string) error
}

// requeueCommand implements RequeueCommand.
type requeueCommand struct {
	queueDir              string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewRequeueCommand creates a new RequeueCommand.
func NewRequeueCommand(
	queueDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) RequeueCommand {
	return &requeueCommand{
		queueDir:              queueDir,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Run executes the requeue command.
func (r *requeueCommand) Run(ctx context.Context, args []string) error {
	failedOnly := false
	var filename string

	for _, arg := range args {
		if arg == "--failed" {
			failedOnly = true
		} else if !strings.HasPrefix(arg, "--") {
			filename = arg
		}
	}

	if failedOnly {
		return r.requeueFailed(ctx)
	}

	if filename != "" {
		return r.requeueFile(ctx, filename)
	}

	return errors.Errorf(ctx, "usage: requeue <file> or requeue --failed")
}

// requeueFile requeues a specific file in the queue directory.
func (r *requeueCommand) requeueFile(ctx context.Context, id string) error {
	path, err := FindPromptFile(ctx, r.queueDir, id)
	if err != nil {
		return errors.Errorf(ctx, "file not found: %s", id)
	}

	pf, err := prompt.Load(ctx, path, r.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.MarkApproved()
	pf.Frontmatter.RetryCount = 0 // reset auto-retry budget on manual re-queue
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	fmt.Printf("requeued: %s\n", filepath.Base(path))
	return nil
}

// requeueFailed requeues all failed prompts in the queue directory.
func (r *requeueCommand) requeueFailed(ctx context.Context) error {
	entries, err := os.ReadDir(r.queueDir)
	if err != nil {
		return errors.Wrap(ctx, err, "read queue directory")
	}

	requeued := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(r.queueDir, entry.Name())
		pf, err := prompt.Load(ctx, path, r.currentDateTimeGetter)
		if err != nil {
			continue
		}

		if pf.Frontmatter.Status == string(prompt.FailedPromptStatus) ||
			pf.Frontmatter.Status == string(prompt.PermanentlyFailedPromptStatus) {
			pf.MarkApproved()
			pf.Frontmatter.RetryCount = 0 // reset auto-retry budget on manual re-queue
			if err := pf.Save(ctx); err != nil {
				return errors.Wrap(ctx, err, "save prompt")
			}
			fmt.Printf("requeued: %s\n", entry.Name())
			requeued++
		}
	}

	if requeued == 0 {
		fmt.Println("no failed prompts found")
	}
	return nil
}
