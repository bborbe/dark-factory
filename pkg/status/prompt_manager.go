// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/status-prompt-manager.go --fake-name StatusPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the status package uses.
type PromptManager interface {
	ListQueued(ctx context.Context) ([]prompt.Prompt, error)
	Title(ctx context.Context, path string) (string, error)
	ReadFrontmatter(ctx context.Context, path string) (*prompt.Frontmatter, error)
	HasExecuting(ctx context.Context) bool
	FindCommitting(ctx context.Context) ([]string, error)
}
