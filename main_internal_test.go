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
		hideGit       *bool
		model         string
	}
	parse := func(rawArgs []string) result {
		debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model := ParseArgs(
			rawArgs,
		)
		return result{debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model}
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
})

var _ = Describe("applyArgOverrides", func() {
	ctx := context.Background()

	It("applies hideGit=true override", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		t := true
		Expect(applyArgOverrides(ctx, &cfg, &sources, "run", &t, "")).To(Succeed())
		Expect(cfg.HideGit).To(BeTrue())
		Expect(sources.HideGit).To(Equal("arg"))
	})

	It("applies hideGit=false override", func() {
		cfg := config.Defaults()
		cfg.HideGit = true
		sources := config.FieldSources{}
		f := false
		Expect(applyArgOverrides(ctx, &cfg, &sources, "run", &f, "")).To(Succeed())
		Expect(cfg.HideGit).To(BeFalse())
		Expect(sources.HideGit).To(Equal("arg"))
	})

	It("applies model override", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		Expect(
			applyArgOverrides(ctx, &cfg, &sources, "daemon", nil, "claude-opus-4-7"),
		).To(Succeed())
		Expect(cfg.Model).To(Equal("claude-opus-4-7"))
		Expect(sources.Model).To(Equal("arg"))
	})

	It("rejects --hide-git on non-run/daemon command", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		t := true
		err := applyArgOverrides(ctx, &cfg, &sources, "status", &t, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown flag: --hide-git"))
	})

	It("rejects --model on non-run/daemon command", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applyArgOverrides(ctx, &cfg, &sources, "config", nil, "claude-opus-4-7")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown flag: --model"))
	})

	It("rejects invalid model value", func() {
		cfg := config.Defaults()
		sources := config.FieldSources{}
		err := applyArgOverrides(ctx, &cfg, &sources, "run", nil, "claude;bad")
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
})
