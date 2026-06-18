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
	"strings"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-unapprove-command.go --fake-name SpecUnapproveCommand . SpecUnapproveCommand

// SpecUnapproveCommand executes the spec unapprove subcommand.
type SpecUnapproveCommand interface {
	Run(ctx context.Context, args []string) error
}

// specUnapproveCommand implements SpecUnapproveCommand.
type specUnapproveCommand struct {
	inboxDir              string
	inProgressDir         string
	promptsInboxDir       string
	promptsInProgressDir  string
	promptManager         PromptManager
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	dirLockFactory        func(dirPath string) lock.DirLock
	lockTimeout           time.Duration
}

// NewSpecUnapproveCommand creates a new SpecUnapproveCommand.
func NewSpecUnapproveCommand(
	inboxDir string,
	inProgressDir string,
	promptsInboxDir string,
	promptsInProgressDir string,
	promptManager PromptManager,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	dirLockFactory func(dirPath string) lock.DirLock,
	lockTimeout time.Duration,
) SpecUnapproveCommand {
	if dirLockFactory == nil {
		dirLockFactory = lock.NewDirLock
	}
	if lockTimeout == 0 {
		lockTimeout = 5 * time.Second
	}
	return &specUnapproveCommand{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		promptsInboxDir:       promptsInboxDir,
		promptsInProgressDir:  promptsInProgressDir,
		promptManager:         promptManager,
		currentDateTimeGetter: currentDateTimeGetter,
		dirLockFactory:        dirLockFactory,
		lockTimeout:           lockTimeout,
	}
}

// Run executes the spec unapprove command.
func (s *specUnapproveCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "spec identifier required")
	}

	id := args[0]
	path, err := FindSpecFile(ctx, s.inProgressDir, id)
	if err != nil {
		return errors.Wrap(ctx, err, "find spec file")
	}

	fl := s.dirLockFactory(filepath.Dir(path))
	if err := fl.Acquire(ctx, s.lockTimeout); err != nil {
		return errors.Wrap(ctx, err, "acquire spec unapprove lock")
	}
	defer func() {
		if relErr := fl.Release(ctx); relErr != nil {
			slog.Warn(
				"spec unapprove: lock release failed",
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

	if err := spec.Status(sf.Frontmatter.Status).CanTransitionTo(ctx, spec.StatusDraft); err != nil {
		return errors.Errorf(
			ctx,
			"cannot unapprove spec with status %q: only approved specs can be unapproved",
			sf.Frontmatter.Status,
		)
	}

	// Check for linked prompts before making any changes
	if err := s.checkLinkedPrompts(ctx, sf.Name); err != nil {
		return errors.Wrap(ctx, err, "check linked prompts")
	}

	specNum := sf.SpecNumber()

	// Clear approval metadata and reset status
	sf.Frontmatter.Status = string(spec.StatusDraft)
	sf.Frontmatter.Approved = ""
	sf.Frontmatter.Branch = ""

	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec")
	}

	// Strip numeric prefix for inbox filename
	strippedName := prompt.StripNumberPrefix(filepath.Base(path))
	dest := filepath.Join(s.inboxDir, strippedName)

	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move spec to inbox")
	}

	// Renumber remaining specs to close the gap
	if specNum >= 0 {
		if err := spec.RenumberSpecsAfterRemoval(ctx, s.inProgressDir, specNum); err != nil {
			return errors.Wrap(ctx, err, "renumber specs")
		}
	}

	fmt.Printf("unapproved: %s\n", strippedName)
	return nil
}

// checkLinkedPrompts returns an error if any prompt in the prompt dirs references this spec.
func (s *specUnapproveCommand) checkLinkedPrompts(ctx context.Context, specName string) error {
	for _, dir := range []string{s.promptsInboxDir, s.promptsInProgressDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return errors.Wrap(ctx, err, "read prompt dir")
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			ppath := filepath.Join(dir, entry.Name())
			pf, err := s.promptManager.Load(ctx, ppath)
			if err != nil {
				continue
			}
			if pf.Frontmatter.HasSpec(specName) {
				return errors.Errorf(
					ctx,
					"spec has linked prompts in %s: unapprove or remove those prompts first",
					dir,
				)
			}
		}
	}
	return nil
}
