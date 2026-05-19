// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvMerge(t *testing.T) {
	cases := []struct {
		name     string
		global   map[string]string
		project  map[string]string
		expected map[string]string
	}{
		{
			name:     "global_only",
			global:   map[string]string{"A": "1", "B": "2"},
			project:  nil,
			expected: map[string]string{"A": "1", "B": "2"},
		},
		{
			name:     "project_only",
			global:   nil,
			project:  map[string]string{"X": "10", "Y": "20"},
			expected: map[string]string{"X": "10", "Y": "20"},
		},
		{
			name:     "overlap_project_wins",
			global:   map[string]string{"SHARED": "global", "G_ONLY": "g"},
			project:  map[string]string{"SHARED": "project", "P_ONLY": "p"},
			expected: map[string]string{"SHARED": "project", "G_ONLY": "g", "P_ONLY": "p"},
		},
		{
			name:     "disjoint_union",
			global:   map[string]string{"A": "1"},
			project:  map[string]string{"B": "2"},
			expected: map[string]string{"A": "1", "B": "2"},
		},
		{
			name:     "both_nil_returns_nil",
			global:   nil,
			project:  nil,
			expected: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeEnv(tc.global, tc.project)
			require.Equal(t, tc.expected, got)
		})
	}
}
