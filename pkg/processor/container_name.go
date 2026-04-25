// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import "regexp"

var sanitizeContainerNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// BaseName is the prompt filename without the .md extension and with special characters replaced.
type BaseName string

// String returns the underlying string value.
func (b BaseName) String() string { return string(b) }

// ContainerName is a sanitized Docker container name derived from project and prompt names.
type ContainerName string

// Sanitize replaces any character not in [a-zA-Z0-9_-] with '-'.
func (n ContainerName) Sanitize() ContainerName {
	return ContainerName(sanitizeContainerNameRegexp.ReplaceAllString(string(n), "-"))
}

// String returns the underlying string for use with exec / docker.
func (n ContainerName) String() string { return string(n) }
