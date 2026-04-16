// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package watcher

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/watcher-prompt-manager.go --fake-name WatcherPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the watcher package uses.
type PromptManager interface {
	NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
}
