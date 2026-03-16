// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/factory"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/notifier"
)

var _ = Describe("Factory", func() {
	var cfg config.Config

	BeforeEach(func() {
		cfg = config.Defaults()
	})

	Describe("CreateRunner", func() {
		It("should return a non-nil runner", func() {
			runner := factory.CreateRunner(cfg, "v0.0.1")
			Expect(runner).NotTo(BeNil())
		})
	})

	Describe("CreateWatcher", func() {
		It("should return a non-nil watcher", func() {
			ready := make(chan struct{}, 10)
			watcher := factory.CreateWatcher(
				cfg.Prompts.InProgressDir,
				cfg.Prompts.InboxDir,
				nil, // promptManager not needed for nil check
				ready,
				100,
				libtime.NewCurrentDateTime(),
			)
			Expect(watcher).NotTo(BeNil())
		})
	})

	Describe("CreateProcessor", func() {
		It("should return a non-nil processor", func() {
			ready := make(chan struct{}, 10)
			processor := factory.CreateProcessor(
				cfg.Prompts.InProgressDir,
				cfg.Prompts.CompletedDir,
				cfg.Prompts.LogDir,
				"test-project",
				nil, // promptManager not needed for nil check
				nil, // releaser not needed for nil check
				nil, // versionGetter not needed for nil check
				ready,
				cfg.ContainerImage,
				cfg.Model,
				cfg.NetrcFile,
				cfg.GitconfigFile,
				cfg.PR,
				cfg.Worktree,
				git.NewBrancher(),
				git.NewPRCreator(""),
				git.NewPRMerger("", libtime.NewCurrentDateTime()),
				false,
				false,
				false,
				"make precommit",
				"",
				"specs/inbox",
				"specs/in-progress",
				"specs/completed",
				false,
				nil,
				libtime.NewCurrentDateTime(),
				notifier.NewMultiNotifier(),
			)
			Expect(processor).NotTo(BeNil())
		})
	})

	Describe("CreateLocker", func() {
		It("should return a non-nil locker", func() {
			locker := factory.CreateLocker(".")
			Expect(locker).NotTo(BeNil())
		})
	})

	Describe("CreateServer", func() {
		It("should return a non-nil server", func() {
			server := factory.CreateServer(
				8080,
				cfg.Prompts.InboxDir,
				cfg.Prompts.InProgressDir,
				cfg.Prompts.CompletedDir,
				cfg.Prompts.LogDir,
				nil, // promptManager not needed for nil check
				libtime.NewCurrentDateTime(),
			)
			Expect(server).NotTo(BeNil())
		})
	})

	Describe("CreateStatusCommand", func() {
		It("should return a non-nil status command", func() {
			cmd := factory.CreateStatusCommand(cfg)
			Expect(cmd).NotTo(BeNil())
		})
	})

	Describe("CreateCombinedStatusCommand", func() {
		It("should return a non-nil combined status command", func() {
			cmd := factory.CreateCombinedStatusCommand(cfg)
			Expect(cmd).NotTo(BeNil())
		})
	})

	Describe("CreateCombinedListCommand", func() {
		It("should return a non-nil combined list command", func() {
			cmd := factory.CreateCombinedListCommand(cfg)
			Expect(cmd).NotTo(BeNil())
		})
	})
})
