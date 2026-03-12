// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// DetermineBumpFromChangelog reads CHANGELOG.md from the given directory and returns
// MinorBump if any ## Unreleased entry starts with "- feat:", PatchBump otherwise.
// Returns PatchBump when CHANGELOG.md is missing or has no ## Unreleased section.
func DetermineBumpFromChangelog(ctx context.Context, dir string) VersionBump {
	// #nosec G304 -- dir is a trusted application-controlled path, not user input
	content, err := os.ReadFile(filepath.Join(dir, "CHANGELOG.md"))
	if err != nil {
		return PatchBump
	}

	lines := strings.Split(string(content), "\n")
	inUnreleased := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## Unreleased") {
			inUnreleased = true
			continue
		}
		if inUnreleased && strings.HasPrefix(line, "##") {
			break
		}
		if inUnreleased && strings.HasPrefix(strings.TrimSpace(line), "- feat:") {
			return MinorBump
		}
	}
	return PatchBump
}
