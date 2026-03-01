// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/runner"
)

// CreateRunner creates a Runner that watches prompts/ and executes via Docker.
func CreateRunner(promptsDir string) runner.Runner {
	return runner.NewRunner(
		promptsDir,
		executor.NewDockerExecutor(),
	)
}
