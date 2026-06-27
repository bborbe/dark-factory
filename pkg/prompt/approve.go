// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
)

// ApproveManager is the minimum subset of Manager required by
// ApproveFromInbox — just Load. Each consumer package's own PromptManager
// interface (pkg/cmd, pkg/generator, ...) already includes Load, so the
// existing managers satisfy this implicitly with no new method needed.
type ApproveManager interface {
	Load(ctx context.Context, path string) (*PromptFile, error)
}

// ApproveFromInbox renames a prompt from the inbox dir to the queue dir
// (stripping any numeric prefix), loads the file, marks it approved, and
// saves it. Returns the new on-disk path so callers can log it or chain
// a post-approve step (typically NormalizeFilenames).
//
// Used by:
//   - pkg/cmd.approveCommand — CLI `dark-factory approve` after fuzzy match
//   - pkg/generator.dockerSpecGenerator — spec-generator auto-approve path
//
// Both call sites previously inlined this sequence; consolidated 2026-06-27
// as Phase-2 cleanup of [[Harden Dark Factory Architecture]]. The earlier
// generator.approvePromptFromInbox even admitted in its own comment:
// "Replicates the core of approveFromInbox in pkg/cmd/approve.go without
// the fuzzy search" — exactly the drift class the goal targets.
//
// NormalizeFilenames is intentionally NOT part of this primitive: the two
// callers diverge on how they handle its failure (cmd returns the error;
// generator demotes to a WARN log because auto-approve can tolerate a
// partial-rename state). Keeping that policy decision at the call sites
// avoids pushing a "tolerate normalize failures?" flag into the shared
// primitive's signature.
func ApproveFromInbox(
	ctx context.Context,
	inboxPath string,
	queueDir string,
	pm ApproveManager,
) (string, error) {
	filename := StripNumberPrefix(filepath.Base(inboxPath))
	newPath := filepath.Join(queueDir, filename)

	if err := os.Rename(inboxPath, newPath); err != nil {
		return "", errors.Wrap(ctx, err, "move file to queue")
	}

	pf, err := pm.Load(ctx, newPath)
	if err != nil {
		return "", errors.Wrap(ctx, err, "load prompt after move")
	}
	pf.MarkApproved()
	if err := pf.Save(ctx); err != nil {
		return "", errors.Wrap(ctx, err, "save approved prompt")
	}

	return newPath, nil
}
