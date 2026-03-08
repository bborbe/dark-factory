// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package specnum provides spec number parsing utilities.
package specnum

import (
	"regexp"
	"strconv"
)

var numericPrefixRegexp = regexp.MustCompile(`^(\d+)`)

// Parse extracts the leading numeric value from a spec ID string.
// Handles bare numbers ("019" → 19), padded numbers ("0019" → 19),
// and full spec names ("019-review-fix-loop" → 19).
// Returns -1 if s has no numeric prefix.
func Parse(s string) int {
	matches := numericPrefixRegexp.FindStringSubmatch(s)
	if matches == nil {
		return -1
	}
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}
	return num
}
