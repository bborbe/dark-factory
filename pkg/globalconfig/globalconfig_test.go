// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package globalconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/globalconfig"
)

var _ = Describe("GlobalConfig.Validate", func() {
	var ctx context.Context
	BeforeEach(func() { ctx = context.Background() })

	It("returns nil for valid config", func() {
		cfg := globalconfig.GlobalConfig{MaxContainers: 3}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})

	It("returns error when MaxContainers is 0", func() {
		cfg := globalconfig.GlobalConfig{MaxContainers: 0}
		Expect(cfg.Validate(ctx)).To(HaveOccurred())
	})

	It("returns error when MaxContainers is negative", func() {
		cfg := globalconfig.GlobalConfig{MaxContainers: -1}
		Expect(cfg.Validate(ctx)).To(HaveOccurred())
	})

	It("returns nil for MaxContainers of 1", func() {
		cfg := globalconfig.GlobalConfig{MaxContainers: 1}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})

	It("returns nil when HideGit is set to true", func() {
		t := true
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, HideGit: &t}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})

	It("returns nil when HideGit is set to false", func() {
		f := false
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, HideGit: &f}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})

	It("returns nil when DirtyFileThreshold is zero", func() {
		z := 0
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, DirtyFileThreshold: &z}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})

	It("returns error when DirtyFileThreshold is negative", func() {
		n := -1
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, DirtyFileThreshold: &n}
		Expect(cfg.Validate(ctx)).To(HaveOccurred())
		Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("dirtyFileThreshold"))
	})

	It("returns nil when Model is a valid Anthropic ID", func() {
		m := "claude-opus-4-7"
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &m}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})

	It("returns nil when Model contains colon and slash (Docker image ref)", func() {
		m := "docker.io/bborbe/claude-yolo:v0.6.1"
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &m}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})

	It("returns error when Model is empty string", func() {
		empty := ""
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &empty}
		Expect(cfg.Validate(ctx)).To(HaveOccurred())
		Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("model"))
	})

	It("returns error when Model contains semicolons (shell metachar)", func() {
		bad := "claude;rm -rf /"
		cfg := globalconfig.GlobalConfig{MaxContainers: 3, Model: &bad}
		Expect(cfg.Validate(ctx)).To(HaveOccurred())
		Expect(cfg.Validate(ctx).Error()).To(ContainSubstring("model"))
	})

	It("returns nil when all 4 new fields are nil (not set)", func() {
		cfg := globalconfig.GlobalConfig{MaxContainers: 3}
		Expect(cfg.Validate(ctx)).To(Succeed())
	})
})
