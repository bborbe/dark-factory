// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
)

var _ = Describe("Provider", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	DescribeTable("Validate",
		func(provider config.Provider, expectErr bool) {
			err := provider.Validate(ctx)
			if expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("github is valid", config.ProviderGitHub, false),
		Entry("bitbucket-server is valid", config.ProviderBitbucketServer, false),
		Entry("empty string is invalid", config.Provider(""), true),
		Entry("invalid value is rejected", config.Provider("invalid"), true),
		Entry("gitlab is not supported", config.Provider("gitlab"), true),
	)
})
