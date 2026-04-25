// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generator

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/generator-prompt-manager.go --fake-name GeneratorPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the generator package uses.
type PromptManager interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
}
