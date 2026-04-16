// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/runner-prompt-manager.go --fake-name RunnerPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the runner package uses.
type PromptManager interface {
	NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	ListQueued(ctx context.Context) ([]prompt.Prompt, error)
}
