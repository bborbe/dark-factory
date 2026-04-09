// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor_test

import (
	"testing"

	"github.com/bborbe/dark-factory/pkg/executor"
)

func lookupFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveExtraMountSrc_Darwin_DefaultCache(t *testing.T) {
	result := executor.ResolveExtraMountSrcForTest(
		"$HOST_CACHE_DIR/go-build",
		lookupFrom(map[string]string{"HOME": "/Users/alice"}),
		"darwin",
	)
	if result != "/Users/alice/Library/Caches/go-build" {
		t.Errorf("expected /Users/alice/Library/Caches/go-build, got %s", result)
	}
}

func TestResolveExtraMountSrc_Darwin_IgnoresXDG(t *testing.T) {
	result := executor.ResolveExtraMountSrcForTest(
		"$HOST_CACHE_DIR",
		lookupFrom(map[string]string{"HOME": "/Users/alice", "XDG_CACHE_HOME": "/custom/xdg"}),
		"darwin",
	)
	if result != "/Users/alice/Library/Caches" {
		t.Errorf("expected /Users/alice/Library/Caches, got %s", result)
	}
}

func TestResolveExtraMountSrc_Linux_XDG(t *testing.T) {
	result := executor.ResolveExtraMountSrcForTest(
		"$HOST_CACHE_DIR/go-build",
		lookupFrom(map[string]string{"HOME": "/home/bob", "XDG_CACHE_HOME": "/custom/xdg"}),
		"linux",
	)
	if result != "/custom/xdg/go-build" {
		t.Errorf("expected /custom/xdg/go-build, got %s", result)
	}
}

func TestResolveExtraMountSrc_Linux_NoXDG(t *testing.T) {
	result := executor.ResolveExtraMountSrcForTest(
		"$HOST_CACHE_DIR/go-build",
		lookupFrom(map[string]string{"HOME": "/home/bob"}),
		"linux",
	)
	if result != "/home/bob/.cache/go-build" {
		t.Errorf("expected /home/bob/.cache/go-build, got %s", result)
	}
}

func TestResolveExtraMountSrc_UserOverride(t *testing.T) {
	lookup := lookupFrom(map[string]string{"HOST_CACHE_DIR": "/preset", "HOME": "/home/bob"})

	resultLinux := executor.ResolveExtraMountSrcForTest("$HOST_CACHE_DIR/x", lookup, "linux")
	if resultLinux != "/preset/x" {
		t.Errorf("linux: expected /preset/x, got %s", resultLinux)
	}

	resultDarwin := executor.ResolveExtraMountSrcForTest("$HOST_CACHE_DIR/x", lookup, "darwin")
	if resultDarwin != "/preset/x" {
		t.Errorf("darwin: expected /preset/x, got %s", resultDarwin)
	}
}

func TestResolveExtraMountSrc_Linux_EmptyHomeNoXDG(t *testing.T) {
	result := executor.ResolveExtraMountSrcForTest(
		"$HOST_CACHE_DIR/x",
		lookupFrom(map[string]string{}),
		"linux",
	)
	if result != "/x" {
		t.Errorf("expected /x, got %s", result)
	}
}

func TestResolveExtraMountSrc_PassThroughOtherVars(t *testing.T) {
	result := executor.ResolveExtraMountSrcForTest(
		"$HOME/foo",
		lookupFrom(map[string]string{"HOME": "/home/bob"}),
		"linux",
	)
	if result != "/home/bob/foo" {
		t.Errorf("expected /home/bob/foo, got %s", result)
	}
}
