// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

// Brancher handles git branch operations.
//
//counterfeiter:generate -o ../../mocks/brancher.go --fake-name Brancher . Brancher
type Brancher interface {
	CreateAndSwitch(ctx context.Context, name string) error
	Push(ctx context.Context, name string) error
	Switch(ctx context.Context, name string) error
	CurrentBranch(ctx context.Context) (string, error)
}

// brancher implements Brancher.
type brancher struct{}

// NewBrancher creates a new Brancher.
func NewBrancher() Brancher {
	return &brancher{}
}

// CreateAndSwitch creates a new branch and switches to it.
func (b *brancher) CreateAndSwitch(ctx context.Context, name string) error {
	// #nosec G204 -- branch name is derived from prompt filename and sanitized
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "create and switch to branch")
	}
	return nil
}

// Push pushes a branch to the remote repository.
func (b *brancher) Push(ctx context.Context, name string) error {
	// #nosec G204 -- branch name is derived from prompt filename and sanitized
	cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", name)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "push branch to remote")
	}
	return nil
}

// Switch switches to an existing branch.
func (b *brancher) Switch(ctx context.Context, name string) error {
	// #nosec G204 -- branch name is controlled by the application
	cmd := exec.CommandContext(ctx, "git", "checkout", name)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "switch to branch")
	}
	return nil
}

// CurrentBranch returns the name of the current branch.
func (b *brancher) CurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get current branch")
	}
	return strings.TrimSpace(string(output)), nil
}
