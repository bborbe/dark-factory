// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package launchpolicy_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/launchpolicy"
)

var _ = Describe("FormatEnvFile", func() {
	It("renders sorted KEY=VALUE lines", func() {
		Expect(launchpolicy.FormatEnvFile(map[string]string{
			"ZED":   "last",
			"ALPHA": "first",
		})).To(Equal("ALPHA=first\nZED=last\n"))
	})

	It("renders an empty env as an empty string", func() {
		Expect(launchpolicy.FormatEnvFile(map[string]string{})).To(Equal(""))
	})
})

var _ = Describe("PrepareEnvFile", func() {
	It("writes the env to a 0600 file, points opts at it, and clears opts.Env", func() {
		opts := launchpolicy.ContainerLaunchOpts{
			Env: map[string]string{
				"ANTHROPIC_AUTH_TOKEN": "sentinel-token-value",
				"ANTHROPIC_BASE_URL":   "http://127.0.0.1:8788",
			},
		}

		cleanup, err := launchpolicy.PrepareEnvFile(context.Background(), &opts)
		Expect(err).To(BeNil())
		defer cleanup()

		Expect(opts.Env).To(BeNil(), "opts.Env must be cleared so nothing reaches -e flags")
		Expect(opts.EnvFile).NotTo(BeEmpty())

		info, err := os.Stat(opts.EnvFile)
		Expect(err).To(BeNil())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))

		content, err := os.ReadFile(opts.EnvFile)
		Expect(err).To(BeNil())
		Expect(string(content)).To(Equal(
			"ANTHROPIC_AUTH_TOKEN=sentinel-token-value\n" +
				"ANTHROPIC_BASE_URL=http://127.0.0.1:8788\n" +
				launchpolicy.EnvManagedMarker + "=true\n",
		))
	})

	It("emits the managed marker even when opts.Env is empty", func() {
		opts := launchpolicy.ContainerLaunchOpts{}

		cleanup, err := launchpolicy.PrepareEnvFile(context.Background(), &opts)
		Expect(err).To(BeNil())
		defer cleanup()

		content, err := os.ReadFile(opts.EnvFile)
		Expect(err).To(BeNil())
		Expect(string(content)).To(Equal(launchpolicy.EnvManagedMarker + "=true\n"))
	})

	It("removes the env file when cleanup runs", func() {
		opts := launchpolicy.ContainerLaunchOpts{Env: map[string]string{"A": "1"}}

		cleanup, err := launchpolicy.PrepareEnvFile(context.Background(), &opts)
		Expect(err).To(BeNil())
		path := opts.EnvFile

		cleanup()

		_, err = os.Stat(path)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
