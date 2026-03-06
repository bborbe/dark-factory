// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
)

type parseArgsResult struct {
	debug      bool
	command    string
	subcommand string
	args       []string
}

func assertParseArgs(t *testing.T, input []string, want parseArgsResult) {
	t.Helper()
	debug, command, subcommand, args := ParseArgs(input)
	if debug != want.debug {
		t.Errorf("debug: got %v, want %v", debug, want.debug)
	}
	if command != want.command {
		t.Errorf("command: got %q, want %q", command, want.command)
	}
	if subcommand != want.subcommand {
		t.Errorf("subcommand: got %q, want %q", subcommand, want.subcommand)
	}
	if len(args) != len(want.args) {
		t.Errorf("args: got %v, want %v", args, want.args)
		return
	}
	for i := range args {
		if args[i] != want.args[i] {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], want.args[i])
		}
	}
}

func TestParseArgsDefaults(t *testing.T) {
	t.Parallel()
	assertParseArgs(t, []string{}, parseArgsResult{false, "run", "", []string{}})
	assertParseArgs(t, []string{"run"}, parseArgsResult{false, "run", "", []string{}})
	assertParseArgs(t, []string{"status"}, parseArgsResult{false, "status", "", []string{}})
	assertParseArgs(t, []string{"list"}, parseArgsResult{false, "list", "", []string{}})
}

func TestParseArgsPrompt(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"prompt", "list"},
		parseArgsResult{false, "prompt", "list", []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "status"},
		parseArgsResult{false, "prompt", "status", []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "retry"},
		parseArgsResult{false, "prompt", "retry", []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "approve", "042"},
		parseArgsResult{false, "prompt", "approve", []string{"042"}},
	)
}

func TestParseArgsSpec(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"spec", "approve", "017"},
		parseArgsResult{false, "spec", "approve", []string{"017"}},
	)
	assertParseArgs(t, []string{"spec", "list"}, parseArgsResult{false, "spec", "list", []string{}})
}

func TestParseArgsUnknown(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"prompts"},
		parseArgsResult{false, "unknown", "", []string{"prompts"}},
	)
	assertParseArgs(
		t,
		[]string{"foo"},
		parseArgsResult{false, "unknown", "", []string{"foo"}},
	)
}

func TestParseArgsDebug(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"-debug", "prompt", "list"},
		parseArgsResult{true, "prompt", "list", []string{}},
	)
	assertParseArgs(t, []string{"-debug", "run"}, parseArgsResult{true, "run", "", []string{}})
}
