// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/doctor"
	"github.com/bborbe/dark-factory/pkg/healthcheckgate"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/pipelinegate"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// createDoctorDeps builds the doctor Deps shared by `dark-factory doctor` and
// the daemon-startup pipeline gate — one detector definition, two callers.
// createPromptManager and spec.NewLister are pure constructors, so the extra
// construction is intentional and harmless.
func createDoctorDeps(
	cfg config.Config,
	verifyingStaleHours int,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) doctor.Deps {
	promptManager, _ := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)

	specLister := spec.NewLister(
		currentDateTimeGetter,
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		cfg.Specs.RejectedDir,
	)

	return doctor.Deps{
		SpecsInboxDir:         cfg.Specs.InboxDir,
		SpecsInProgressDir:    cfg.Specs.InProgressDir,
		SpecsCompletedDir:     cfg.Specs.CompletedDir,
		SpecsRejectedDir:      cfg.Specs.RejectedDir,
		PromptsInboxDir:       cfg.Prompts.InboxDir,
		PromptsInProgressDir:  cfg.Prompts.InProgressDir,
		PromptsCompletedDir:   cfg.Prompts.CompletedDir,
		PromptsCancelledDir:   cfg.Prompts.CancelledDir,
		PromptsRejectedDir:    cfg.Prompts.RejectedDir,
		SpecLister:            specLister,
		PromptManager:         promptManager,
		CurrentDateTimeGetter: currentDateTimeGetter,
		VerifyingStaleHours:   verifyingStaleHours,
	}
}

// CreateDoctorCommand creates a DoctorCommand with all required dependencies.
func CreateDoctorCommand(
	ctx context.Context,
	cfg config.Config,
	verifyingStaleHours int,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) cmd.DoctorCommand {
	promptManager, releaser := createPromptManager(
		cfg.Prompts.InboxDir,
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Prompts.CancelledDir,
		currentDateTimeGetter,
	)

	autoCompleter := spec.NewAutoCompleter(
		cfg.Prompts.InProgressDir,
		cfg.Prompts.CompletedDir,
		cfg.Specs.InboxDir,
		cfg.Specs.InProgressDir,
		cfg.Specs.CompletedDir,
		currentDateTimeGetter,
		cfg.ProjectName,
		notifier.NewMultiNotifier(),
		promptManager,
	)

	deps := createDoctorDeps(cfg, verifyingStaleHours, currentDateTimeGetter)

	checker := doctor.NewChecker(deps)

	fixer := doctor.NewFixer(doctor.FixerDeps{
		Deps:            deps,
		AutoCompleter:   autoCompleter,
		Mover:           releaser,
		FileLockFactory: lock.NewDirLock,
	})

	return cmd.NewDoctorCommand(checker, fixer)
}

// CreateHealthcheckGate builds the daemon-startup healthcheck gate. The gate's
// disabled/skip/cache logic lives in healthcheckgate.gate.Check; this factory only
// constructs collaborators (the underlying HealthcheckCommand, the file cache, the
// cache key, the notifier) and passes them in.
//
// os.UserHomeDir error: failure is logged, then the gate falls back to a CWD-relative
// cache path. The cache is non-secret, non-critical, and write failures are tolerated
// by design; surfacing the error here is enough — refusing to start the daemon over a
// cache-dir resolution miss would be worse than the silent fallback.
func CreateHealthcheckGate(
	ctx context.Context,
	cfg config.Config,
	skipHealthcheck bool,
	projectName string,
	n notifier.Notifier,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) healthcheckgate.Gate {
	cacheKey := healthcheckgate.CacheKey(
		cfg.ContainerImage,
		projectName,
		cfg.ParsedHealthcheckInterval(),
	)
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn(
			"healthcheck cache: os.UserHomeDir failed; cache root will be CWD-relative",
			"error",
			err,
		)
	}
	cacheRoot := filepath.Join(home, ".dark-factory", "healthcheck-cache")
	return healthcheckgate.NewGate(
		healthcheckEnabledForBackend(cfg),
		skipHealthcheck,
		cfg.ParsedHealthcheckInterval(),
		CreateHealthcheckCommand(ctx, cfg, currentDateTimeGetter),
		cacheKey,
		healthcheckgate.NewFileCache(cacheRoot),
		n,
		projectName,
		currentDateTimeGetter,
	)
}

// CreatePipelineGate builds the daemon-startup pipeline gate. The gate's
// enabled/skip logic and the doctor-checker call live in pipelinegate.gate.Check;
// this factory only constructs collaborators and passes them in.
//
// verifyingStaleHours is passed as 0: pkg/doctor's detectVerifyingStale treats a
// non-positive value as its default (24h), and `--verifying-stale-hours` is a
// doctor-only CLI flag that does not apply to the daemon gate.
// Arming is per-repo via the `pipelineGate:` config key and bypassable for a
// single run via --skip-pipeline-gate. Both previously did not exist: enabled
// was wired to the pipelinegate.DefaultEnabled compile-time constant and skip
// was a literal false, which made arming a global, release-gated flip with no
// runtime escape hatch — the reverse of how the sibling merge gate arms, one
// repo at a time via rulesets.
func CreatePipelineGate(
	cfg config.Config,
	skipPipelineGate bool,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) pipelinegate.Gate {
	return pipelinegate.NewGate(
		cfg.PipelineGateEnabledValue(),
		skipPipelineGate,
		doctor.NewChecker(createDoctorDeps(cfg, 0, currentDateTimeGetter)),
	)
}

// healthcheckEnabledForBackend reports whether the daemon-startup healthcheck
// gate should run. Under backend: local the docker probes are meaningless (no
// docker daemon is required — spec 104), so the gate is always disabled;
// otherwise it follows the configured value.
func healthcheckEnabledForBackend(cfg config.Config) bool {
	if cfg.Backend == config.BackendLocal {
		return false
	}
	return cfg.HealthcheckEnabledValue()
}
