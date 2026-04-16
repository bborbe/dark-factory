// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package globalconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("fileLoader.Load", func() {
	var (
		ctx    context.Context
		tmpDir string
		origFn func() (string, error)
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "globalconfig-test-*")
		Expect(err).NotTo(HaveOccurred())

		origFn = userHomeDir
		userHomeDir = func() (string, error) { return tmpDir, nil }
	})

	AfterEach(func() {
		userHomeDir = origFn
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	writeConfig := func(content string) {
		dir := filepath.Join(tmpDir, ".dark-factory")
		Expect(os.MkdirAll(dir, 0750)).To(Succeed())
		path := filepath.Join(dir, "config.yaml")
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
	}

	It("returns defaults when file does not exist", func() {
		cfg, err := NewLoader().Load(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
	})

	It("returns defaults when file is empty", func() {
		writeConfig("")
		cfg, err := NewLoader().Load(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
	})

	It("returns defaults when file is only whitespace", func() {
		writeConfig("   \n  ")
		cfg, err := NewLoader().Load(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
	})

	It("reads maxContainers from file", func() {
		writeConfig("maxContainers: 5\n")
		cfg, err := NewLoader().Load(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.MaxContainers).To(Equal(5))
	})

	It("returns default maxContainers when field is omitted", func() {
		writeConfig("# no fields\n")
		cfg, err := NewLoader().Load(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
	})

	It("returns error for invalid YAML", func() {
		writeConfig("maxContainers: [not an int\n")
		_, err := NewLoader().Load(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("parse config file"))
	})

	It("returns error when maxContainers is 0", func() {
		writeConfig("maxContainers: 0\n")
		_, err := NewLoader().Load(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("maxContainers must be >= 1"))
	})

	It("returns error when maxContainers is negative", func() {
		writeConfig(fmt.Sprintf("maxContainers: %d\n", -1))
		_, err := NewLoader().Load(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("maxContainers must be >= 1"))
	})

	It("returns error when home dir lookup fails", func() {
		userHomeDir = func() (string, error) { return "", fmt.Errorf("no home") }
		_, err := NewLoader().Load(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get home directory"))
	})
})

var _ = Describe("FileExists", func() {
	var (
		ctx    context.Context
		tmpDir string
		origFn func() (string, error)
	)

	BeforeEach(func() {
		ctx = context.Background()
		tmpDir = GinkgoT().TempDir()
		origFn = userHomeDir
		userHomeDir = func() (string, error) { return tmpDir, nil }
	})

	AfterEach(func() {
		userHomeDir = origFn
	})

	It("returns false when the config file does not exist", func() {
		exists, err := FileExists(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse())
	})

	It("returns false when only the home dir exists but config dir is missing", func() {
		// tmpDir exists but .dark-factory/ subdir does not
		exists, err := FileExists(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse())
	})

	It("returns true when the config file is a zero-byte file", func() {
		dir := filepath.Join(tmpDir, ".dark-factory")
		Expect(os.MkdirAll(dir, 0750)).To(Succeed())
		path := filepath.Join(dir, "config.yaml")
		Expect(os.WriteFile(path, []byte{}, 0600)).To(Succeed())

		exists, err := FileExists(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("returns true when the config file contains valid YAML", func() {
		dir := filepath.Join(tmpDir, ".dark-factory")
		Expect(os.MkdirAll(dir, 0750)).To(Succeed())
		path := filepath.Join(dir, "config.yaml")
		Expect(os.WriteFile(path, []byte("maxContainers: 5\n"), 0600)).To(Succeed())

		exists, err := FileExists(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("returns false and non-nil error when home dir lookup fails", func() {
		userHomeDir = func() (string, error) {
			return "", errors.Errorf(ctx, "no home dir")
		}

		exists, err := FileExists(ctx)
		Expect(err).To(HaveOccurred())
		Expect(exists).To(BeFalse())
	})
})
