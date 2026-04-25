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
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-reject-command.go --fake-name SpecRejectCommand . SpecRejectCommand

// SpecRejectCommand executes the spec reject subcommand.
type SpecRejectCommand interface {
	Run(ctx context.Context, args []string) error
}

// specRejectCommand implements SpecRejectCommand.
type specRejectCommand struct {
	specsInboxDir         string
	specsInProgressDir    string
	specsRejectedDir      string
	promptsInboxDir       string
	promptsInProgressDir  string
	promptsCompletedDir   string
	promptsRejectedDir    string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewSpecRejectCommand creates a new SpecRejectCommand.
func NewSpecRejectCommand(
	specsInboxDir string,
	specsInProgressDir string,
	specsRejectedDir string,
	promptsInboxDir string,
	promptsInProgressDir string,
	promptsCompletedDir string,
	promptsRejectedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) SpecRejectCommand {
	return &specRejectCommand{
		specsInboxDir:         specsInboxDir,
		specsInProgressDir:    specsInProgressDir,
		specsRejectedDir:      specsRejectedDir,
		promptsInboxDir:       promptsInboxDir,
		promptsInProgressDir:  promptsInProgressDir,
		promptsCompletedDir:   promptsCompletedDir,
		promptsRejectedDir:    promptsRejectedDir,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Run executes the spec reject command.
func (s *specRejectCommand) Run(ctx context.Context, args []string) error {
	reason, remaining, err := parseReasonFlag(args)
	if err != nil {
		return errors.Errorf(ctx, "%v", err)
	}
	if len(remaining) == 0 {
		return errors.Errorf(ctx, "usage: dark-factory spec reject <name> --reason <text>")
	}
	return s.rejectSpec(ctx, remaining[0], reason)
}

func (s *specRejectCommand) rejectSpec(ctx context.Context, id, reason string) error {
	path, err := FindSpecFileInDirs(ctx, id, s.specsInboxDir, s.specsInProgressDir)
	if err != nil {
		return errors.Errorf(ctx, "spec not found: %s", id)
	}

	sf, err := spec.Load(ctx, path, s.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	status := spec.Status(sf.Frontmatter.Status)
	if status == spec.StatusRejected {
		return errors.Errorf(ctx, "%s is already rejected", filepath.Base(path))
	}
	if !status.IsRejectable() {
		return errors.Errorf(
			ctx,
			"cannot reject spec with status %q — rejectable states: idea, draft, approved, generating, prompted",
			sf.Frontmatter.Status,
		)
	}

	linkedPaths, err := s.findLinkedPrompts(ctx, sf.Name)
	if err != nil {
		return errors.Wrap(ctx, err, "find linked prompts")
	}

	if err := s.preflight(ctx, linkedPaths); err != nil {
		return err
	}

	for _, ppath := range linkedPaths {
		if err := s.rejectLinkedPrompt(ctx, ppath, reason); err != nil {
			return errors.Wrapf(ctx, err, "reject linked prompt %s", filepath.Base(ppath))
		}
	}

	sf.StampRejected(reason)
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec")
	}

	if err := os.MkdirAll(s.specsRejectedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create specs rejected dir")
	}

	dest := filepath.Join(s.specsRejectedDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move spec to rejected")
	}

	fmt.Printf("rejected: %s\n", filepath.Base(path))
	return nil
}

// findLinkedPrompts scans inbox, in-progress, and completed prompt directories
// for any prompt whose spec: frontmatter array references this spec by name.
// Prompts already in prompts/rejected/ are intentionally excluded.
func (s *specRejectCommand) findLinkedPrompts(
	ctx context.Context,
	specName string,
) ([]string, error) {
	var paths []string
	for _, dir := range []string{s.promptsInboxDir, s.promptsInProgressDir, s.promptsCompletedDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(ctx, err, "read prompt dir")
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			ppath := filepath.Join(dir, entry.Name())
			pf, err := prompt.Load(ctx, ppath, s.currentDateTimeGetter)
			if err != nil {
				continue
			}
			if pf.Frontmatter.HasSpec(specName) {
				paths = append(paths, ppath)
			}
		}
	}
	return paths, nil
}

// preflight verifies every linked prompt is in a rejectable state.
// Returns an error listing all offending prompts if any are not rejectable.
// No files are mutated by this method.
func (s *specRejectCommand) preflight(ctx context.Context, linkedPaths []string) error {
	var offenders []string
	for _, ppath := range linkedPaths {
		pf, err := prompt.Load(ctx, ppath, s.currentDateTimeGetter)
		if err != nil {
			return errors.Wrapf(ctx, err, "load prompt %s for preflight", filepath.Base(ppath))
		}
		status := prompt.PromptStatus(pf.Frontmatter.Status)
		if !status.IsRejectable() {
			offenders = append(
				offenders,
				fmt.Sprintf("%s (status: %s)", filepath.Base(ppath), pf.Frontmatter.Status),
			)
		}
	}
	if len(offenders) > 0 {
		return errors.Errorf(
			ctx,
			"cannot reject spec: linked prompts are not in a rejectable state:\n  %s\nCancel or wait for them to complete first",
			strings.Join(offenders, "\n  "),
		)
	}
	return nil
}

func (s *specRejectCommand) rejectLinkedPrompt(ctx context.Context, ppath, reason string) error {
	pf, err := prompt.Load(ctx, ppath, s.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.StampRejected(reason)
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	if err := os.MkdirAll(s.promptsRejectedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create prompts rejected dir")
	}

	dest := filepath.Join(s.promptsRejectedDir, filepath.Base(ppath))
	if err := os.Rename(ppath, dest); err != nil {
		return errors.Wrap(ctx, err, "move prompt to rejected")
	}

	fmt.Printf("  rejected prompt: %s\n", filepath.Base(ppath))
	return nil
}
