// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

// MergeEnv returns a new map with globalEnv as the base and projectEnv overlaid.
// For each key present in both, the project value wins.
// Keys present only in either source are preserved unchanged.
// Returns nil when both inputs are nil or empty.
func MergeEnv(globalEnv, projectEnv map[string]string) map[string]string {
	if len(globalEnv) == 0 && len(projectEnv) == 0 {
		return nil
	}
	merged := make(map[string]string, len(globalEnv)+len(projectEnv))
	for k, v := range globalEnv {
		merged[k] = v
	}
	for k, v := range projectEnv {
		merged[k] = v // project wins on collision
	}
	return merged
}
