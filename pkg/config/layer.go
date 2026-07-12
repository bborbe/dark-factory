// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"strconv"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/globalconfig"
)

// SupportedSetKeys is the authoritative list of yaml-backed user-pref keys
// accepted by --set. Adding a new yaml field requires a new entry here.
var SupportedSetKeys = []string{
	"hideGit",
	"autoRelease",
	"dirtyFileThreshold",

	"model",
	"maxContainers",
	"backend",

	"workflow",
	"pr",
	"autoMerge",
	"autoGeneratePrompts",
}

// ApplyGlobalOverrides applies global config values for the layered user-pref
// fields into cfg, but only where the project config did not explicitly set
// the field. Fields the project explicitly set (non-nil pointer in proj) are
// left untouched.
func ApplyGlobalOverrides(
	cfg *Config,
	global globalconfig.GlobalConfig,
	proj LayeredProjectOverrides,
) {
	if global.Model != nil && proj.Model == nil {
		cfg.Model = *global.Model
	}
	if global.HideGit != nil && proj.HideGit == nil {
		cfg.HideGit = *global.HideGit
	}
	if global.AutoRelease != nil && proj.AutoRelease == nil {
		cfg.AutoRelease = *global.AutoRelease
	}
	if global.DirtyFileThreshold != nil && proj.DirtyFileThreshold == nil {
		cfg.DirtyFileThreshold = *global.DirtyFileThreshold
	}
	if global.AutoApprovePrompts != nil && proj.AutoApprovePrompts == nil {
		cfg.AutoApprovePrompts = *global.AutoApprovePrompts
	}
	if global.AutoGeneratePrompts != nil && proj.AutoGeneratePrompts == nil {
		cfg.AutoGeneratePrompts = *global.AutoGeneratePrompts
	}
	if global.Backend != nil && proj.Backend == nil {
		cfg.Backend = Backend(*global.Backend)
	}
	// Env layering: project entries win on key collision, global fills the
	// rest. Without this, global env (typically `ANTHROPIC_BASE_URL` +
	// `ANTHROPIC_AUTH_TOKEN` for alt-provider configs) never reaches the
	// container — claude defaults to anthropic.com with the global model
	// name and silently fails. cfg.Env at this point is the project's env
	// only (cfg was constructed from the project YAML), so merging global
	// underneath gives project-wins-on-collision semantics.
	//
	// Reserved keys (ANTHROPIC_MODEL, YOLO_PROMPT_FILE) are stripped from
	// the merge: they're set by the executor/healthcheck factory from
	// cfg.Model and cfg.Prompts at container-launch time, and `validateEnv`
	// (called by Config.Validate after this layering) rejects them in
	// cfg.Env. Operators commonly mirror `model:` into `env: ANTHROPIC_MODEL:`
	// in their global config as habit — silently drop the duplicate at
	// merge so daemon/run startup doesn't break on the resulting validation.
	globalEnvForMerge := stripReservedEnvKeys(global.Env)
	cfg.Env = MergeEnv(globalEnvForMerge, cfg.Env)
}

// stripReservedEnvKeys returns a copy of env with `reservedEnvKeys`
// (ANTHROPIC_MODEL, YOLO_PROMPT_FILE) removed. Used during global→project
// env layering so global-config duplicates of model-derived env don't trip
// validateEnv. Returns nil for a nil input.
func stripReservedEnvKeys(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
keys:
	for k, v := range env {
		for _, reserved := range reservedEnvKeys {
			if k == reserved {
				continue keys
			}
		}
		out[k] = v
	}
	return out
}

// ComputeFieldSources determines which config layer provided each of the
// layered user-pref fields.
// Rules: global wins over default; project wins over global.
// "arg" source is not set here — it is set via ApplyArgOverrides /
// ApplySetOverrides when CLI flags override.
//
//nolint:funlen // table of 1-line assignments by source layer
func ComputeFieldSources(
	global globalconfig.GlobalConfig,
	proj LayeredProjectOverrides,
) FieldSources {
	s := FieldSources{
		HideGit:             "default",
		AutoRelease:         "default",
		DirtyFileThreshold:  "default",
		Model:               "default",
		Workflow:            "default",
		PR:                  "default",
		AutoMerge:           "default",
		AutoApprovePrompts:  "default",
		AutoGeneratePrompts: "default",
		HealthcheckEnabled:  "default",
		HealthcheckInterval: "default",
		Backend:             "default",
	}
	if global.Model != nil {
		s.Model = "global"
	}
	if global.HideGit != nil {
		s.HideGit = "global"
	}
	if global.AutoRelease != nil {
		s.AutoRelease = "global"
	}
	if global.DirtyFileThreshold != nil {
		s.DirtyFileThreshold = "global"
	}
	if global.AutoApprovePrompts != nil {
		s.AutoApprovePrompts = "global"
	}
	if global.AutoGeneratePrompts != nil {
		s.AutoGeneratePrompts = "global"
	}
	if global.Backend != nil {
		s.Backend = "global"
	}
	if proj.Model != nil {
		s.Model = "project"
	}
	if proj.HideGit != nil {
		s.HideGit = "project"
	}
	if proj.AutoRelease != nil {
		s.AutoRelease = "project"
	}
	if proj.DirtyFileThreshold != nil {
		s.DirtyFileThreshold = "project"
	}
	if proj.AutoApprovePrompts != nil {
		s.AutoApprovePrompts = "project"
	}
	if proj.AutoGeneratePrompts != nil {
		s.AutoGeneratePrompts = "project"
	}
	if proj.Backend != nil {
		s.Backend = "project"
	}
	if proj.MaxContainers != nil {
		s.MaxContainers = "project"
	}
	if proj.Workflow != nil {
		s.Workflow = "project"
	}
	if proj.PR != nil {
		s.PR = "project"
	}
	if proj.AutoMerge != nil {
		s.AutoMerge = "project"
	}
	if proj.HealthcheckEnabled != nil {
		s.HealthcheckEnabled = "project"
	}
	if proj.HealthcheckInterval != nil {
		s.HealthcheckInterval = "project"
	}
	return s
}

// ApplyArgOverrides validates command-gate rules and applies --model CLI flag
// override to cfg and sources. model is the extracted flag value from
// ParseArgs (empty = not set).
func ApplyArgOverrides(
	ctx context.Context,
	cfg *Config,
	sources *FieldSources,
	command string,
	model string,
) error {
	if model != "" && command != "run" && command != "daemon" {
		return errors.Errorf(ctx, "unknown flag: --model")
	}
	if model != "" {
		if err := ValidateModelArg(ctx, model); err != nil {
			return err
		}
		cfg.Model = model
		sources.Model = "arg"
	}
	return nil
}

// validateModelArg validates a --model flag value against the shared model
// identifier regex. Returns an error if the value contains invalid characters.
func ValidateModelArg(ctx context.Context, model string) error {
	if !globalconfig.ModelRegex.MatchString(model) {
		return errors.Errorf(
			ctx,
			"--model value %q does not match required pattern %s",
			model,
			globalconfig.ModelPattern,
		)
	}
	return nil
}

// ApplySetOverrides validates command-gate rules and applies --set key=value
// overrides to cfg and sources. Valid only for "run" and "daemon" commands.
func ApplySetOverrides(
	ctx context.Context,
	cfg *Config,
	sources *FieldSources,
	command string,
	setOverrides map[string]string,
) error {
	if len(setOverrides) == 0 {
		return nil
	}
	switch command {
	case "run", "daemon":
		// valid
	default:
		return errors.Errorf(ctx, "unknown flag: --set")
	}
	for key, value := range setOverrides {
		if err := ApplyOneSetOverride(ctx, cfg, sources, key, value); err != nil {
			return err
		}
	}
	return nil
}

// applyOneSetOverride applies a single --set key=value entry with type
// coercion and validation.
//
//nolint:funlen // switch over every supported --set key; splitting hides which keys are wired where
func ApplyOneSetOverride(
	ctx context.Context,
	cfg *Config,
	sources *FieldSources,
	key, value string,
) error {
	switch key {
	case "hideGit":
		b, err := parseStrictBool(ctx, key, value)
		if err != nil {
			return err
		}
		cfg.HideGit = b
		sources.HideGit = "arg"
	case "autoGeneratePrompts":
		b, err := parseStrictBool(ctx, key, value)
		if err != nil {
			return err
		}
		cfg.AutoGeneratePrompts = b
		sources.AutoGeneratePrompts = "arg"
	case "autoRelease":
		b, err := parseStrictBool(ctx, key, value)
		if err != nil {
			return err
		}
		cfg.AutoRelease = b
		sources.AutoRelease = "arg"
	case "dirtyFileThreshold":
		n, err := strconv.Atoi(value)
		if err != nil {
			return errors.Errorf(ctx, "--set %s: invalid integer %q", key, value)
		}
		if n < 0 {
			return errors.Errorf(ctx, "--set %s: dirtyFileThreshold must be >= 0, got %d", key, n)
		}
		cfg.DirtyFileThreshold = n
		sources.DirtyFileThreshold = "arg"
	case "model":
		if err := ValidateModelArg(ctx, value); err != nil {
			return err
		}
		cfg.Model = value
		sources.Model = "arg"
	case "maxContainers":
		n, err := strconv.Atoi(value)
		if err != nil {
			return errors.Errorf(ctx, "--set %s: invalid integer %q", key, value)
		}
		if n < 1 {
			return errors.Errorf(ctx, "--set %s: maxContainers must be >= 1, got %d", key, n)
		}
		cfg.MaxContainers = n
		sources.MaxContainers = "arg"
	case "backend":
		b := Backend(value)
		if err := b.Validate(ctx); err != nil {
			return err
		}
		cfg.Backend = b
		sources.Backend = "arg"
	case "workflow", "pr", "autoMerge":
		return applyDeliverySetOverride(ctx, cfg, sources, key, value)
	default:
		return errors.Errorf(
			ctx,
			"unknown config key: %s (supported: %s)",
			key,
			strings.Join(SupportedSetKeys, ", "),
		)
	}
	return nil
}

// applyDeliverySetOverride handles --set for workflow, pr, and autoMerge keys.
func applyDeliverySetOverride(
	ctx context.Context,
	cfg *Config,
	sources *FieldSources,
	key, value string,
) error {
	switch key {
	case "workflow":
		// Reject the legacy "pr" enum value at the arg layer. The yaml loader
		// maps it to workflow: clone + pr: true; the arg layer intentionally
		// does not reproduce that mapping.
		if value == string(WorkflowPR) {
			return errors.Errorf(
				ctx,
				"legacy workflow value %q not accepted via --set; use --set workflow=clone --set pr=true",
				value,
			)
		}
		w := Workflow(value)
		if err := w.Validate(ctx); err != nil {
			return err
		}
		cfg.Workflow = w
		sources.Workflow = "arg"
	case "pr":
		b, err := parseStrictBool(ctx, key, value)
		if err != nil {
			return err
		}
		cfg.PR = b
		sources.PR = "arg"
	default: // "autoMerge"
		b, err := parseStrictBool(ctx, key, value)
		if err != nil {
			return err
		}
		cfg.AutoMerge = b
		sources.AutoMerge = "arg"
	}
	return nil
}

// parseStrictBool parses a --set bool value. Only "true" and "false" are
// accepted. strconv.ParseBool is intentionally NOT used — it accepts
// 1/0/yes/no which would diverge from yaml semantics.
func parseStrictBool(ctx context.Context, key, value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.Errorf(
			ctx,
			"--set %s: invalid bool %q, expected true or false",
			key,
			value,
		)
	}
}
