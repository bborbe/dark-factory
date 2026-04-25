// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/spec-prompt-manager.go --fake-name SpecPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the spec package uses.
type PromptManager interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
}
