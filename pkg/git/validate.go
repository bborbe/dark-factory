// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"fmt"
	"regexp"
)

var branchNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_.-]*$`)

// ValidateBranchName returns an error if the branch name contains characters
// that could be used for argument injection in git commands.
func ValidateBranchName(name string) error {
	if !branchNameRegexp.MatchString(name) {
		return fmt.Errorf(
			"invalid branch name %q: must start with alphanumeric and contain only [a-zA-Z0-9/_.-]",
			name,
		)
	}
	return nil
}

// ValidatePRTitle returns an error if the PR title is empty or starts with a dash,
// which could be interpreted as a flag by the gh CLI.
func ValidatePRTitle(title string) error {
	if title == "" {
		return fmt.Errorf("invalid PR title: must not be empty")
	}
	if title[0] == '-' {
		return fmt.Errorf("invalid PR title: must not start with a dash")
	}
	return nil
}
