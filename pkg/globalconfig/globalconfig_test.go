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
})
