// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/processor-prompt-manager.go --fake-name ProcessorPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the processor package uses.
type PromptManager interface {
	ListQueued(ctx context.Context) ([]prompt.Prompt, error)
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	AllPreviousCompleted(ctx context.Context, n int) bool
	FindMissingCompleted(ctx context.Context, n int) []int
	FindPromptStatusInProgress(ctx context.Context, number int) string
	SetStatus(ctx context.Context, path string, status string) error
	MoveToCompleted(ctx context.Context, path string) error
	HasQueuedPromptsOnBranch(ctx context.Context, branch string, excludePath string) (bool, error)
	SetPRURL(ctx context.Context, path string, url string) error
	FindCommitting(ctx context.Context) ([]string, error)
}
