// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
)

var _ = Describe("extractMaxContainers", func() {
	ctx := context.Background()

	It("returns value and empty remaining args when only flag present", func() {
		n, remaining, err := extractMaxContainers(ctx, []string{"--max-containers", "5"})
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(5))
		Expect(remaining).To(BeEmpty())
	})

	It("returns value and trailing args", func() {
		n, remaining, err := extractMaxContainers(ctx, []string{"--max-containers", "5", "other"})
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(5))
		Expect(remaining).To(Equal([]string{"other"}))
	})

	It("returns value with leading args", func() {
		n, remaining, err := extractMaxContainers(ctx, []string{"other", "--max-containers", "5"})
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(5))
		Expect(remaining).To(Equal([]string{"other"}))
	})

	It("returns 0 and original args when flag not present", func() {
		n, remaining, err := extractMaxContainers(ctx, []string{})
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(0))
		Expect(remaining).To(BeEmpty())
	})

	It("returns error when value is missing", func() {
		_, _, err := extractMaxContainers(ctx, []string{"--max-containers"})
		Expect(err).To(HaveOccurred())
	})

	It("returns error when value is not an integer", func() {
		_, _, err := extractMaxContainers(ctx, []string{"--max-containers", "abc"})
		Expect(err).To(HaveOccurred())
	})

	It("returns error when value is 0", func() {
		_, _, err := extractMaxContainers(ctx, []string{"--max-containers", "0"})
		Expect(err).To(HaveOccurred())
	})

	It("returns error when value is negative", func() {
		_, _, err := extractMaxContainers(ctx, []string{"--max-containers", "-1"})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ParseArgs", func() {
	type result struct {
		debug         bool
		command       string
		subcommand    string
		args          []string
		autoApprove   bool
		skipPreflight bool
		model         string
	}
	parse := func(rawArgs []string) result {
		debug, command, subcommand, args, autoApprove, skipPreflight, model := ParseArgs(rawArgs)
		return result{debug, command, subcommand, args, autoApprove, skipPreflight, model}
	}

	It("returns run command with --help in args (validation at dispatch)", func() {
		r := parse([]string{"run", "--help"})
		Expect(r.command).To(Equal("run"))
		Expect(r.args).To(ContainElement("--help"))
	})

	It("returns daemon command with -h in args (validation at dispatch)", func() {
		r := parse([]string{"daemon", "-h"})
		Expect(r.command).To(Equal("daemon"))
		Expect(r.args).To(ContainElement("-h"))
	})

	It("returns prompt command with approve subcommand and --help in args", func() {
		r := parse([]string{"prompt", "approve", "--help"})
		Expect(r.command).To(Equal("prompt"))
		Expect(r.subcommand).To(Equal("approve"))
		Expect(r.args).To(ContainElement("--help"))
	})

	It("returns run command when given unknown positional arg (validation at dispatch)", func() {
		r := parse([]string{"run", "banana"})
		Expect(r.command).To(Equal("run"))
		Expect(r.args).To(ContainElement("banana"))
	})
})

var _ = Describe("validateNoArgs", func() {
	ctx := context.Background()
	noop := func() {}

	It("returns nil for empty args", func() {
		Expect(validateNoArgs(ctx, []string{}, noop)).To(Succeed())
	})

	It("returns error for unexpected positional arg", func() {
		Expect(validateNoArgs(ctx, []string{"banana"}, noop)).To(HaveOccurred())
	})

	It("returns error for unknown flag", func() {
		Expect(validateNoArgs(ctx, []string{"--foo"}, noop)).To(HaveOccurred())
	})
})

var _ = Describe("validateListArgs", func() {
	ctx := context.Background()
	noop := func() {}

	It("accepts no args", func() {
		Expect(validateListArgs(ctx, []string{}, noop)).To(Succeed())
	})

	It("accepts --all", func() {
		Expect(validateListArgs(ctx, []string{"--all"}, noop)).To(Succeed())
	})

	It("returns error for unknown flag", func() {
		Expect(validateListArgs(ctx, []string{"--foo"}, noop)).To(HaveOccurred())
	})

	It("returns error for positional argument", func() {
		Expect(validateListArgs(ctx, []string{"spec-name"}, noop)).To(HaveOccurred())
	})
})

var _ = Describe("validateOneArg", func() {
	ctx := context.Background()
	noop := func() {}

	It("returns nil for exactly one arg", func() {
		Expect(validateOneArg(ctx, []string{"my-slug"}, noop)).To(Succeed())
	})

	It("returns error for missing arg", func() {
		Expect(validateOneArg(ctx, []string{}, noop)).To(HaveOccurred())
	})

	It("returns error for too many args", func() {
		Expect(validateOneArg(ctx, []string{"a", "b"}, noop)).To(HaveOccurred())
	})

	It("returns error for unknown flag", func() {
		Expect(validateOneArg(ctx, []string{"--foo"}, noop)).To(HaveOccurred())
	})
})

var _ = Describe("runCommand --skip-preflight rejection", func() {
	ctx := context.Background()
	dt := libtime.NewCurrentDateTime()

	DescribeTable("rejects --skip-preflight on unsupported commands",
		func(command string) {
			err := runCommand(
				ctx,
				config.Config{},
				command,
				"",
				[]string{},
				false,
				true,
				config.FieldSources{},
				dt,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown flag: --skip-preflight"))
		},
		Entry("status", "status"),
		Entry("list", "list"),
		Entry("prompt", "prompt"),
		Entry("spec", "spec"),
		Entry("scenario", "scenario"),
		Entry("config", "config"),
	)
})

var _ = Describe("applyGlobalOverrides", func() {
	It("applies global model when project did not set it", func() {
		cfg := config.Defaults()
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		m := "claude-opus-4-7"
		global.Model = &m
		proj := config.LayeredProjectOverrides{}
		applyGlobalOverrides(&cfg, global, proj)
		Expect(cfg.Model).To(Equal("claude-opus-4-7"))
	})

	It("does not overwrite project model with global model", func() {
		cfg := config.Defaults()
		cfg.Model = "claude-sonnet-4-6"
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		gm := "claude-opus-4-7"
		global.Model = &gm
		pm := "claude-sonnet-4-6"
		proj := config.LayeredProjectOverrides{Model: &pm}
		applyGlobalOverrides(&cfg, global, proj)
		Expect(cfg.Model).To(Equal("claude-sonnet-4-6"))
	})

	It("applies global hideGit when project did not set it", func() {
		cfg := config.Defaults()
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		global.HideGit = &t
		proj := config.LayeredProjectOverrides{}
		applyGlobalOverrides(&cfg, global, proj)
		Expect(cfg.HideGit).To(BeTrue())
	})

	It("does not overwrite project hideGit=false with global hideGit=true", func() {
		cfg := config.Defaults()
		cfg.HideGit = false
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		global.HideGit = &t
		f := false
		proj := config.LayeredProjectOverrides{HideGit: &f}
		applyGlobalOverrides(&cfg, global, proj)
		Expect(cfg.HideGit).To(BeFalse())
	})

	It("applies global autoApprovePrompts when project did not set it", func() {
		cfg := config.Defaults()
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		global.AutoApprovePrompts = &t
		proj := config.LayeredProjectOverrides{}
		applyGlobalOverrides(&cfg, global, proj)
		Expect(cfg.AutoApprovePrompts).To(BeTrue())
	})

	It(
		"does not overwrite project autoApprovePrompts=false with global autoApprovePrompts=true",
		func() {
			cfg := config.Defaults()
			cfg.AutoApprovePrompts = false
			global := globalconfig.GlobalConfig{MaxContainers: 3}
			t := true
			global.AutoApprovePrompts = &t
			f := false
			proj := config.LayeredProjectOverrides{AutoApprovePrompts: &f}
			applyGlobalOverrides(&cfg, global, proj)
			Expect(cfg.AutoApprovePrompts).To(BeFalse())
		},
	)

	It("applies global AutoGeneratePrompts when project did not set it", func() {
		cfg := config.Defaults()
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		global.AutoGeneratePrompts = &t
		proj := config.LayeredProjectOverrides{}
		applyGlobalOverrides(&cfg, global, proj)
		Expect(cfg.AutoGeneratePrompts).To(BeTrue())
	})

	It("does not overwrite project AutoGeneratePrompts=false with global true", func() {
		cfg := config.Defaults()
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		global.AutoGeneratePrompts = &t
		f := false
		proj := config.LayeredProjectOverrides{AutoGeneratePrompts: &f}
		applyGlobalOverrides(&cfg, global, proj)
		Expect(cfg.AutoGeneratePrompts).To(BeFalse())
	})
})

var _ = Describe("applyArgOverrides", func() {
	ctx := context.Background()

	It("applies model override", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(applyArgOverrides(ctx, &cfg, &sources, "daemon", "claude-opus-4-7")).To(Succeed())
		Expect(cfg.Model).To(Equal("claude-opus-4-7"))
		Expect(sources.Model).To(Equal("arg"))
	})

	It("rejects --model on non-run/daemon command", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applyArgOverrides(ctx, &cfg, &sources, "config", "claude-opus-4-7")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown flag: --model"))
	})

	It("rejects invalid model value", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applyArgOverrides(ctx, &cfg, &sources, "run", "claude;bad")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not match required pattern"))
	})
})

var _ = Describe("validateModelArg", func() {
	ctx := context.Background()

	It("accepts a valid Anthropic model ID", func() {
		Expect(validateModelArg(ctx, "claude-opus-4-7")).To(Succeed())
	})

	It("accepts a model with colon and slash", func() {
		Expect(validateModelArg(ctx, "qwen3.6:35b-a3b")).To(Succeed())
	})

	It("accepts a namespaced local model", func() {
		Expect(validateModelArg(ctx, "local/qwen3.6:35b-a3b")).To(Succeed())
	})

	It("accepts a Docker image ref", func() {
		Expect(validateModelArg(ctx, "docker.io/bborbe/claude-yolo:v0.6.1")).To(Succeed())
	})

	It("rejects model with semicolon (shell metachar)", func() {
		err := validateModelArg(ctx, "claude;rm -rf /")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not match required pattern"))
	})

	It("rejects model with dollar sign", func() {
		err := validateModelArg(ctx, "claude-$VERSION")
		Expect(err).To(HaveOccurred())
	})

	It("rejects model with spaces", func() {
		err := validateModelArg(ctx, "claude opus")
		Expect(err).To(HaveOccurred())
	})

	It("rejects empty model", func() {
		err := validateModelArg(ctx, "")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("computeFieldSources", func() {
	It("returns default for all fields when global and project both absent", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		proj := config.LayeredProjectOverrides{}
		s := computeFieldSources(global, proj)
		Expect(s.Model).To(Equal("default"))
		Expect(s.HideGit).To(Equal("default"))
		Expect(s.AutoRelease).To(Equal("default"))
		Expect(s.DirtyFileThreshold).To(Equal("default"))
		Expect(s.AutoApprovePrompts).To(Equal("default"))
		Expect(s.AutoGeneratePrompts).To(Equal("default"))
	})

	It(
		"returns global for autoGeneratePrompts when global sets it and project does not",
		func() {
			global := globalconfig.GlobalConfig{MaxContainers: 3}
			t := true
			global.AutoGeneratePrompts = &t
			proj := config.LayeredProjectOverrides{}
			s := computeFieldSources(global, proj)
			Expect(s.AutoGeneratePrompts).To(Equal("global"))
		},
	)

	It("returns project for autoGeneratePrompts when project explicitly sets false", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		global.AutoGeneratePrompts = &t
		f := false
		proj := config.LayeredProjectOverrides{AutoGeneratePrompts: &f}
		s := computeFieldSources(global, proj)
		Expect(s.AutoGeneratePrompts).To(Equal("project"))
	})

	It("returns project for autoGeneratePrompts when project explicitly sets true", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		proj := config.LayeredProjectOverrides{}
		t := true
		proj.AutoGeneratePrompts = &t
		s := computeFieldSources(global, proj)
		Expect(s.AutoGeneratePrompts).To(Equal("project"))
	})

	It("returns global when global sets model and project does not", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		m := "claude-opus-4-7"
		global.Model = &m
		proj := config.LayeredProjectOverrides{}
		s := computeFieldSources(global, proj)
		Expect(s.Model).To(Equal("global"))
	})

	It("returns project when project sets model (even if global also set)", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		gm := "claude-opus-4-7"
		global.Model = &gm
		pm := "claude-haiku-4-5"
		proj := config.LayeredProjectOverrides{Model: &pm}
		s := computeFieldSources(global, proj)
		Expect(s.Model).To(Equal("project"))
	})

	It("returns project for hideGit when project explicitly sets false", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		f := false
		proj := config.LayeredProjectOverrides{HideGit: &f}
		s := computeFieldSources(global, proj)
		Expect(s.HideGit).To(Equal("project"))
	})

	It("returns project for maxContainers when project sets it", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		n := 2
		proj := config.LayeredProjectOverrides{MaxContainers: &n}
		s := computeFieldSources(global, proj)
		Expect(s.MaxContainers).To(Equal("project"))
	})

	It("returns empty maxContainers when project did not set it", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		proj := config.LayeredProjectOverrides{}
		s := computeFieldSources(global, proj)
		Expect(s.MaxContainers).To(BeEmpty())
	})

	It("returns default for workflow/pr/autoMerge when project did not set them", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		proj := config.LayeredProjectOverrides{}
		s := computeFieldSources(global, proj)
		Expect(s.Workflow).To(Equal("default"))
		Expect(s.PR).To(Equal("default"))
		Expect(s.AutoMerge).To(Equal("default"))
	})

	It("returns project for workflow when project explicitly sets it", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		w := config.WorkflowBranch
		proj := config.LayeredProjectOverrides{Workflow: &w}
		s := computeFieldSources(global, proj)
		Expect(s.Workflow).To(Equal("project"))
	})

	It("returns project for pr when project explicitly sets it", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		proj := config.LayeredProjectOverrides{PR: &t}
		s := computeFieldSources(global, proj)
		Expect(s.PR).To(Equal("project"))
	})

	It("returns project for autoMerge when project explicitly sets it", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		proj := config.LayeredProjectOverrides{AutoMerge: &t}
		s := computeFieldSources(global, proj)
		Expect(s.AutoMerge).To(Equal("project"))
	})

	It("returns global for autoApprovePrompts when global sets it and project does not", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		t := true
		global.AutoApprovePrompts = &t
		proj := config.LayeredProjectOverrides{}
		s := computeFieldSources(global, proj)
		Expect(s.AutoApprovePrompts).To(Equal("global"))
	})

	It(
		"returns project for autoApprovePrompts when project sets it (even if global also set)",
		func() {
			global := globalconfig.GlobalConfig{MaxContainers: 3}
			gt := true
			global.AutoApprovePrompts = &gt
			pf := false
			proj := config.LayeredProjectOverrides{AutoApprovePrompts: &pf}
			s := computeFieldSources(global, proj)
			Expect(s.AutoApprovePrompts).To(Equal("project"))
		},
	)

	It("returns default for autoApprovePrompts when neither global nor project set it", func() {
		global := globalconfig.GlobalConfig{MaxContainers: 3}
		proj := config.LayeredProjectOverrides{}
		s := computeFieldSources(global, proj)
		Expect(s.AutoApprovePrompts).To(Equal("default"))
	})
})

var _ = Describe("parseSetFlags", func() {
	ctx := context.Background()

	It("returns empty map and unchanged args when no --set flags", func() {
		overrides, filtered, err := parseSetFlags(
			ctx,
			[]string{"run", "--model", "claude-opus-4-7"},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(overrides).To(BeEmpty())
		Expect(filtered).To(Equal([]string{"run", "--model", "claude-opus-4-7"}))
	})

	It("collects a single --set key=value", func() {
		overrides, filtered, err := parseSetFlags(ctx, []string{"run", "--set", "hideGit=true"})
		Expect(err).NotTo(HaveOccurred())
		Expect(overrides).To(HaveKeyWithValue("hideGit", "true"))
		Expect(filtered).To(Equal([]string{"run"}))
	})

	It("collects multiple distinct --set keys", func() {
		overrides, filtered, err := parseSetFlags(ctx, []string{
			"--set", "hideGit=false",
			"--set", "autoRelease=true",
			"run",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(overrides).To(HaveKeyWithValue("hideGit", "false"))
		Expect(overrides).To(HaveKeyWithValue("autoRelease", "true"))
		Expect(filtered).To(Equal([]string{"run"}))
	})

	It("last value wins for duplicate keys", func() {
		overrides, _, err := parseSetFlags(ctx, []string{
			"--set", "hideGit=true",
			"--set", "hideGit=false",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(overrides).To(HaveKeyWithValue("hideGit", "false"))
	})

	It("handles value containing = (e.g. model with docker image ref)", func() {
		overrides, _, err := parseSetFlags(ctx, []string{"--set", "model=docker.io/foo:v1=extra"})
		Expect(err).NotTo(HaveOccurred())
		Expect(overrides).To(HaveKeyWithValue("model", "docker.io/foo:v1=extra"))
	})

	It("returns error when --set has no following value", func() {
		_, _, err := parseSetFlags(ctx, []string{"--set"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--set requires a value"))
	})

	It("returns error when --set value has no = sign", func() {
		_, _, err := parseSetFlags(ctx, []string{"--set", "hideGit"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("key=value"))
	})

	It("returns error when --set key is empty", func() {
		_, _, err := parseSetFlags(ctx, []string{"--set", "=true"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("key must not be empty"))
	})
})

var _ = Describe("applySetOverrides", func() {
	ctx := context.Background()

	It("does nothing when overrides map is empty", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{})).To(Succeed())
		Expect(sources.HideGit).To(BeEmpty())
	})

	It("sets hideGit=true and marks source=arg", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"hideGit": "true"}),
		).To(Succeed())
		Expect(cfg.HideGit).To(BeTrue())
		Expect(sources.HideGit).To(Equal("arg"))
	})

	It("sets hideGit=false and marks source=arg", func() {
		cfg := config.Defaults()
		cfg.HideGit = true
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"hideGit": "false"}),
		).To(Succeed())
		Expect(cfg.HideGit).To(BeFalse())
		Expect(sources.HideGit).To(Equal("arg"))
	})

	It("rejects hideGit=yes (strict bool)", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"hideGit": "yes"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid bool"))
		Expect(err.Error()).To(ContainSubstring("true or false"))
	})

	It("sets autoGeneratePrompts=true and marks source=arg", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(
				ctx,
				&cfg,
				&sources,
				"run",
				map[string]string{"autoGeneratePrompts": "true"},
			),
		).To(Succeed())
		Expect(cfg.AutoGeneratePrompts).To(BeTrue())
		Expect(sources.AutoGeneratePrompts).To(Equal("arg"))
	})

	It("sets autoGeneratePrompts=false and marks source=arg", func() {
		cfg := config.Defaults()
		cfg.AutoGeneratePrompts = true
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(
				ctx,
				&cfg,
				&sources,
				"daemon",
				map[string]string{"autoGeneratePrompts": "false"},
			),
		).To(Succeed())
		Expect(cfg.AutoGeneratePrompts).To(BeFalse())
		Expect(sources.AutoGeneratePrompts).To(Equal("arg"))
	})

	It("rejects autoGeneratePrompts=yes (strict bool)", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(
			ctx,
			&cfg,
			&sources,
			"run",
			map[string]string{"autoGeneratePrompts": "yes"},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid bool"))
		Expect(err.Error()).To(ContainSubstring("true or false"))
	})

	It(
		"rejects --set disableAutoGeneratePrompts=true as unknown key (legacy name removed)",
		func() {
			cfg := config.Defaults()
			sources := config.FieldSources{}
			err := applySetOverrides(
				ctx,
				&cfg,
				&sources,
				"run",
				map[string]string{"disableAutoGeneratePrompts": "true"},
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disableAutoGeneratePrompts"))
		},
	)

	It("sets autoRelease=false and marks source=arg", func() {
		cfg := config.Defaults()
		cfg.AutoRelease = true
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(
				ctx,
				&cfg,
				&sources,
				"daemon",
				map[string]string{"autoRelease": "false"},
			),
		).To(Succeed())
		Expect(cfg.AutoRelease).To(BeFalse())
		Expect(sources.AutoRelease).To(Equal("arg"))
	})

	It("sets dirtyFileThreshold=5 and marks source=arg", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(
				ctx,
				&cfg,
				&sources,
				"run",
				map[string]string{"dirtyFileThreshold": "5"},
			),
		).To(Succeed())
		Expect(cfg.DirtyFileThreshold).To(Equal(5))
		Expect(sources.DirtyFileThreshold).To(Equal("arg"))
	})

	It("rejects dirtyFileThreshold=-1 (range check)", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(
			ctx,
			&cfg,
			&sources,
			"run",
			map[string]string{"dirtyFileThreshold": "-1"},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("dirtyFileThreshold must be >= 0"))
	})

	It("rejects dirtyFileThreshold=abc (parse error)", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(
			ctx,
			&cfg,
			&sources,
			"run",
			map[string]string{"dirtyFileThreshold": "abc"},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid integer"))
	})

	It("sets model=claude-opus-4-7 and marks source=arg", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(
				ctx,
				&cfg,
				&sources,
				"run",
				map[string]string{"model": "claude-opus-4-7"},
			),
		).To(Succeed())
		Expect(cfg.Model).To(Equal("claude-opus-4-7"))
		Expect(sources.Model).To(Equal("arg"))
	})

	It("rejects model with shell metachar", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(
			ctx,
			&cfg,
			&sources,
			"run",
			map[string]string{"model": "claude;rm -rf /"},
		)
		Expect(err).To(HaveOccurred())
	})

	It("sets maxContainers=2", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"maxContainers": "2"}),
		).To(Succeed())
		Expect(cfg.MaxContainers).To(Equal(2))
		Expect(sources.MaxContainers).To(Equal("arg"))
	})

	It("rejects maxContainers=0 (range check)", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(
			ctx,
			&cfg,
			&sources,
			"run",
			map[string]string{"maxContainers": "0"},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("maxContainers must be >= 1"))
	})

	It("rejects unknown key and lists supported keys", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"unknownKey": "foo"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown config key"))
		Expect(err.Error()).To(ContainSubstring("hideGit"))
		Expect(err.Error()).To(ContainSubstring("autoRelease"))
	})

	It("rejects --set on non-run/daemon command", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(
			ctx,
			&cfg,
			&sources,
			"status",
			map[string]string{"hideGit": "true"},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown flag: --set"))
	})

	It("--max-containers N takes precedence over --set maxContainers=N when applied after", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"maxContainers": "5"}),
		).To(Succeed())
		Expect(cfg.MaxContainers).To(Equal(5))
		// Simulate extractMaxContainers applying --max-containers 3 afterwards
		cfg.MaxContainers = 3
		Expect(cfg.MaxContainers).To(Equal(3))
	})

	It("sets workflow=branch and marks source=arg", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"workflow": "branch"}),
		).To(Succeed())
		Expect(cfg.Workflow).To(Equal(config.WorkflowBranch))
		Expect(sources.Workflow).To(Equal("arg"))
	})

	It("sets workflow=clone and marks source=arg", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"workflow": "clone"}),
		).To(Succeed())
		Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
		Expect(sources.Workflow).To(Equal("arg"))
	})

	It("rejects workflow=invalid with error listing valid values", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(
			ctx,
			&cfg,
			&sources,
			"run",
			map[string]string{"workflow": "invalid"},
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown workflow"))
		Expect(err.Error()).To(ContainSubstring("direct"))
		Expect(err.Error()).To(ContainSubstring("branch"))
		Expect(err.Error()).To(ContainSubstring("worktree"))
		Expect(err.Error()).To(ContainSubstring("clone"))
	})

	It("rejects workflow=pr (legacy enum) with message pointing to clone+pr", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"workflow": "pr"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("legacy workflow value"))
		Expect(err.Error()).To(ContainSubstring("workflow=clone"))
		Expect(err.Error()).To(ContainSubstring("pr=true"))
	})

	It("sets pr=true and marks source=arg", func() {
		cfg := config.Defaults()
		cfg.Workflow = config.WorkflowBranch // pr=true requires non-direct workflow
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "true"}),
		).To(Succeed())
		Expect(cfg.PR).To(BeTrue())
		Expect(sources.PR).To(Equal("arg"))
	})

	It("sets pr=false and marks source=arg", func() {
		cfg := config.Defaults()
		cfg.PR = true
		cfg.Workflow = config.WorkflowBranch
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "false"}),
		).To(Succeed())
		Expect(cfg.PR).To(BeFalse())
		Expect(sources.PR).To(Equal("arg"))
	})

	It("rejects pr=yes (invalid bool)", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "yes"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid bool"))
		Expect(err.Error()).To(ContainSubstring("true or false"))
	})

	It("sets autoMerge=true and marks source=arg", func() {
		cfg := config.Defaults()
		cfg.PR = true
		cfg.Workflow = config.WorkflowBranch
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"autoMerge": "true"}),
		).To(Succeed())
		Expect(cfg.AutoMerge).To(BeTrue())
		Expect(sources.AutoMerge).To(Equal("arg"))
	})

	It("rejects autoMerge=1 (invalid bool)", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"autoMerge": "1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid bool"))
	})

	It("workflow=direct pr=true combination fails cfg.Validate after both applied", func() {
		cfg := config.Defaults() // Workflow=direct (default)
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "true"}),
		).To(Succeed()) // applySetOverrides itself succeeds — single-key validation only
		// The cross-field validator fires on cfg.Validate (called by run() after applySetOverrides)
		err := cfg.Validate(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("incompatible"))
	})

	It("autoMerge=true without pr=true fails cfg.Validate after applied", func() {
		cfg := config.Defaults() // PR=false (default)
		sources := config.FieldSources{}
		Expect(
			applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"autoMerge": "true"}),
		).To(Succeed())
		err := cfg.Validate(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("autoMerge requires pr: true"))
	})

	It("last workflow wins when key appears twice", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		// Simulate last-wins by applying two separate calls (map iteration order is undefined;
		// use the parseSetFlags result directly for deterministic ordering in unit tests)
		Expect(applyOneSetOverride(ctx, &cfg, &sources, "workflow", "branch")).To(Succeed())
		Expect(applyOneSetOverride(ctx, &cfg, &sources, "workflow", "clone")).To(Succeed())
		Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
		Expect(sources.Workflow).To(Equal("arg"))
	})
})

var _ = Describe("extractVerifyingStaleHours", func() {
	ctx := context.Background()

	It("returns default 24 and original args when flag absent", func() {
		n, remaining, err := extractVerifyingStaleHours(ctx, []string{})
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(24))
		Expect(remaining).To(BeEmpty())
	})

	It("returns parsed value with --verifying-stale-hours=48", func() {
		n, remaining, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours=48"})
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(48))
		Expect(remaining).To(BeEmpty())
	})

	It("returns error on empty value (--verifying-stale-hours=)", func() {
		_, _, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours="})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requires a value"))
	})

	It("returns error on non-integer value", func() {
		_, _, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours=abc"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("positive integer"))
	})

	It("returns error on value less than 1", func() {
		_, _, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours=0"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("positive integer"))
	})

	It("strips flag from remaining args, preserving other args", func() {
		n, remaining, err := extractVerifyingStaleHours(ctx, []string{"--fix", "--verifying-stale-hours=48", "other"})
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(48))
		Expect(remaining).To(Equal([]string{"--fix", "other"}))
	})
})

var _ = Describe("validateDoctorArgs", func() {
	ctx := context.Background()

	It("returns nil for empty args", func() {
		Expect(validateDoctorArgs(ctx, []string{})).To(Succeed())
	})

	It("accepts --fix", func() {
		Expect(validateDoctorArgs(ctx, []string{"--fix"})).To(Succeed())
	})

	It("accepts --yes", func() {
		Expect(validateDoctorArgs(ctx, []string{"--yes"})).To(Succeed())
	})

	It("accepts --fix --yes combined", func() {
		Expect(validateDoctorArgs(ctx, []string{"--fix", "--yes"})).To(Succeed())
	})

	It("accepts --verifying-stale-hours=24", func() {
		Expect(validateDoctorArgs(ctx, []string{"--verifying-stale-hours=24"})).To(Succeed())
	})

	It("accepts --help and -h", func() {
		Expect(validateDoctorArgs(ctx, []string{"--help"})).To(Succeed())
		Expect(validateDoctorArgs(ctx, []string{"-h"})).To(Succeed())
	})

	It("returns error on unknown flag", func() {
		err := validateDoctorArgs(ctx, []string{"--unknown"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown flag"))
	})

	It("returns error on positional argument", func() {
		err := validateDoctorArgs(ctx, []string{"some-positional"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown flag"))
	})
})
