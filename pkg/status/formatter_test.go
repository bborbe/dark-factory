// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/status"
)

var _ = Describe("Formatter", func() {
	var formatter status.Formatter

	BeforeEach(func() {
		formatter = status.NewFormatter()
	})

	Describe("Format", func() {
		It("formats idle status", func() {
			st := &status.Status{
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Dark Factory Status"))
			Expect(output).To(ContainSubstring("Daemon:     not running"))
			Expect(output).To(ContainSubstring("Current:    idle"))
			Expect(output).To(ContainSubstring("Queue:      0 prompts"))
			Expect(output).To(ContainSubstring("Completed:  0 prompts"))
		})

		It("formats running status with executing prompt", func() {
			st := &status.Status{
				Daemon:           "running",
				DaemonPID:        12345,
				CurrentPrompt:    "001-test.md",
				ExecutingSince:   "2m30s",
				Container:        "dark-factory-001-test",
				ContainerRunning: true,
				QueueCount:       2,
				QueuedPrompts:    []string{"002-next.md", "003-after.md"},
				CompletedCount:   5,
				LastLogFile:      "prompts/log/001-test.log",
				LastLogSize:      1234,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Daemon:     running (pid 12345)"))
			Expect(output).To(ContainSubstring("Current:    001-test.md (executing since 2m30s)"))
			Expect(output).To(ContainSubstring("Container:  dark-factory-001-test (running)"))
			Expect(output).To(ContainSubstring("Queue:      2 prompts"))
			Expect(output).To(ContainSubstring("002-next.md"))
			Expect(output).To(ContainSubstring("003-after.md"))
			Expect(output).To(ContainSubstring("Completed:  5 prompts"))
			Expect(output).To(ContainSubstring("Last log:   prompts/log/001-test.log"))
		})

		It("formats container not running status", func() {
			st := &status.Status{
				Daemon:           "running",
				CurrentPrompt:    "001-test.md",
				Container:        "dark-factory-001-test",
				ContainerRunning: false,
				QueueCount:       0,
				QueuedPrompts:    []string{},
				CompletedCount:   0,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Container:  dark-factory-001-test (not running)"))
		})

		It("formats bytes correctly", func() {
			st := &status.Status{
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
				LastLogFile:    "test.log",
				LastLogSize:    1536,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("(1.5 KB)"))
		})

		It("formats small bytes", func() {
			st := &status.Status{
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
				LastLogFile:    "test.log",
				LastLogSize:    512,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("(512 B)"))
		})

		It("formats megabytes", func() {
			st := &status.Status{
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
				LastLogFile:    "test.log",
				LastLogSize:    2 * 1024 * 1024,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("(2.0 MB)"))
		})

		It("formats gigabytes", func() {
			st := &status.Status{
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
				LastLogFile:    "test.log",
				LastLogSize:    3 * 1024 * 1024 * 1024,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("(3.0 GB)"))
		})

		It("formats generating spec status", func() {
			st := &status.Status{
				Daemon:              "running",
				DaemonPID:           12345,
				GeneratingSpec:      "034-resume-executing-on-restart.md",
				GeneratingContainer: "dark-factory-gen-034-resume-executing-on-restart",
				QueueCount:          0,
				QueuedPrompts:       []string{},
				CompletedCount:      3,
			}

			output := formatter.Format(st)
			Expect(
				output,
			).To(ContainSubstring("Current:    generating spec 034-resume-executing-on-restart.md"))
			Expect(
				output,
			).To(ContainSubstring("Container:  dark-factory-gen-034-resume-executing-on-restart (running)"))
			Expect(output).NotTo(ContainSubstring("idle"))
		})

		It("does not show log line when no log file", func() {
			st := &status.Status{
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
			}

			output := formatter.Format(st)
			Expect(output).NotTo(ContainSubstring("Last log:"))
		})

		It("shows project dir when set", func() {
			st := &status.Status{
				ProjectDir:     "/home/user/myproject",
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
			}

			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Project:    /home/user/myproject"))
		})

		It("omits project dir line when empty", func() {
			st := &status.Status{
				ProjectDir:     "",
				Daemon:         "not running",
				QueueCount:     0,
				QueuedPrompts:  []string{},
				CompletedCount: 0,
			}

			output := formatter.Format(st)
			Expect(output).NotTo(ContainSubstring("Project:"))
		})
	})

	Describe("Format container line", func() {
		It("includes container line when ContainerMax > 0", func() {
			st := &status.Status{
				Daemon:         "not running",
				QueuedPrompts:  []string{},
				ContainerCount: 2,
				ContainerMax:   3,
			}
			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Containers: 2/3 (system-wide)"))
		})

		It("omits container line when ContainerMax is 0", func() {
			st := &status.Status{
				Daemon:        "not running",
				QueuedPrompts: []string{},
				ContainerMax:  0,
			}
			output := formatter.Format(st)
			Expect(output).NotTo(ContainSubstring("Containers:"))
		})

		It("shows container line after Project and before Daemon", func() {
			st := &status.Status{
				ProjectDir:     "/my/project",
				Daemon:         "running",
				DaemonPID:      1234,
				QueuedPrompts:  []string{},
				ContainerCount: 1,
				ContainerMax:   5,
			}
			output := formatter.Format(st)
			lines := strings.Split(output, "\n")
			var projectIdx, containerIdx, daemonIdx int
			for i, l := range lines {
				if strings.Contains(l, "Project:") {
					projectIdx = i
				}
				if strings.Contains(l, "Containers:") {
					containerIdx = i
				}
				if strings.Contains(l, "Daemon:") {
					daemonIdx = i
				}
			}
			Expect(containerIdx).To(BeNumerically(">", projectIdx))
			Expect(containerIdx).To(BeNumerically("<", daemonIdx))
		})
	})

	Describe("Format warnings section", func() {
		It("shows no warnings section when GitIndexLock is false and DirtyFileCount is 0", func() {
			st := &status.Status{
				Daemon:        "not running",
				QueuedPrompts: []string{},
			}
			output := formatter.Format(st)
			Expect(output).NotTo(ContainSubstring("Warnings:"))
		})

		It("shows .git/index.lock warning when GitIndexLock is true", func() {
			st := &status.Status{
				Daemon:        "not running",
				GitIndexLock:  true,
				QueuedPrompts: []string{},
			}
			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Warnings:"))
			Expect(output).To(ContainSubstring(".git/index.lock exists"))
			Expect(output).To(ContainSubstring("daemon will skip prompts"))
		})

		It("shows dirty file count with threshold when DirtyFileThreshold > 0", func() {
			st := &status.Status{
				Daemon:             "not running",
				DirtyFileCount:     42,
				DirtyFileThreshold: 30,
				QueuedPrompts:      []string{},
			}
			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Warnings:"))
			Expect(output).To(ContainSubstring("42 dirty files (threshold: 30)"))
		})

		It("shows dirty file count without threshold when DirtyFileThreshold is 0", func() {
			st := &status.Status{
				Daemon:             "not running",
				DirtyFileCount:     5,
				DirtyFileThreshold: 0,
				QueuedPrompts:      []string{},
			}
			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Warnings:"))
			Expect(output).To(ContainSubstring("5 dirty files"))
			Expect(output).NotTo(ContainSubstring("threshold"))
		})

		It("shows both warnings when both conditions are true", func() {
			st := &status.Status{
				Daemon:             "not running",
				GitIndexLock:       true,
				DirtyFileCount:     10,
				DirtyFileThreshold: 5,
				QueuedPrompts:      []string{},
			}
			output := formatter.Format(st)
			Expect(output).To(ContainSubstring("Warnings:"))
			Expect(output).To(ContainSubstring(".git/index.lock exists"))
			Expect(output).To(ContainSubstring("10 dirty files (threshold: 5)"))
		})
	})
})
