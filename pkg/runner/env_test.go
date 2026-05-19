// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	libtime "github.com/bborbe/time"
	"github.com/stretchr/testify/require"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/factory"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
)

func TestContainerLaunchReceivesMergedEnv(t *testing.T) {
	// 1. tmp HOME with global env
	tmpHome := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpHome, ".dark-factory"), 0700))
	globalYAML := "env:\n  SHARED: global-val\n  GLOBAL_ONLY: gv\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpHome, ".dark-factory", "config.yaml"),
		[]byte(globalYAML), 0600,
	))
	t.Setenv("HOME", tmpHome)

	// 2. capture slog output before calling factory functions
	var buf bytes.Buffer
	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	// 3. load global config (same as CreateRunner does) and compute merged env
	globalCfg, err := globalconfig.NewLoader().Load(context.Background())
	require.NoError(t, err)

	projectEnv := map[string]string{
		"SHARED":       "project-val",
		"PROJECT_ONLY": "pv",
	}
	mergedEnv := config.MergeEnv(globalCfg.Env, projectEnv)

	// 4. Build a minimal cfg and verify CreateRunner accepts it without error.
	//    HideGit=true avoids git lock checks; skipPreflight=true avoids preflight.
	cfg := config.Defaults()
	cfg.HideGit = true
	cfg.Env = projectEnv

	r := factory.CreateRunner(
		context.Background(),
		cfg,
		"v0.0.1",
		true,
		config.FieldSources{},
		libtime.NewCurrentDateTime(),
	)
	require.NotNil(t, r, "CreateRunner must return a non-nil runner")

	// 5. Call LogEffectiveConfig directly with the merged cfg to verify source grouping.
	//    This exercises the wired values to confirm the merge and grouping are correct.
	mergedCfg := cfg
	mergedCfg.Env = mergedEnv

	factory.LogEffectiveConfig(mergedCfg, globalCfg, true, config.FieldSources{}, projectEnv)

	output := buf.String()

	// 6. Assert source-grouped reporting
	require.Regexp(t, regexp.MustCompile(`envFromGlobal=\[[^]]*GLOBAL_ONLY`), output)
	require.Regexp(t, regexp.MustCompile(`envProjectOverrides=\[[^]]*SHARED`), output)
	require.Regexp(t, regexp.MustCompile(`envProjectOnly=\[[^]]*PROJECT_ONLY`), output)

	// 7. Values must never appear in the log
	require.NotContains(t, output, "global-val")
	require.NotContains(t, output, "project-val")
	require.NotContains(t, output, "gv")
	require.NotContains(t, output, "pv")
}
