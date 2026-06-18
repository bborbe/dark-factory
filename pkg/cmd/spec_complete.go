// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-complete-command.go --fake-name SpecCompleteCommand . SpecCompleteCommand

// SpecCompleteCommand executes the spec complete subcommand.
type SpecCompleteCommand interface {
	Run(ctx context.Context, args []string) error
}

// specCompleteCommand implements SpecCompleteCommand.
type specCompleteCommand struct {
	inboxDir              string
	inProgressDir         string
	completedDir          string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	dirLockFactory        func(dirPath string) lock.DirLock
	lockTimeout           time.Duration
}

// NewSpecCompleteCommand creates a new SpecCompleteCommand.
func NewSpecCompleteCommand(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	dirLockFactory func(dirPath string) lock.DirLock,
	lockTimeout time.Duration,
) SpecCompleteCommand {
	if dirLockFactory == nil {
		dirLockFactory = lock.NewDirLock
	}
	if lockTimeout == 0 {
		lockTimeout = 5 * time.Second
	}
	return &specCompleteCommand{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		currentDateTimeGetter: currentDateTimeGetter,
		dirLockFactory:        dirLockFactory,
		lockTimeout:           lockTimeout,
	}
}

// Run executes the spec complete command.
func (s *specCompleteCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "spec identifier required")
	}

	id := args[0]
	// Search all three dirs — a verifying spec is in inProgressDir, but allow
	// searching all dirs for convenience
	path, err := FindSpecFileInDirs(ctx, id, s.inboxDir, s.inProgressDir, s.completedDir)
	if err != nil {
		return errors.Wrap(ctx, err, "find spec file")
	}

	fl := s.dirLockFactory(filepath.Dir(path))
	if err := fl.Acquire(ctx, s.lockTimeout); err != nil {
		return errors.Wrap(ctx, err, "acquire spec complete lock")
	}
	defer func() {
		if relErr := fl.Release(ctx); relErr != nil {
			slog.Warn(
				"spec complete: lock release failed",
				"dir",
				filepath.Dir(path),
				"error",
				relErr.Error(),
			)
		}
	}()

	sf, err := spec.Load(ctx, path, s.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	if err := spec.Status(sf.Frontmatter.Status).CanTransitionTo(ctx, spec.StatusCompleted); err != nil {
		return errors.Errorf(
			ctx,
			"spec is not in verifying state (current: %s)",
			sf.Frontmatter.Status,
		)
	}

	sf.MarkCompleted()
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec")
	}

	// Ensure completedDir exists
	if err := os.MkdirAll(s.completedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create completed dir")
	}

	// Move file to completedDir
	dest := filepath.Join(s.completedDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move spec to completed")
	}

	fmt.Printf("completed: %s\n", filepath.Base(dest))
	return nil
}
