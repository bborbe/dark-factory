// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
)

var _ = Describe("ApplyGlobalOverrides Env merge", func() {
	// Regression: prior to the fix, ApplyGlobalOverrides handled
	// Model/HideGit/AutoRelease/... but NOT Env. Global env (typically
	// ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN for alt-provider configs
	// like minimax / self-hosted vLLM) was dropped on the floor when the
	// project config didn't already have those keys. Container then got
	// only ANTHROPIC_MODEL set explicitly by the executor/healthcheck
	// factory — claude tried Anthropic with the alt-provider model name,
	// rejected silently, exit 1, stdout/stderr empty. Surfaced via the
	// healthcheck claude probe after the entrypoint.sh + YOLO_OUTPUT=json
	// alignment exposed the underlying env gap.

	It("merges global Env underneath project Env (project wins on collision)", func() {
		cfg := config.Config{
			Env: map[string]string{
				"PROJECT_ONLY": "p1",
				"OVERLAP":      "from-project",
			},
		}
		global := globalconfig.GlobalConfig{
			Env: map[string]string{
				"GLOBAL_ONLY":          "g1",
				"OVERLAP":              "from-global",
				"ANTHROPIC_BASE_URL":   "https://api.minimax.io/anthropic",
				"ANTHROPIC_AUTH_TOKEN": "sk-x",
			},
		}
		config.ApplyGlobalOverrides(&cfg, global, config.LayeredProjectOverrides{})

		Expect(cfg.Env).To(HaveKeyWithValue("PROJECT_ONLY", "p1"))
		Expect(cfg.Env).To(HaveKeyWithValue("GLOBAL_ONLY", "g1"))
		Expect(cfg.Env).To(HaveKeyWithValue("OVERLAP", "from-project"))
		Expect(
			cfg.Env,
		).To(HaveKeyWithValue("ANTHROPIC_BASE_URL", "https://api.minimax.io/anthropic"))
		Expect(cfg.Env).To(HaveKeyWithValue("ANTHROPIC_AUTH_TOKEN", "sk-x"))
	})

	It("copies global Env when project Env is nil", func() {
		cfg := config.Config{Env: nil}
		global := globalconfig.GlobalConfig{
			Env: map[string]string{"ANTHROPIC_BASE_URL": "https://api.minimax.io/anthropic"},
		}
		config.ApplyGlobalOverrides(&cfg, global, config.LayeredProjectOverrides{})

		Expect(
			cfg.Env,
		).To(HaveKeyWithValue("ANTHROPIC_BASE_URL", "https://api.minimax.io/anthropic"))
	})

	It("strips reserved keys (ANTHROPIC_MODEL) from global Env during merge", func() {
		// Operators commonly mirror `model:` into `env: ANTHROPIC_MODEL:` in
		// their global config out of habit. validateEnv rejects
		// ANTHROPIC_MODEL in cfg.Env (it's set by factory from cfg.Model).
		// Without stripping, the daemon's post-layer Validate trips on the
		// merged-in duplicate. Stripping at merge time keeps user configs
		// working without changing the validation contract.
		cfg := config.Config{Env: map[string]string{}}
		global := globalconfig.GlobalConfig{
			Env: map[string]string{
				"ANTHROPIC_BASE_URL":   "https://api.minimax.io/anthropic",
				"ANTHROPIC_AUTH_TOKEN": "sk-x",
				"ANTHROPIC_MODEL":      "MiniMax-M3-highspeed",
				"YOLO_PROMPT_FILE":     "/tmp/should-be-stripped",
			},
		}
		config.ApplyGlobalOverrides(&cfg, global, config.LayeredProjectOverrides{})

		Expect(cfg.Env).To(HaveKeyWithValue("ANTHROPIC_BASE_URL", "https://api.minimax.io/anthropic"))
		Expect(cfg.Env).To(HaveKeyWithValue("ANTHROPIC_AUTH_TOKEN", "sk-x"))
		Expect(cfg.Env).NotTo(HaveKey("ANTHROPIC_MODEL"))
		Expect(cfg.Env).NotTo(HaveKey("YOLO_PROMPT_FILE"))
	})

	It("leaves project Env intact when global Env is empty", func() {
		cfg := config.Config{
			Env: map[string]string{"PROJECT_ONLY": "p1"},
		}
		global := globalconfig.GlobalConfig{}
		config.ApplyGlobalOverrides(&cfg, global, config.LayeredProjectOverrides{})

		Expect(cfg.Env).To(HaveLen(1))
		Expect(cfg.Env).To(HaveKeyWithValue("PROJECT_ONLY", "p1"))
	})
})
