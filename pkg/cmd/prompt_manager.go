// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/cmd-prompt-manager.go --fake-name CmdPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the cmd package uses.
type PromptManager interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
	MoveToCompleted(ctx context.Context, path string) error
}
