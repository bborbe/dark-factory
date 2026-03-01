// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/bborbe/errors"
)

// SemanticVersionNumber represents a parsed semantic version.
type SemanticVersionNumber struct {
	Major int
	Minor int
	Patch int
}

// ParseSemanticVersionNumber parses "vX.Y.Z" into a SemanticVersionNumber.
// Returns error if format is invalid.
func ParseSemanticVersionNumber(ctx context.Context, tag string) (SemanticVersionNumber, error) {
	// Match vX.Y.Z
	re := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(tag)
	if matches == nil {
		return SemanticVersionNumber{}, errors.Errorf(ctx, "invalid version tag: %s", tag)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return SemanticVersionNumber{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// String returns the "vX.Y.Z" representation.
func (v SemanticVersionNumber) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// BumpPatch returns a new version with patch incremented.
func (v SemanticVersionNumber) BumpPatch() SemanticVersionNumber {
	return SemanticVersionNumber{
		Major: v.Major,
		Minor: v.Minor,
		Patch: v.Patch + 1,
	}
}

// BumpMinor returns a new version with minor incremented and patch reset to 0.
func (v SemanticVersionNumber) BumpMinor() SemanticVersionNumber {
	return SemanticVersionNumber{
		Major: v.Major,
		Minor: v.Minor + 1,
		Patch: 0,
	}
}

// Less returns true if v is lower than other.
func (v SemanticVersionNumber) Less(other SemanticVersionNumber) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}
