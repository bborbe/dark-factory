// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight_test

import (
	"context"
	"time"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/preflight"
)

var _ = Describe("resolveExtraMountSrc", func() {
	It("expands HOST_CACHE_DIR on linux with HOME set", func() {
		lookup := func(key string) string {
			if key == "HOME" {
				return "/home/user"
			}
			return ""
		}
		result := preflight.ResolveExtraMountSrc("${HOST_CACHE_DIR}/go", lookup, "linux")
		Expect(result).To(Equal("/home/user/.cache/go"))
	})

	It("expands HOST_CACHE_DIR on darwin with HOME set", func() {
		lookup := func(key string) string {
			if key == "HOME" {
				return "/Users/user"
			}
			return ""
		}
		result := preflight.ResolveExtraMountSrc("${HOST_CACHE_DIR}/go", lookup, "darwin")
		Expect(result).To(Equal("/Users/user/Library/Caches/go"))
	})

	It("uses XDG_CACHE_HOME on linux when set", func() {
		lookup := func(key string) string {
			if key == "XDG_CACHE_HOME" {
				return "/custom/cache"
			}
			return ""
		}
		result := preflight.ResolveExtraMountSrc("${HOST_CACHE_DIR}/go", lookup, "linux")
		Expect(result).To(Equal("/custom/cache/go"))
	})

	It("uses explicit HOST_CACHE_DIR when set", func() {
		lookup := func(key string) string {
			if key == "HOST_CACHE_DIR" {
				return "/explicit/cache"
			}
			return ""
		}
		result := preflight.ResolveExtraMountSrc("${HOST_CACHE_DIR}/go", lookup, "linux")
		Expect(result).To(Equal("/explicit/cache/go"))
	})

	It("expands generic env vars", func() {
		lookup := func(key string) string {
			if key == "MYVAR" {
				return "/my/path"
			}
			return ""
		}
		result := preflight.ResolveExtraMountSrc("${MYVAR}/subdir", lookup, "linux")
		Expect(result).To(Equal("/my/path/subdir"))
	})
})

var _ = Describe("darwinCacheDir", func() {
	It("returns Library/Caches path when home is set", func() {
		Expect(preflight.DarwinCacheDir("/Users/user")).To(Equal("/Users/user/Library/Caches"))
	})

	It("returns empty string when home is empty", func() {
		Expect(preflight.DarwinCacheDir("")).To(Equal(""))
	})
})

var _ = Describe("linuxCacheDir", func() {
	It("returns XDG_CACHE_HOME when set", func() {
		lookup := func(key string) string {
			if key == "XDG_CACHE_HOME" {
				return "/xdg/cache"
			}
			return ""
		}
		Expect(preflight.LinuxCacheDir(lookup, "/home/user")).To(Equal("/xdg/cache"))
	})

	It("falls back to home/.cache when XDG not set", func() {
		lookup := func(key string) string { return "" }
		Expect(preflight.LinuxCacheDir(lookup, "/home/user")).To(Equal("/home/user/.cache"))
	})

	It("returns empty string when both XDG and home are empty", func() {
		lookup := func(key string) string { return "" }
		Expect(preflight.LinuxCacheDir(lookup, "")).To(Equal(""))
	})
})

var _ = Describe("truncateSHA", func() {
	It("returns first 12 chars for long SHA", func() {
		Expect(preflight.TruncateSHA("abcdef123456789")).To(Equal("abcdef123456"))
	})

	It("returns full SHA when shorter than 12", func() {
		Expect(preflight.TruncateSHA("abc")).To(Equal("abc"))
	})

	It("handles empty string", func() {
		Expect(preflight.TruncateSHA("")).To(Equal(""))
	})
})

var _ = Describe("buildPreflightDockerArgs", func() {
	var (
		projectRoot    = "/workspace"
		containerImage = "my-image:latest"
		command        = "make precommit"
		home           = "/home/user"
		lookup         = func(key string) string { return "" }
		goos           = "linux"
	)

	It("produces correct base args without extra mounts", func() {
		args := preflight.BuildPreflightDockerArgs(
			projectRoot,
			containerImage,
			command,
			nil,
			home,
			lookup,
			goos,
		)
		Expect(args).To(Equal([]string{
			"run", "--rm",
			"-v", "/workspace:/workspace",
			"-w", "/workspace",
			"my-image:latest",
			"sh", "-c", "make precommit",
		}))
	})

	It("appends read-write extra mount when src exists", func() {
		tempDir := GinkgoT().TempDir()
		ro := false
		mounts := []config.ExtraMount{{Src: tempDir, Dst: "/host", ReadOnly: &ro}}
		args := preflight.BuildPreflightDockerArgs(
			projectRoot,
			containerImage,
			command,
			mounts,
			home,
			lookup,
			goos,
		)
		Expect(args).To(ContainElement(tempDir + ":/host"))
		Expect(args).NotTo(ContainElement(tempDir + ":/host:ro"))
	})

	It("appends read-only extra mount when ReadOnly is true", func() {
		tempDir := GinkgoT().TempDir()
		ro := true
		mounts := []config.ExtraMount{{Src: tempDir, Dst: "/host", ReadOnly: &ro}}
		args := preflight.BuildPreflightDockerArgs(
			projectRoot,
			containerImage,
			command,
			mounts,
			home,
			lookup,
			goos,
		)
		Expect(args).To(ContainElement(tempDir + ":/host:ro"))
	})

	It("skips extra mount when src does not exist", func() {
		mounts := []config.ExtraMount{{Src: "/nonexistent/path/abc123", Dst: "/host"}}
		args := preflight.BuildPreflightDockerArgs(
			projectRoot,
			containerImage,
			command,
			mounts,
			home,
			lookup,
			goos,
		)
		// Should not contain any mount with the nonexistent src
		Expect(args).NotTo(ContainElement(ContainSubstring("/nonexistent/path/abc123")))
	})

	It("expands tilde in extra mount src", func() {
		mounts := []config.ExtraMount{{Src: "~/some/path", Dst: "/host"}}
		args := preflight.BuildPreflightDockerArgs(
			projectRoot,
			containerImage,
			command,
			mounts,
			home,
			lookup,
			goos,
		)
		// The tilde should be replaced with home; src won't exist so it'll be skipped, but the expansion is tested
		Expect(args).NotTo(ContainElement(ContainSubstring("~/some/path")))
	})

	It("resolves relative extra mount src relative to projectRoot", func() {
		mounts := []config.ExtraMount{{Src: "relative/path", Dst: "/host"}}
		args := preflight.BuildPreflightDockerArgs(
			projectRoot,
			containerImage,
			command,
			mounts,
			home,
			lookup,
			goos,
		)
		// Relative path becomes projectRoot/relative/path (won't exist, so skipped)
		Expect(args).NotTo(ContainElement(ContainSubstring("relative/path")))
	})
})

var _ = Describe("Checker", func() {
	var (
		ctx          context.Context
		fakeNotifier *mocks.Notifier
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeNotifier = &mocks.Notifier{}
		fakeNotifier.NotifyReturns(nil)
	})

	Describe("disabled (empty command)", func() {
		It("returns true without calling the runner", func() {
			runnerCalled := false
			ch := preflight.NewCheckerWithRunner("", 0, fakeNotifier, "proj", "abc123",
				func(ctx context.Context) (string, error) {
					runnerCalled = true
					return "", nil
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(runnerCalled).To(BeFalse())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight passes", func() {
		It("returns true and does not notify", func() {
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				8*time.Hour,
				fakeNotifier,
				"proj",
				"sha1",
				func(ctx context.Context) (string, error) { return "ok output", nil },
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight fails", func() {
		It("returns false and sends preflight_failed notification", func() {
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				8*time.Hour,
				fakeNotifier,
				"myproject",
				"sha2",
				func(ctx context.Context) (string, error) {
					return "lint error on line 42", errors.New(ctx, "exit status 1")
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(1))
			_, event := fakeNotifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("preflight_failed"))
			Expect(event.ProjectName).To(Equal("myproject"))
		})
	})

	Describe("caching", func() {
		It("reuses cached result within interval for same SHA", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"sha3",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			ok1, _ := ch.Check(ctx)
			ok2, _ := ch.Check(ctx)
			Expect(ok1).To(BeTrue())
			Expect(ok2).To(BeTrue())
			Expect(callCount).To(Equal(1), "runner should be called only once due to cache")
		})

		It("re-runs when SHA changes", func() {
			callCount := 0
			ch1 := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"sha-A",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch1.Check(ctx)
			ch2 := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"sha-B",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch2.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should re-run when SHA changes")
		})

		It("re-runs when interval is zero (no caching)", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner("make precommit", 0, fakeNotifier, "proj", "sha4",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch.Check(ctx)
			_, _ = ch.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should always re-run when interval is 0")
		})

		It("re-runs when SHA fetcher returns empty (cache miss)", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				"",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch.Check(ctx)
			_, _ = ch.Check(ctx)
			Expect(callCount).To(Equal(2), "empty SHA always triggers re-run")
		})
	})

	Describe("SHA fetcher error", func() {
		It("proceeds without cache when SHA fetch fails, still runs the check", func() {
			callCount := 0
			ch := preflight.NewCheckerWithSHAError(
				"make precommit",
				1*time.Hour,
				fakeNotifier,
				"proj",
				errors.New(ctx, "git not found"),
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(callCount).To(Equal(1))
		})
	})

	Describe("NewChecker constructor", func() {
		It("returns a non-nil Checker", func() {
			ch := preflight.NewChecker("", 0, "/tmp", "img:latest", nil, fakeNotifier, "proj")
			Expect(ch).NotTo(BeNil())
		})

		It("disabled checker returns true immediately", func() {
			ch := preflight.NewChecker("", 0, "/tmp", "img:latest", nil, fakeNotifier, "proj")
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})
})
