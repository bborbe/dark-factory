// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/factory"
)

var _ = Describe("Factory", func() {
	Describe("CreateRunner", func() {
		It("should return a non-nil runner", func() {
			cfg := config.Defaults()
			runner := factory.CreateRunner(cfg, "v0.0.1")
			Expect(runner).NotTo(BeNil())
		})
	})
})
