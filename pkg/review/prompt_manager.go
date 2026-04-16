// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package review

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/review-prompt-manager.go --fake-name ReviewPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the review package uses.
type PromptManager interface {
	ReadFrontmatter(ctx context.Context, path string) (*prompt.Frontmatter, error)
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	MoveToCompleted(ctx context.Context, path string) error
	SetStatus(ctx context.Context, path string, status string) error
	IncrementRetryCount(ctx context.Context, path string) error
}
