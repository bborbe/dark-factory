// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package globalconfig_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bborbe/dark-factory/pkg/globalconfig"
)

func TestEnv(t *testing.T) {
	setupHome := func(t *testing.T, configYAML string, perm os.FileMode) string {
		t.Helper()
		tmpHome := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpHome, ".config", "dark-factory"), 0700))
		cfgPath := filepath.Join(tmpHome, ".config", "dark-factory", "config.yaml")
		require.NoError(t, os.WriteFile(cfgPath, []byte(configYAML), perm))
		t.Setenv("HOME", tmpHome)
		return tmpHome
	}

	t.Run("valid_env_key_loads", func(t *testing.T) {
		setupHome(t, "env:\n  API_KEY: value\n", 0600)
		cfg, err := globalconfig.NewLoader().Load(context.Background())
		require.NoError(t, err)
		require.Equal(t, "value", cfg.Env["API_KEY"])
	})

	t.Run("invalid_lowercase_key_rejected", func(t *testing.T) {
		setupHome(t, "env:\n  lowercase_key: value\n", 0600)
		_, err := globalconfig.NewLoader().Load(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "lowercase_key")
		require.NotContains(t, err.Error(), "value") // values never leak into errors
	})

	t.Run("invalid_leading_digit_rejected", func(t *testing.T) {
		setupHome(t, "env:\n  1BADKEY: secret\n", 0600)
		_, err := globalconfig.NewLoader().Load(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "1BADKEY")
		require.NotContains(t, err.Error(), "secret")
	})

	t.Run("invalid_special_chars_rejected", func(t *testing.T) {
		setupHome(t, "env:\n  BAD-KEY: value\n", 0600)
		_, err := globalconfig.NewLoader().Load(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "BAD-KEY")
	})

	t.Run("world_readable_perm_warns_but_loads", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX perms only")
		}
		var buf bytes.Buffer
		origLogger := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
		t.Cleanup(func() { slog.SetDefault(origLogger) })

		tmpHome := setupHome(t, "env:\n  API_KEY: value\n", 0644)
		cfg, err := globalconfig.NewLoader().Load(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, cfg.Env, "config must still load")

		output := buf.String()
		require.Contains(
			t,
			output,
			filepath.Join(tmpHome, ".config", "dark-factory", "config.yaml"),
		)
		require.Contains(t, output, "chmod 600")
	})
}
