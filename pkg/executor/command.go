// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os/exec"
)

// commandRunner runs an external command.
type commandRunner interface {
	Run(ctx context.Context, cmd *exec.Cmd) error
}

// defaultCommandRunner runs commands directly via cmd.Run().
type defaultCommandRunner struct{}

func (r *defaultCommandRunner) Run(ctx context.Context, cmd *exec.Cmd) error {
	return cmd.Run()
}
