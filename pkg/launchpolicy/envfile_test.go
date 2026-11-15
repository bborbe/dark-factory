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

var _ = Describe("IsSecretEnvKey", func() {
	DescribeTable("classifies credential-shaped keys",
		func(key string, want bool) {
			Expect(launchpolicy.IsSecretEnvKey(key)).To(Equal(want))
		},
		Entry("auth token", "ANTHROPIC_AUTH_TOKEN", true),
		Entry("api key", "SOME_API_KEY", true),
		Entry("lowercase token", "github_token", true),
		Entry("secret", "CLIENT_SECRET", true),
		Entry("password", "DB_PASSWORD", true),
		Entry("private key", "DEPLOY_PRIVATE_KEY", true),
		Entry("base url", "ANTHROPIC_BASE_URL", false),
		Entry("model", "ANTHROPIC_MODEL", false),
		Entry("goprivate", "GOPRIVATE", false),
		Entry("managed marker", launchpolicy.EnvManagedMarker, false),
	)
})

var _ = Describe("DropSecretEnvKeys", func() {
	It("removes credential-shaped keys and reports their names", func() {
		kept, dropped := launchpolicy.DropSecretEnvKeys(map[string]string{
			"ANTHROPIC_AUTH_TOKEN": "secret-value",
			"GOPRIVATE":            "example.com/*",
		})

		Expect(dropped).To(Equal([]string{"ANTHROPIC_AUTH_TOKEN"}))
		Expect(kept).To(Equal(map[string]string{"GOPRIVATE": "example.com/*"}))
	})

	It("returns the input unchanged when nothing is credential-shaped", func() {
		env := map[string]string{"GOPRIVATE": "example.com/*"}
		kept, dropped := launchpolicy.DropSecretEnvKeys(env)

		Expect(dropped).To(BeEmpty())
		Expect(kept).To(Equal(env))
	})

	It("returns a copy, so mutating the result never mutates the input", func() {
		env := map[string]string{"GOPRIVATE": "example.com/*"}
		kept, _ := launchpolicy.DropSecretEnvKeys(env)

		kept["GOPRIVATE"] = "mutated"
		Expect(env["GOPRIVATE"]).To(Equal("example.com/*"))
	})

	It("drops an access key id", func() {
		kept, dropped := launchpolicy.DropSecretEnvKeys(map[string]string{
			"AWS_ACCESS_KEY_ID": "AKIAEXAMPLE",
			"GOPRIVATE":         "example.com/*",
		})

		Expect(dropped).To(Equal([]string{"AWS_ACCESS_KEY_ID"}))
		Expect(kept).To(Equal(map[string]string{"GOPRIVATE": "example.com/*"}))
	})

	It("handles a nil env", func() {
		kept, dropped := launchpolicy.DropSecretEnvKeys(nil)

		Expect(dropped).To(BeEmpty())
		Expect(kept).To(BeNil())
	})
})
