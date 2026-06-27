// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package claudeargv is the single source of truth for the env keys
// the claude-yolo image's /usr/local/bin/entrypoint.sh understands.
//
// Two call sites — the prompt executor (pkg/executor) and the
// healthcheck claude probe (pkg/cmd/healthcheck) — each build a
// per-invocation EnvOverlay map for launchpolicy.Extras. Before this
// package existed both sites constructed the map literal independently,
// duplicating the string literals "ANTHROPIC_MODEL", "YOLO_PROMPT",
// "YOLO_PROMPT_FILE", "YOLO_OUTPUT". That drift caused real production
// regressions (silent minimax failures, mismatched flags) — see the
// 2026-06-27 architecture re-audit finding behind this consolidation.
//
// The `hotpath-claudeargv-check` precommit gate rejects raw string
// literals matching the above keys anywhere in pkg/ except this package
// (which OWNS them) — so adding a fourth call site cannot reintroduce
// the drift class.
package claudeargv

// Env keys understood by the claude-yolo image's entrypoint.sh.
// Adding a new key here is the only sanctioned way for a call site to
// pass new env-driven options to claude.
const (
	// EnvAnthropicModel sets `--model "$VALUE"` on the claude invocation.
	// Empty value → entrypoint.sh falls back to the YOLO_MODEL / sonnet
	// default.
	EnvAnthropicModel = "ANTHROPIC_MODEL"

	// EnvYoloPromptFile is the path INSIDE the container to a mounted
	// prompt file (the executor mounts /tmp/prompt.md). entrypoint.sh
	// picks this branch when both this and EnvYoloPrompt are set.
	EnvYoloPromptFile = "YOLO_PROMPT_FILE"

	// EnvYoloPrompt is the inline prompt string. entrypoint.sh writes
	// it to a temp file inside the container before invoking claude.
	// Used by the healthcheck probe to avoid mounting a file.
	EnvYoloPrompt = "YOLO_PROMPT"

	// EnvYoloOutput selects the claude output format applied by
	// entrypoint.sh: "print" (raw text), "json" (stream-json JSONL),
	// or anything else / unset (stream-json piped through the formatter).
	EnvYoloOutput = "YOLO_OUTPUT"
)

// Output mode constants for the YoloOutput field.
const (
	OutputJSON   = "json"
	OutputPrint  = "print"
	OutputStream = "stream"
)

// Options describes a claude run for entrypoint.sh consumption.
// Empty fields are dropped from the resulting EnvOverlay map.
type Options struct {
	// Model is the value for ANTHROPIC_MODEL.
	Model string
	// Output is the value for YOLO_OUTPUT.
	Output string
	// PromptFile is the value for YOLO_PROMPT_FILE. Use this when a
	// prompt is mounted into the container (executor path).
	PromptFile string
	// Prompt is the value for YOLO_PROMPT. Use this when passing the
	// prompt inline (probe path).
	Prompt string
}

// EnvOverlay returns the env map for launchpolicy.Extras.EnvOverlay.
// Keys whose Options field is empty are NOT emitted, so callers can use
// the zero value of any field to mean "use entrypoint.sh's default for
// this key".
func EnvOverlay(opts Options) map[string]string {
	env := map[string]string{}
	if opts.Model != "" {
		env[EnvAnthropicModel] = opts.Model
	}
	if opts.Output != "" {
		env[EnvYoloOutput] = opts.Output
	}
	if opts.PromptFile != "" {
		env[EnvYoloPromptFile] = opts.PromptFile
	}
	if opts.Prompt != "" {
		env[EnvYoloPrompt] = opts.Prompt
	}
	return env
}

// ReservedKeys returns the env keys that pkg/config.validateEnv rejects
// in operator-supplied `env:` blocks. These keys are owned by the
// executor / factory and would be silently overridden if operators set
// them — explicit rejection at config-validate time is friendlier than
// silent override.
//
// Two of the four keys (YOLO_PROMPT, YOLO_OUTPUT) are NOT reserved:
//   - YOLO_OUTPUT is operator-settable in global env (override default
//     stream-json formatting).
//   - YOLO_PROMPT is the healthcheck probe's path, never set by operator
//     env in production prompt runs.
//
// If the reservation policy changes, update this list and validateEnv
// picks it up automatically.
func ReservedKeys() []string {
	return []string{EnvYoloPromptFile, EnvAnthropicModel}
}
