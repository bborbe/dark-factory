// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

// FieldSources records which config layer provided each of the layered user-pref fields.
// Valid values for each field are: "default", "global", "project", "arg".
// Zero value (empty string) is treated the same as "default" by callers.
type FieldSources struct {
	HideGit            string
	AutoRelease        string
	DirtyFileThreshold string
	Model              string
	MaxContainers      string
	Workflow           string
	PR                 string
	AutoMerge          string
}
