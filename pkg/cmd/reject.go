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

	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/reject-command.go --fake-name RejectCommand . RejectCommand

// RejectCommand executes the prompt reject subcommand.
type RejectCommand interface {
	Run(ctx context.Context, args []string) error
}

// rejectCommand implements RejectCommand.
type rejectCommand struct {
	inboxDir        string
	inProgressDir   string
	rejectedDir     string
	promptManager   PromptManager
	fileLockFactory func(path string) lock.DirLock
	lockTimeout     time.Duration
}

// NewRejectCommand creates a new RejectCommand.
//
// fileLockFactory may be nil — it defaults to lock.NewDirLock. The factory is
// used to acquire a per-prompt lock before mutating the file, so a concurrent
// daemon advance (spec 092 AC "concurrent-reject-advance") cannot interleave
// a save/rename with our read. lockTimeout may be zero — it defaults to 5
// seconds; on timeout the reject returns a wrapped error so the operator sees
// the failure rather than a silent stall.
func NewRejectCommand(
	inboxDir string,
	inProgressDir string,
	rejectedDir string,
	promptManager PromptManager,
	fileLockFactory func(path string) lock.DirLock,
	lockTimeout time.Duration,
) RejectCommand {
	if fileLockFactory == nil {
		fileLockFactory = lock.NewDirLock
	}
	if lockTimeout == 0 {
		lockTimeout = 5 * time.Second
	}
	return &rejectCommand{
		inboxDir:        inboxDir,
		inProgressDir:   inProgressDir,
		rejectedDir:     rejectedDir,
		promptManager:   promptManager,
		fileLockFactory: fileLockFactory,
		lockTimeout:     lockTimeout,
	}
}

// Run executes the prompt reject command.
func (r *rejectCommand) Run(ctx context.Context, args []string) error {
	reason, remaining, err := parseReasonFlag(args)
	if err != nil {
		return errors.Errorf(ctx, "%v", err)
	}
	if len(remaining) == 0 {
		return errors.Errorf(ctx, "usage: dark-factory prompt reject <file> --reason <text>")
	}
	id := remaining[0]
	return r.rejectByID(ctx, id, reason)
}

func (r *rejectCommand) rejectByID(ctx context.Context, id, reason string) error {
	path, err := FindPromptFileInDirs(ctx, id, r.inboxDir, r.inProgressDir, r.rejectedDir)
	if err != nil {
		return errors.Errorf(ctx, "prompt not found: %s", id)
	}

	// Fast-fail BEFORE the lock: during execution the scanner holds the
	// per-prompt lock for the whole container run (minutes), so blocking
	// lockTimeout only to report "cannot reject executing" is a UX
	// regression. This read is advisory — the post-lock Load below
	// re-validates on authoritative state.
	if pre, preErr := r.promptManager.Load(ctx, path); preErr == nil {
		preStatus := prompt.PromptStatus(pre.Frontmatter.Status)
		if preStatus != prompt.RejectedPromptStatus &&
			!preStatus.IsRejectable() && preStatus != prompt.FailedPromptStatus {
			return errors.Errorf(
				ctx,
				"cannot reject prompt with status %q — allowed: idea, draft, approved, failed",
				pre.Frontmatter.Status,
			)
		}
	}

	// Acquire the status-directory lock BEFORE loading the file. The Load
	// below is the post-lock re-read: it observes whatever the daemon (or
	// another operator) wrote last, then we stamp + save + rename under
	// the same lock so a concurrent advance cannot interleave its own
	// save/rename on the same path (spec 092 AC "concurrent-reject-advance").
	fl := r.fileLockFactory(filepath.Dir(path))
	if err := fl.Acquire(ctx, r.lockTimeout); err != nil {
		return errors.Wrap(ctx, err, "acquire reject lock")
	}
	defer func() {
		if relErr := fl.Release(ctx); relErr != nil {
			slog.Warn(
				"reject: file lock release failed",
				"path",
				filepath.Base(path),
				"error",
				relErr.Error(),
			)
		}
	}()
	slog.Info("lock acquired", "file", filepath.Base(path))

	pf, err := r.promptManager.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	status := prompt.PromptStatus(pf.Frontmatter.Status)
	if status == prompt.RejectedPromptStatus {
		return errors.Errorf(ctx, "%s is already rejected", filepath.Base(path))
	}
	allowed := status.IsRejectable() || status == prompt.FailedPromptStatus
	if !allowed {
		return errors.Errorf(
			ctx,
			"cannot reject prompt with status %q — allowed: idea, draft, approved, failed",
			pf.Frontmatter.Status,
		)
	}

	pf.StampRejectedWithOriginal(reason, string(status))
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt")
	}

	if err := os.MkdirAll(r.rejectedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create rejected dir")
	}

	dest := filepath.Join(r.rejectedDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move prompt to rejected")
	}

	fmt.Printf("rejected: %s\n", filepath.Base(path))
	return nil
}

// parseReasonFlag extracts --reason <text> from args.
// Returns the reason string, remaining args (without --reason and its value), and an error if
// --reason is missing or has no value.
func parseReasonFlag(args []string) (string, []string, error) {
	var reason string
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--reason" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--reason requires a value")
			}
			reason = args[i+1]
			i++ // skip the value
			continue
		}
		remaining = append(remaining, args[i])
	}
	if reason == "" {
		return "", nil, fmt.Errorf("--reason is required")
	}
	return reason, remaining, nil
}
