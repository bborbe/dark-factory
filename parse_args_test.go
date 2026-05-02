// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
)

type parseArgsResult struct {
	debug         bool
	command       string
	subcommand    string
	args          []string
	autoApprove   bool
	skipPreflight bool
	model         string
}

func assertParseArgs(t *testing.T, input []string, want parseArgsResult) {
	t.Helper()
	debug, command, subcommand, args, autoApprove, skipPreflight, model := ParseArgs(input)
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
	if autoApprove != want.autoApprove {
		t.Errorf("autoApprove: got %v, want %v", autoApprove, want.autoApprove)
	}
	if skipPreflight != want.skipPreflight {
		t.Errorf("skipPreflight: got %v, want %v", skipPreflight, want.skipPreflight)
	}
	if model != want.model {
		t.Errorf("model: got %q, want %q", model, want.model)
	}
}

func TestParseArgsDefaults(t *testing.T) {
	t.Parallel()
	assertParseArgs(t, []string{"run"}, parseArgsResult{command: "run", args: []string{}})
	assertParseArgs(t, []string{"daemon"}, parseArgsResult{command: "daemon", args: []string{}})
	assertParseArgs(t, []string{"status"}, parseArgsResult{command: "status", args: []string{}})
	assertParseArgs(t, []string{"list"}, parseArgsResult{command: "list", args: []string{}})
	assertParseArgs(t, []string{"config"}, parseArgsResult{command: "config", args: []string{}})
}

func TestParseArgsNoArgs(t *testing.T) {
	t.Parallel()
	// No args must return "help" — so running `dark-factory` with no args prints usage and exits 0
	assertParseArgs(t, []string{}, parseArgsResult{command: "help", args: []string{}})
}

func TestParseArgsHelpWord(t *testing.T) {
	t.Parallel()
	// Bare `help` word must be treated exactly like `--help`
	assertParseArgs(t, []string{"help"}, parseArgsResult{command: "help", args: []string{}})
	// The -debug flag is still extracted when combined with help
	assertParseArgs(
		t,
		[]string{"-debug", "help"},
		parseArgsResult{debug: true, command: "help", args: []string{}},
	)
	// `--help` continues to work (regression guard)
	assertParseArgs(t, []string{"--help"}, parseArgsResult{command: "help", args: []string{}})
}

func TestParseArgsPrompt(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"prompt", "list"},
		parseArgsResult{command: "prompt", subcommand: "list", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "status"},
		parseArgsResult{command: "prompt", subcommand: "status", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "retry"},
		parseArgsResult{command: "prompt", subcommand: "retry", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "approve", "042"},
		parseArgsResult{command: "prompt", subcommand: "approve", args: []string{"042"}},
	)
}

func TestParseArgsSpec(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"spec", "approve", "017"},
		parseArgsResult{command: "spec", subcommand: "approve", args: []string{"017"}},
	)
	assertParseArgs(
		t,
		[]string{"spec", "list"},
		parseArgsResult{command: "spec", subcommand: "list", args: []string{}},
	)
}

func TestParseArgsPromptHelp(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"prompt", "--help"},
		parseArgsResult{command: "prompt", subcommand: "--help", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "-h"},
		parseArgsResult{command: "prompt", subcommand: "-h", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt", "help"},
		parseArgsResult{command: "prompt", subcommand: "help", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"prompt"},
		parseArgsResult{command: "prompt", subcommand: "", args: []string{}},
	)
}

func TestParseArgsSpecHelp(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"spec", "--help"},
		parseArgsResult{command: "spec", subcommand: "--help", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"spec", "-h"},
		parseArgsResult{command: "spec", subcommand: "-h", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"spec", "help"},
		parseArgsResult{command: "spec", subcommand: "help", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"spec"},
		parseArgsResult{command: "spec", subcommand: "", args: []string{}},
	)
}

func TestParseArgsUnknown(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"prompts"},
		parseArgsResult{command: "unknown", args: []string{"prompts"}},
	)
	assertParseArgs(
		t,
		[]string{"foo"},
		parseArgsResult{command: "unknown", args: []string{"foo"}},
	)
}

func TestParseArgsDebug(t *testing.T) {
	t.Parallel()
	assertParseArgs(
		t,
		[]string{"-debug", "prompt", "list"},
		parseArgsResult{debug: true, command: "prompt", subcommand: "list", args: []string{}},
	)
	assertParseArgs(
		t,
		[]string{"-debug", "run"},
		parseArgsResult{debug: true, command: "run", args: []string{}},
	)
}

func TestParseArgsAutoApprove(t *testing.T) {
	t.Parallel()
	// --auto-approve present sets autoApprove=true
	assertParseArgs(
		t,
		[]string{"run", "--auto-approve"},
		parseArgsResult{command: "run", args: []string{}, autoApprove: true},
	)
	// without --auto-approve, autoApprove defaults to false
	assertParseArgs(
		t,
		[]string{"run"},
		parseArgsResult{command: "run", args: []string{}, autoApprove: false},
	)
	// --auto-approve before "run" is also extracted
	assertParseArgs(
		t,
		[]string{"--auto-approve", "run"},
		parseArgsResult{command: "run", args: []string{}, autoApprove: true},
	)
	// --auto-approve combined with -debug
	assertParseArgs(
		t,
		[]string{"-debug", "run", "--auto-approve"},
		parseArgsResult{debug: true, command: "run", args: []string{}, autoApprove: true},
	)
}

func TestParseArgsSkipPreflight(t *testing.T) {
	t.Parallel()
	// flag after command
	assertParseArgs(t,
		[]string{"run", "--skip-preflight"},
		parseArgsResult{command: "run", args: []string{}, skipPreflight: true},
	)
	// flag before command (position-agnostic)
	assertParseArgs(t,
		[]string{"--skip-preflight", "run"},
		parseArgsResult{command: "run", args: []string{}, skipPreflight: true},
	)
	// flag for daemon
	assertParseArgs(t,
		[]string{"daemon", "--skip-preflight"},
		parseArgsResult{command: "daemon", args: []string{}, skipPreflight: true},
	)
	// without flag, skipPreflight defaults to false
	assertParseArgs(t,
		[]string{"run"},
		parseArgsResult{command: "run", args: []string{}, skipPreflight: false},
	)
	// combined with other flags
	assertParseArgs(
		t,
		[]string{"-debug", "run", "--auto-approve", "--skip-preflight"},
		parseArgsResult{
			debug:         true,
			command:       "run",
			args:          []string{},
			autoApprove:   true,
			skipPreflight: true,
		},
	)
	// idempotent: flag passed twice
	assertParseArgs(t,
		[]string{"run", "--skip-preflight", "--skip-preflight"},
		parseArgsResult{command: "run", args: []string{}, skipPreflight: true},
	)
}

func TestParseArgsModel(t *testing.T) {
	t.Parallel()

	// --model after command
	assertParseArgs(t,
		[]string{"run", "--model", "claude-opus-4-7"},
		parseArgsResult{command: "run", args: []string{}, model: "claude-opus-4-7"},
	)
	// --model before command
	assertParseArgs(t,
		[]string{"--model", "claude-haiku-4-5", "daemon"},
		parseArgsResult{command: "daemon", args: []string{}, model: "claude-haiku-4-5"},
	)
	// no flag → model is empty string
	assertParseArgs(t,
		[]string{"run"},
		parseArgsResult{command: "run", args: []string{}, model: ""},
	)
	// combined with other flags
	assertParseArgs(
		t,
		[]string{"run", "--model", "claude-sonnet-4-6", "--skip-preflight"},
		parseArgsResult{
			command:       "run",
			args:          []string{},
			model:         "claude-sonnet-4-6",
			skipPreflight: true,
		},
	)
	// model with colon (other provider)
	assertParseArgs(t,
		[]string{"run", "--model", "qwen3.6:35b-a3b"},
		parseArgsResult{command: "run", args: []string{}, model: "qwen3.6:35b-a3b"},
	)
}
