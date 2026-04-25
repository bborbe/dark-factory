// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slugmigrator

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/slugmigrator-prompt-manager.go --fake-name SlugMigratorPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the slugmigrator package uses.
type PromptManager interface {
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
}
