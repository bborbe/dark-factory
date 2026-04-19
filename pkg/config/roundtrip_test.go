// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package config provides internal tests that need access to unexported types
// such as partialConfig. External tests live in package config_test.
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// exclusions lists Config fields that are intentionally absent from partialConfig,
// each with a short reason. Nested structs and collections have their own merge helpers;
// special-cased fields are handled by the Load step logic.
var exclusions = map[string]string{
	// Nested structs — own merge helpers (mergePartialPrompts, mergePartialSpecs, mergePartialProviders)
	"Prompts":       "nested struct, own merge helper mergePartialPrompts",
	"Specs":         "nested struct, own merge helper mergePartialSpecs",
	"GitHub":        "nested struct, merged wholesale via mergePartialProviders",
	"Bitbucket":     "nested struct, merged wholesale via mergePartialProviders",
	"Notifications": "nested struct, merged wholesale via mergePartialProviders",
	// Collection fields
	"Env":              "map[string]string collection, merged as-is without pointer indirection",
	"ExtraMounts":      "slice collection, merged as-is without pointer indirection",
	"AllowedReviewers": "slice collection, merged as-is without pointer indirection",
	// Legacy / special Load step handling
	"Worktree": "zeroed unconditionally in Load step D; legacy field",
	"Workflow": "legacy pr-enum mapping applied in Load step A; not a plain assignment",
	// Validation-coupled fields — round-trip covered by paired-yaml tests below
	"PR":         "validation-coupled: workflow: direct + pr: true is invalid; covered by paired-yaml test",
	"AutoMerge":  "validation-coupled: requires pr: true; covered by paired-yaml test",
	"AutoReview": "validation-coupled: requires pr+autoMerge+allowedReviewers; covered by paired-yaml test",
}

var _ = Describe("Config/partialConfig parity", func() {
	// Parity test: every non-excluded Config field must have a counterpart in partialConfig.
	// This guards against adding a Config field without threading it through partialConfig
	// and the merge helpers — the root cause of the silent PreflightCommand/PreflightInterval drop.
	//
	// Note: nested structs (GitHubConfig, BitbucketConfig, NotificationsConfig, PromptsConfig,
	// SpecsConfig) are excluded here because they have dedicated partial structs and merge
	// helpers. Their internal parity is a separate concern.
	Describe("field parity", func() {
		It("every non-excluded Config field has a matching field in partialConfig", func() {
			configType := reflect.TypeOf(Config{})
			partialType := reflect.TypeOf(partialConfig{})

			// Build a set of partialConfig field names for O(1) lookup.
			partialFields := make(map[string]struct{}, partialType.NumField())
			for i := 0; i < partialType.NumField(); i++ {
				partialFields[partialType.Field(i).Name] = struct{}{}
			}

			var missing []string
			for i := 0; i < configType.NumField(); i++ {
				name := configType.Field(i).Name
				if _, excluded := exclusions[name]; excluded {
					continue
				}
				if _, found := partialFields[name]; !found {
					missing = append(missing, name)
				}
			}
			Expect(missing).To(BeEmpty(),
				"Config fields missing from partialConfig (add field + merge branch, or add to exclusions with reason): %v",
				missing,
			)
		})
	})

	// Round-trip test: for each scalar yaml key, write a minimal yaml file with a sentinel
	// value and assert it survives Load unchanged.
	Describe("isolated scalar round-trip", func() {
		var ctx context.Context
		BeforeEach(func() {
			ctx = context.Background()
		})

		writeAndLoad := func(yamlContent string) (Config, error) {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, ".dark-factory.yaml")
			// #nosec G306 -- test file, not a secret
			if err := os.WriteFile(path, []byte(yamlContent), 0600); err != nil {
				return Config{}, err
			}
			return (&fileLoader{configPath: path}).Load(ctx)
		}

		DescribeTable(
			"field round-trips through yaml load",
			func(yamlKey string, yamlValue string, check func(Config)) {
				yaml := fmt.Sprintf("%s: %s\n", yamlKey, yamlValue)
				cfg, err := writeAndLoad(yaml)
				Expect(err).NotTo(HaveOccurred(), "yaml: %s", yaml)
				check(cfg)
			},
			// String fields
			Entry("projectName", "projectName", "sentinel-projectName",
				func(cfg Config) { Expect(cfg.ProjectName).To(Equal("sentinel-projectName")) }),
			Entry("defaultBranch", "defaultBranch", "sentinel-defaultBranch",
				func(cfg Config) { Expect(cfg.DefaultBranch).To(Equal("sentinel-defaultBranch")) }),
			Entry(
				"containerImage",
				"containerImage",
				"sentinel-containerImage",
				func(cfg Config) { Expect(cfg.ContainerImage).To(Equal("sentinel-containerImage")) },
			),
			Entry("model", "model", "sentinel-model",
				func(cfg Config) { Expect(cfg.Model).To(Equal("sentinel-model")) }),
			Entry(
				"validationCommand",
				"validationCommand",
				"sentinel-validationCommand",
				func(cfg Config) { Expect(cfg.ValidationCommand).To(Equal("sentinel-validationCommand")) },
			),
			Entry("validationPrompt", "validationPrompt", "relative/prompt.md",
				func(cfg Config) { Expect(cfg.ValidationPrompt).To(Equal("relative/prompt.md")) }),
			Entry("testCommand", "testCommand", "sentinel-testCommand",
				func(cfg Config) { Expect(cfg.TestCommand).To(Equal("sentinel-testCommand")) }),
			Entry("claudeDir", "claudeDir", "sentinel-claudeDir",
				func(cfg Config) { Expect(cfg.ClaudeDir).To(Equal("sentinel-claudeDir")) }),
			Entry(
				"generateCommand",
				"generateCommand",
				"sentinel-generateCommand",
				func(cfg Config) { Expect(cfg.GenerateCommand).To(Equal("sentinel-generateCommand")) },
			),
			Entry(
				"additionalInstructions",
				"additionalInstructions",
				"sentinel-additionalInstructions",
				func(cfg Config) {
					Expect(cfg.AdditionalInstructions).To(Equal("sentinel-additionalInstructions"))
				},
			),
			Entry(
				"preflightCommand",
				"preflightCommand",
				"sentinel-preflightCommand",
				func(cfg Config) { Expect(cfg.PreflightCommand).To(Equal("sentinel-preflightCommand")) },
			),
			// Duration strings — must be valid time.ParseDuration values (not "sentinel-X")
			Entry("maxPromptDuration", "maxPromptDuration", "7h",
				func(cfg Config) { Expect(cfg.MaxPromptDuration).To(Equal("7h")) }),
			Entry("preflightInterval", "preflightInterval", "3h",
				func(cfg Config) { Expect(cfg.PreflightInterval).To(Equal("3h")) }),
			// Int fields
			Entry("debounceMs", "debounceMs", "42",
				func(cfg Config) { Expect(cfg.DebounceMs).To(Equal(42)) }),
			Entry("serverPort", "serverPort", "8888",
				func(cfg Config) { Expect(cfg.ServerPort).To(Equal(8888)) }),
			Entry("maxReviewRetries", "maxReviewRetries", "9",
				func(cfg Config) { Expect(cfg.MaxReviewRetries).To(Equal(9)) }),
			Entry("pollIntervalSec", "pollIntervalSec", "99",
				func(cfg Config) { Expect(cfg.PollIntervalSec).To(Equal(99)) }),
			Entry("maxContainers", "maxContainers", "5",
				func(cfg Config) { Expect(cfg.MaxContainers).To(Equal(5)) }),
			Entry("dirtyFileThreshold", "dirtyFileThreshold", "7",
				func(cfg Config) { Expect(cfg.DirtyFileThreshold).To(Equal(7)) }),
			Entry("autoRetryLimit", "autoRetryLimit", "4",
				func(cfg Config) { Expect(cfg.AutoRetryLimit).To(Equal(4)) }),
			// Bool fields (flipped from defaults)
			Entry("autoRelease", "autoRelease", "true",
				func(cfg Config) { Expect(cfg.AutoRelease).To(BeTrue()) }),
			Entry("verificationGate", "verificationGate", "true",
				func(cfg Config) { Expect(cfg.VerificationGate).To(BeTrue()) }),
			Entry("useCollaborators", "useCollaborators", "true",
				func(cfg Config) { Expect(cfg.UseCollaborators).To(BeTrue()) }),
			Entry("hideGit", "hideGit", "true",
				func(cfg Config) { Expect(cfg.HideGit).To(BeTrue()) }),
		)
	})

	// Paired-yaml round-trip for validation-coupled fields that cannot be tested in isolation.
	Describe("paired-yaml round-trip for validation-coupled fields", func() {
		var ctx context.Context
		BeforeEach(func() {
			ctx = context.Background()
		})

		writeAndLoad := func(yamlContent string) (Config, error) {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, ".dark-factory.yaml")
			// #nosec G306 -- test file, not a secret
			if err := os.WriteFile(path, []byte(yamlContent), 0600); err != nil {
				return Config{}, err
			}
			return (&fileLoader{configPath: path}).Load(ctx)
		}

		It("pr: true round-trips when paired with workflow: clone", func() {
			cfg, err := writeAndLoad("workflow: clone\npr: true\n")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.PR).To(BeTrue())
		})

		It("autoMerge: true round-trips when paired with workflow: clone and pr: true", func() {
			cfg, err := writeAndLoad("workflow: clone\npr: true\nautoMerge: true\n")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AutoMerge).To(BeTrue())
		})

		It("autoReview: true round-trips when all required fields are present", func() {
			yaml := "workflow: clone\npr: true\nautoMerge: true\nautoReview: true\nuseCollaborators: true\n"
			cfg, err := writeAndLoad(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AutoReview).To(BeTrue())
		})

		// provider: bitbucket-server requires bitbucket.baseURL — test with paired field.
		It("provider: bitbucket-server round-trips when bitbucket.baseURL is set", func() {
			yaml := "provider: bitbucket-server\nbitbucket:\n  baseURL: https://bb.example.com\n"
			cfg, err := writeAndLoad(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Provider).To(Equal(ProviderBitbucketServer))
		})
	})
})
