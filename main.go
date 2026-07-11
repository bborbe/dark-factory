// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	"gopkg.in/yaml.v3"

	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/factory"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/version"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	setOverrides, filteredArgs, err := parseSetFlags(ctx, os.Args[1:])
	if err != nil {
		return err
	}

	debug, command, subcommand, args, autoApprove, skipPreflight, model, skipHealthcheck := ParseArgs(
		filteredArgs,
	)

	switch command {
	case "help":
		printHelp(os.Stdout)
		return nil
	case "version":
		fmt.Fprintf(os.Stdout, "dark-factory %s\n", version.Version)
		return nil
	case "unknown":
		cmdName := ""
		if len(args) > 0 {
			cmdName = args[0]
		}
		fmt.Fprintf(os.Stderr, "Run 'dark-factory help' for usage.\n")
		return errors.Errorf(ctx, "unknown command: %q", cmdName)
	}

	// Intercept --help/-h before loading config or acquiring locks
	if containsHelpFlag(args) || containsHelpFlag([]string{subcommand}) {
		printCommandHelp(command)
		return nil
	}

	initLogging(debug)

	projectRoot, err := project.FindRoot(ctx)
	if err != nil {
		return err
	}
	slog.Debug("resolved project root", "root", projectRoot)
	if err := os.Chdir(projectRoot); err != nil {
		return errors.Wrap(ctx, err, "chdir to project root")
	}

	loadResult, err := config.LoadWithOverrides(ctx)
	if err != nil {
		return err
	}
	cfg := loadResult.Config

	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		return err
	}
	config.ApplyGlobalOverrides(&cfg, globalCfg, loadResult.Overrides)
	sources := config.ComputeFieldSources(globalCfg, loadResult.Overrides)
	if err := config.ApplyArgOverrides(ctx, &cfg, &sources, command, model); err != nil {
		return err
	}
	if err := config.ApplySetOverrides(ctx, &cfg, &sources, command, setOverrides); err != nil {
		return err
	}
	if command == "run" || command == "daemon" {
		if err := cfg.Validate(ctx); err != nil {
			return err
		}
	}

	currentDateTimeGetter := libtime.NewCurrentDateTime()
	return runCommand(
		ctx,
		cfg,
		command,
		subcommand,
		args,
		autoApprove,
		skipPreflight,
		skipHealthcheck,
		sources,
		currentDateTimeGetter,
	)
}

func initLogging(debug bool) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	slog.Info("dark-factory starting", "version", version.Version)
}

func validateSkipFlags(
	ctx context.Context,
	command string,
	skipPreflight, skipHealthcheck bool,
) error {
	if skipPreflight {
		switch command {
		case "run", "daemon":
			// valid
		default:
			return errors.Errorf(ctx, "unknown flag: --skip-preflight")
		}
	}
	if skipHealthcheck {
		switch command {
		case "daemon":
			// valid
		default:
			return errors.Errorf(ctx, "unknown flag: --skip-healthcheck")
		}
	}
	return nil
}

func printCommandHelp(command string) {
	switch command {
	case "run":
		printRunHelp()
	case "daemon":
		printDaemonHelp()
	case "kill":
		printKillHelp()
	case "status":
		printStatusHelp()
	case "list":
		printListHelp()
	case "config":
		printConfigHelp()
	case "prompt":
		printPromptHelp()
	case "spec":
		printSpecHelp()
	case "scenario":
		printScenarioHelp()
	case "doctor":
		cmd.DoctorHelp()
	case "healthcheck":
		cmd.HealthcheckHelp()
	}
}

func runCommand(
	ctx context.Context,
	cfg config.Config,
	command, subcommand string,
	args []string,
	autoApprove bool,
	skipPreflight bool,
	skipHealthcheck bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	if err := validateSkipFlags(ctx, command, skipPreflight, skipHealthcheck); err != nil {
		return err
	}
	switch command {
	case "prompt":
		return runPromptCommand(ctx, cfg, subcommand, args, currentDateTimeGetter)
	case "spec":
		return runSpecCommand(ctx, cfg, subcommand, args, currentDateTimeGetter)
	case "scenario":
		return runScenarioCommand(ctx, cfg, subcommand, args)
	case "status":
		return runStatusCommand(ctx, cfg, args, currentDateTimeGetter)
	case "list":
		if err := validateListArgs(ctx, args, printListHelp); err != nil {
			return err
		}
		return factory.CreateCombinedListCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "config":
		if err := validateNoArgs(ctx, args, printConfigHelp); err != nil {
			return err
		}
		return printConfig(ctx, cfg)
	case "kill":
		if err := validateNoArgs(ctx, args, printKillHelp); err != nil {
			return err
		}
		return factory.CreateKillCommand(cfg).Run(ctx, args)
	case "doctor":
		if err := validateDoctorArgs(ctx, args); err != nil {
			return err
		}
		hours, remaining, err := extractVerifyingStaleHours(ctx, args)
		if err != nil {
			return err
		}
		return factory.CreateDoctorCommand(ctx, cfg, hours, currentDateTimeGetter).
			Run(ctx, remaining)
	case "healthcheck":
		return factory.CreateHealthcheckCommand(ctx, cfg, currentDateTimeGetter).
			Run(ctx, args)
	case "run":
		return runRunCommand(
			ctx,
			cfg,
			args,
			autoApprove,
			skipPreflight,
			sources,
			currentDateTimeGetter,
		)
	case "daemon":
		return runDaemonCommand(
			ctx,
			cfg,
			args,
			skipPreflight,
			skipHealthcheck,
			sources,
			currentDateTimeGetter,
		)
	default:
		return errors.Errorf(ctx, "unknown command: %s", command)
	}
}

func runStatusCommand(
	ctx context.Context,
	cfg config.Config,
	args []string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	n, remaining, err := extractMaxContainers(ctx, args)
	if err != nil {
		return err
	}
	if n > 0 {
		cfg.MaxContainers = n
	}
	if err := validateNoArgs(ctx, remaining, printStatusHelp); err != nil {
		return err
	}
	return factory.CreateCombinedStatusCommand(ctx, cfg, currentDateTimeGetter).Run(ctx, remaining)
}

func runRunCommand(
	ctx context.Context,
	cfg config.Config,
	args []string,
	autoApprove bool,
	skipPreflight bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	n, remaining, err := extractMaxContainers(ctx, args)
	if err != nil {
		return err
	}
	if n > 0 {
		cfg.MaxContainers = n
	}
	autoApprovePrompts, remaining := extractAutoApprovePrompts(remaining)
	if autoApprovePrompts {
		cfg.AutoApprovePrompts = true
		sources.AutoApprovePrompts = "arg"
	}
	if err := validateNoArgs(ctx, remaining, printRunHelp); err != nil {
		return err
	}
	if skipPreflight {
		slog.Info("preflight: baseline check disabled for this invocation (--skip-preflight flag)")
	}
	runErr := factory.CreateOneShotRunner(ctx, cfg, version.Version, autoApprove, skipPreflight, sources, currentDateTimeGetter).
		Run(ctx)
	if stderrors.Is(runErr, preflightconditions.ErrPreflightFailed) {
		slog.Error(
			"preflight baseline broken — dark-factory exiting. Fix the tree (e.g. run the failing command manually), then restart dark-factory.",
		)
	}
	return runErr
}

func runDaemonCommand(
	ctx context.Context,
	cfg config.Config,
	args []string,
	skipPreflight bool,
	skipHealthcheck bool,
	sources config.FieldSources,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	n, remaining, err := extractMaxContainers(ctx, args)
	if err != nil {
		return err
	}
	if n > 0 {
		cfg.MaxContainers = n
	}
	autoApprovePrompts, remaining := extractAutoApprovePrompts(remaining)
	if autoApprovePrompts {
		cfg.AutoApprovePrompts = true
		sources.AutoApprovePrompts = "arg"
	}
	if err := validateNoArgs(ctx, remaining, printDaemonHelp); err != nil {
		return err
	}
	if skipPreflight {
		slog.Info("preflight: baseline check disabled for this invocation (--skip-preflight flag)")
	}
	// --skip-healthcheck is logged authoritatively by gate.Check; no pre-gate log needed
	// (would emit a duplicate "healthcheck skipped via --skip-healthcheck" line).
	runErr := factory.CreateRunner(ctx, cfg, version.Version, skipPreflight, skipHealthcheck, sources, currentDateTimeGetter).
		Run(ctx)
	if stderrors.Is(runErr, preflightconditions.ErrPreflightFailed) {
		slog.Error(
			"preflight baseline broken — dark-factory exiting. Fix the tree (e.g. run the failing command manually), then restart dark-factory.",
		)
	}
	return runErr
}

func runPromptCommand(
	ctx context.Context,
	cfg config.Config,
	subcommand string,
	args []string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	switch subcommand {
	case "", "--help", "-h", "help":
		printPromptHelp()
		return nil
	case "status":
		if err := validateNoArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateStatusCommand(ctx, cfg, currentDateTimeGetter).Run(ctx, args)
	case "list":
		if err := validateListArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateListCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "approve":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateApproveCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "requeue":
		if err := validateRequeueArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateRequeueCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "cancel":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateCancelCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "retry":
		if err := validateNoArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateRequeueCommand(cfg, currentDateTimeGetter).
			Run(ctx, []string{"--failed"})
	case "complete":
		forceRelease, remaining := extractForceRelease(args)
		if err := validateOneArg(ctx, remaining, printPromptHelp); err != nil {
			return err
		}
		return factory.CreatePromptCompleteCommand(
			ctx,
			cfg,
			currentDateTimeGetter,
			forceRelease,
		).Run(ctx, remaining)
	case "unapprove":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateUnapproveCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "reject":
		return factory.CreateRejectCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "show":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreatePromptShowCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	default:
		return errors.Errorf(ctx, "unknown prompt subcommand: %s", subcommand)
	}
}

func runSpecCommand(
	ctx context.Context,
	cfg config.Config,
	subcommand string,
	args []string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	switch subcommand {
	case "", "--help", "-h", "help":
		printSpecHelp()
		return nil
	case "list":
		if err := validateListArgs(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecListCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "status":
		if err := validateNoArgs(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecStatusCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "approve":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecApproveCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "unapprove":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecUnapproveCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "reject":
		return factory.CreateSpecRejectCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "complete":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecCompleteCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "mark-prompted":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecMarkPromptedCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	case "show":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecShowCommand(cfg, currentDateTimeGetter).Run(ctx, args)
	default:
		return errors.Errorf(ctx, "unknown spec subcommand: %s", subcommand)
	}
}

func runScenarioCommand(
	ctx context.Context,
	cfg config.Config,
	subcommand string,
	args []string,
) error {
	switch subcommand {
	case "", "--help", "-h", "help":
		printScenarioHelp()
		return nil
	case "list":
		if err := validateNoArgs(ctx, args, printScenarioHelp); err != nil {
			return err
		}
		return factory.CreateScenarioListCommand(cfg).Run(ctx, args)
	case "show":
		if err := validateOneArg(ctx, args, printScenarioHelp); err != nil {
			return err
		}
		return factory.CreateScenarioShowCommand(cfg).Run(ctx, args)
	case "status":
		if err := validateNoArgs(ctx, args, printScenarioHelp); err != nil {
			return err
		}
		return factory.CreateScenarioStatusCommand(cfg).Run(ctx, args)
	default:
		return errors.Errorf(ctx, "unknown scenario subcommand: %s", subcommand)
	}
}

// containsHelpFlag reports whether args contains --help, -help, or -h.
func containsHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-help" || arg == "-h" {
			return true
		}
	}
	return false
}

// validateListArgs returns an error if args contains anything other than the
// optional "--all" flag. Used by the list command dispatchers in main.go.
func validateListArgs(ctx context.Context, args []string, helpFn func()) error {
	for _, arg := range args {
		if arg == "--all" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "unknown flag: %q\n", arg)
		} else {
			fmt.Fprintf(os.Stderr, "unknown argument: %q\n", arg)
		}
		helpFn()
		return errors.Errorf(ctx, "unexpected argument: %q", arg)
	}
	return nil
}

// validateNoArgs returns an error if args is non-empty.
// Unknown flags (starting with -) are reported as "unknown flag", others as "unknown argument".
func validateNoArgs(ctx context.Context, args []string, helpFn func()) error {
	if len(args) == 0 {
		return nil
	}
	arg := args[0]
	if strings.HasPrefix(arg, "-") {
		fmt.Fprintf(os.Stderr, "unknown flag: %q\n", arg)
	} else {
		fmt.Fprintf(os.Stderr, "unknown argument: %q\n", arg)
	}
	helpFn()
	return errors.Errorf(ctx, "unexpected argument: %q", arg)
}

// validateOneArg returns an error if args does not contain exactly one positional argument.
func validateOneArg(ctx context.Context, args []string, helpFn func()) error {
	// Reject unknown flags first
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "unknown flag: %q\n", arg)
			helpFn()
			return errors.Errorf(ctx, "unknown flag: %q", arg)
		}
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "missing required argument\n")
		helpFn()
		return errors.Errorf(ctx, "missing required argument")
	}
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "unknown argument: %q\n", args[1])
		helpFn()
		return errors.Errorf(ctx, "unexpected argument: %q", args[1])
	}
	return nil
}

// validateRequeueArgs validates args for the requeue subcommand:
// exactly one positional arg (slug) or the --failed flag, but not both.
func validateRequeueArgs(ctx context.Context, args []string, helpFn func()) error {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "--failed" {
			fmt.Fprintf(os.Stderr, "unknown flag: %q\n", arg)
			helpFn()
			return errors.Errorf(ctx, "unknown flag: %q", arg)
		}
	}
	positional := 0
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			positional++
		}
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "missing required argument\n")
		helpFn()
		return errors.Errorf(ctx, "missing required argument")
	}
	if positional > 1 {
		fmt.Fprintf(os.Stderr, "too many arguments\n")
		helpFn()
		return errors.Errorf(ctx, "too many arguments")
	}
	return nil
}

// extractMaxContainers removes --max-containers N from args and returns the value (0 = not set).
// Returns an error if the value is missing, not an integer, or < 1.
func extractMaxContainers(ctx context.Context, args []string) (int, []string, error) {
	for i, arg := range args {
		if arg != "--max-containers" {
			continue
		}
		if i+1 >= len(args) {
			return 0, nil, errors.Errorf(ctx, "--max-containers requires a value")
		}
		n, err := strconv.Atoi(args[i+1])
		if err != nil {
			return 0, nil, errors.Errorf(
				ctx,
				"--max-containers value must be an integer, got %q",
				args[i+1],
			)
		}
		if n < 1 {
			return 0, nil, errors.Errorf(ctx, "--max-containers value must be >= 1, got %d", n)
		}
		remaining := make([]string, 0, len(args)-2)
		remaining = append(remaining, args[:i]...)
		remaining = append(remaining, args[i+2:]...)
		return n, remaining, nil
	}
	return 0, args, nil
}

// extractAutoApprovePrompts removes --auto-approve-prompts from args and reports whether it was set.
// The flag is a presence flag: its appearance means true. No value argument is consumed.
func extractAutoApprovePrompts(args []string) (bool, []string) {
	for i, arg := range args {
		if arg != "--auto-approve-prompts" {
			continue
		}
		remaining := make([]string, 0, len(args)-1)
		remaining = append(remaining, args[:i]...)
		remaining = append(remaining, args[i+1:]...)
		return true, remaining
	}
	return false, args
}

// extractForceRelease removes --release from args and reports whether it was set.
// The flag is a presence flag: its appearance means true. No value argument is consumed.
func extractForceRelease(args []string) (bool, []string) {
	for i, arg := range args {
		if arg != "--release" {
			continue
		}
		remaining := make([]string, 0, len(args)-1)
		remaining = append(remaining, args[:i]...)
		remaining = append(remaining, args[i+1:]...)
		return true, remaining
	}
	return false, args
}

// extractVerifyingStaleHours extracts --verifying-stale-hours=N from args and returns the value (default 24).
func extractVerifyingStaleHours(ctx context.Context, args []string) (int, []string, error) {
	for i, arg := range args {
		if !strings.HasPrefix(arg, "--verifying-stale-hours=") {
			continue
		}
		value := strings.TrimPrefix(arg, "--verifying-stale-hours=")
		if value == "" {
			return 0, nil, errors.Errorf(ctx, "--verifying-stale-hours requires a value")
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, nil, errors.Errorf(
				ctx,
				"--verifying-stale-hours value must be a positive integer, got %q",
				value,
			)
		}
		if n < 1 {
			return 0, nil, errors.Errorf(
				ctx,
				"--verifying-stale-hours value must be a positive integer, got %d",
				n,
			)
		}
		remaining := make([]string, 0, len(args)-1)
		remaining = append(remaining, args[:i]...)
		remaining = append(remaining, args[i+1:]...)
		return n, remaining, nil
	}
	return 24, args, nil
}

// validateDoctorArgs rejects unknown flags for the doctor command.
func validateDoctorArgs(ctx context.Context, args []string) error {
	for _, arg := range args {
		switch arg {
		case "--fix", "--yes", "--help", "-h":
			continue
		default:
			if strings.HasPrefix(arg, "--verifying-stale-hours=") {
				continue
			}
			return errors.Errorf(ctx, "unknown flag: %q", arg)
		}
	}
	return nil
}

// parseSetFlags scans rawArgs for --set key=value occurrences, collects them into a
// map (last occurrence wins for duplicates), and returns filtered args with --set
// entries removed. Call before ParseArgs to avoid contaminating the arg list.
func parseSetFlags(ctx context.Context, rawArgs []string) (map[string]string, []string, error) {
	overrides := make(map[string]string)
	filtered := make([]string, 0, len(rawArgs))
	for i := 0; i < len(rawArgs); i++ {
		if rawArgs[i] != "--set" {
			filtered = append(filtered, rawArgs[i])
			continue
		}
		if i+1 >= len(rawArgs) {
			return nil, nil, errors.Errorf(ctx, "--set requires a value")
		}
		val := rawArgs[i+1]
		i++ // consume the value
		parts := strings.SplitN(val, "=", 2)
		if len(parts) != 2 {
			return nil, nil, errors.Errorf(ctx, "--set value must be key=value, got %q", val)
		}
		key := parts[0]
		value := parts[1]
		if key == "" {
			return nil, nil, errors.Errorf(ctx, "--set key must not be empty")
		}
		if _, exists := overrides[key]; exists {
			slog.Debug("--set: duplicate key, last value wins")
		}
		overrides[key] = value
	}
	return overrides, filtered, nil
}

func printConfig(ctx context.Context, cfg config.Config) error {
	globalCfg, err := globalconfig.NewLoader().Load(ctx)
	if err != nil {
		return err
	}

	type output struct {
		Global  globalconfig.GlobalConfig `yaml:"global"`
		Project config.Config             `yaml:"project"`
	}
	out := output{
		Global:  globalCfg,
		Project: cfg,
	}

	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	defer enc.Close()
	return enc.Encode(out)
}

func printHelp(w io.Writer) {
	fmt.Fprintf(
		w,
		"Usage: dark-factory [options] <command [subcommand]>\n\nCommands:\n"+
			"  run [--max-containers N] [--skip-preflight] [--model NAME] [--set key=value ...]    Process all queued prompts and exit\n"+
			"  daemon [--max-containers N] [--skip-preflight] [--model NAME] [--set key=value ...] Watch for queued prompts and execute them (long-running)\n"+
			"  kill                   Stop the running daemon\n"+
			"  doctor [--fix] [--yes] [--verifying-stale-hours=N]  Detect state anomalies (and optionally fix them)\n"+
			"  healthcheck [--no-claude]        Probe the full pipeline-execution stack\n"+
			"  status                 Show combined status of prompts and specs\n"+
			"  list                   List all prompts and specs with their status\n"+
			"  config                 Show effective configuration (defaults + .dark-factory.yaml)\n\n"+
			"  prompt list            List prompts with their status\n"+
			"  prompt status          Show prompt status\n"+
			"  prompt approve <id>    Approve a prompt (move from inbox to queue)\n"+
			"  prompt requeue <id>    Reset a prompt's status to queued\n"+
			"  prompt cancel <id>     Cancel an approved or executing prompt\n"+
			"  prompt retry           Shorthand for prompt requeue --failed\n"+
			"  prompt complete <id>   Complete a prompt (triggers commit/push)\n"+
			"  prompt unapprove <id>  Unapprove a prompt (move back to inbox, reset to draft)\n"+
			"  prompt reject <id> --reason <text>  Reject a prompt (move to rejected/, terminal state)\n"+
			"  prompt show <id>       Show details for a single prompt\n\n"+
			"  spec list              List specs\n"+
			"  spec status            Show spec status\n"+
			"  spec approve <id>      Approve a spec\n"+
			"  spec unapprove <id>    Unapprove a spec (move back to inbox, reset to draft)\n"+
			"  spec complete <id>        Mark a verified spec as completed\n"+
			"  spec mark-prompted <id>  Mark a spec as prompted (transitions approved/generating to prompted)\n"+
			"  spec reject <id> --reason <text>    Reject a spec and all linked prompts (move to rejected/, terminal state)\n"+
			"  spec show <id>         Show details for a single spec\n\n"+
			"  scenario list          List scenarios\n"+
			"  scenario show <id>     Show full contents of a scenario\n"+
			"  scenario status        Show scenario status counts\n\n"+
			"Configuration:\n"+
			"  Global config:  ~/.config/dark-factory/config.yaml (XDG)\n"+
			"                  ~/.dark-factory/config.yaml (legacy)\n"+
			"  Per-project:    .dark-factory.yaml (current directory)\n\n"+
			"Options:\n  -debug  Enable debug logging\n\n"+
			"Flags:\n  --help, -h       Show this help\n  --version, -v    Show version\n",
	)
}

func printRunHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory run [--max-containers N] [--auto-approve] [--skip-preflight] [--model NAME] [--set key=value ...]\n\n"+
			"Process all queued prompts and exit.\n\n"+
			"Flags:\n"+
			"  --max-containers N      Override the container limit for this run\n"+
			"  --auto-approve          Automatically approve new prompts found during run\n"+
			"  --skip-preflight        Skip preflight baseline check for this invocation.\n"+
			"                          Prompts may run on a broken baseline — use with caution.\n"+
			"  --model NAME            Override model for this invocation (overrides yaml)\n"+
			"  --set key=value         Override a config field for this invocation; may repeat\n"+
			"                          Supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers, workflow, pr, autoMerge, autoGeneratePrompts\n"+
			"                          Bool example:   --set hideGit=true  --set pr=true  --set autoMerge=false\n"+
			"                          Int example:    --set dirtyFileThreshold=5\n"+
			"                          String example: --set model=claude-opus-4-7  --set workflow=branch\n"+
			"                          Workflow example: --set workflow=branch --set pr=true\n"+
			"                          Note: 'workflow: pr' is yaml-only legacy; use --set workflow=clone --set pr=true\n"+
			"                          Note: --max-containers N takes precedence over --set maxContainers=N if both are passed.\n"+
			"  --help, -h              Show this help\n",
	)
}

func printDaemonHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory daemon [--max-containers N] [--skip-preflight] [--skip-healthcheck] [--model NAME] [--set key=value ...]\n\n"+
			"Watch for queued prompts and execute them (long-running).\n\n"+
			"Flags:\n"+
			"  --max-containers N      Override the container limit for this run\n"+
			"  --skip-preflight        Skip preflight baseline check for this invocation.\n"+
			"                          Prompts may run on a broken baseline — use with caution.\n"+
			"  --skip-healthcheck      Skip the healthcheck startup gate for this invocation (daemon only).\n"+
			"  --model NAME            Override model for this invocation (overrides yaml)\n"+
			"  --set key=value         Override a config field for this invocation; may repeat\n"+
			"                          Supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers, workflow, pr, autoMerge, autoGeneratePrompts\n"+
			"                          Bool example:   --set hideGit=true  --set pr=true  --set autoMerge=false\n"+
			"                          Int example:    --set dirtyFileThreshold=5\n"+
			"                          String example: --set model=claude-opus-4-7  --set workflow=branch\n"+
			"                          Workflow example: --set workflow=branch --set pr=true\n"+
			"                          Note: 'workflow: pr' is yaml-only legacy; use --set workflow=clone --set pr=true\n"+
			"                          Note: --max-containers N takes precedence over --set maxContainers=N if both are passed.\n"+
			"  --help, -h              Show this help\n",
	)
}

func printKillHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory kill\n\n"+
			"Stop the running daemon.\n\n"+
			"Flags:\n"+
			"  --help, -h  Show this help\n",
	)
}

func printStatusHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory status\n\n"+
			"Show combined status of prompts and specs.\n\n"+
			"Flags:\n"+
			"  --help, -h  Show this help\n",
	)
}

func printListHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory list\n\n"+
			"List all prompts and specs with their status.\n\n"+
			"Flags:\n"+
			"  --help, -h  Show this help\n",
	)
}

func printConfigHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory config\n\n"+
			"Show effective configuration (defaults + .dark-factory.yaml).\n\n"+
			"Flags:\n"+
			"  --help, -h  Show this help\n",
	)
}

func printPromptHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory prompt <subcommand>\n\nSubcommands:\n"+
			"  list            List prompts with their status\n"+
			"  status          Show prompt status\n"+
			"  approve <id>    Approve a prompt (move from inbox to queue)\n"+
			"  requeue <id>    Reset a prompt's status to queued\n"+
			"  cancel <id>     Cancel an approved or executing prompt\n"+
			"  retry           Shorthand for prompt requeue --failed\n"+
			"  complete <id> [--release]\n"+
			"                  Complete a prompt (commits locally; on master+autoRelease: tag+push;\n"+
			"                  --release forces release on any branch)\n"+
			"  unapprove <id>  Unapprove a prompt (move back to inbox, reset to draft)\n"+
			"  reject <id> --reason <text>  Reject a prompt (move to rejected/, terminal state)\n"+
			"  show <id>       Show details for a single prompt\n"+
			"  <id> formats: padded number (063), unpadded number (63), full basename (063-foo-bar), or basename with .md extension\n",
	)
}

func printSpecHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory spec <subcommand>\n\nSubcommands:\n"+
			"  list            List specs\n"+
			"  status          Show spec status\n"+
			"  approve <id>    Approve a spec\n"+
			"  unapprove <id>  Unapprove a spec (move back to inbox, reset to draft)\n"+
			"  complete <id>   Mark a verified spec as completed\n"+
			"  mark-prompted <id>       Mark a spec as prompted (transitions approved/generating to prompted)\n"+
			"  reject <id> --reason <text>  Reject a spec and all linked prompts (move to rejected/, terminal state)\n"+
			"  show <id>       Show details for a single spec\n"+
			"  <id> formats: padded number (063), unpadded number (63), full basename (063-foo-bar), or basename with .md extension\n",
	)
}

func printScenarioHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory scenario <subcommand>\n\nSubcommands:\n"+
			"  list          List scenarios with their status\n"+
			"  show <id>     Show full contents of a scenario\n"+
			"  status        Show scenario status counts\n",
	)
}

// ParseArgs parses command line arguments (without program name) and returns
// (debug, command, subcommand, args, autoApprove, skipPreflight, model, skipHealthcheck).
// The -debug flag can appear anywhere and is extracted before parsing.
// The --auto-approve flag is extracted for the "run" command.
// The --skip-preflight flag is extracted for the "run" and "daemon" commands.
// The --skip-healthcheck flag is extracted for the "daemon" command.
// The --model NAME flag is extracted for "run" and "daemon".
// model is empty string when --model is not passed.
// --set key=value is NOT extracted here — call parseSetFlags before ParseArgs.
// No args → command="help" (prints usage, exits 0)
// Bare "help" word → command="help" (same as --help)
// Unknown command → command="unknown", args[0]=the unrecognized command
// Two-level: "prompt list" → command="prompt", subcommand="list"
// Top-level: "status", "list", "run", "daemon" → command=<cmd>, subcommand=""
func ParseArgs(rawArgs []string) (bool, string, string, []string, bool, bool, string, bool) {
	debug := false
	autoApprove := false
	skipPreflight := false
	skipHealthcheck := false
	model := ""
	filtered := make([]string, 0, len(rawArgs))
	for _, arg := range rawArgs {
		switch arg {
		case "-debug":
			debug = true
		case "--auto-approve":
			autoApprove = true
		case "--skip-preflight":
			skipPreflight = true
		case "--skip-healthcheck":
			skipHealthcheck = true
		default:
			filtered = append(filtered, arg)
		}
	}

	// Extract --model NAME from the filtered args (already stripped of boolean flags)
	modelFiltered := make([]string, 0, len(filtered))
	for i := 0; i < len(filtered); i++ {
		if filtered[i] == "--model" {
			if i+1 >= len(filtered) {
				// Missing value — keep "--model" in filtered so runCommand can reject it
				modelFiltered = append(modelFiltered, filtered[i])
				continue
			}
			model = filtered[i+1]
			i++ // skip the value
			continue
		}
		modelFiltered = append(modelFiltered, filtered[i])
	}
	filtered = modelFiltered

	if len(filtered) == 0 {
		return debug, "help", "", []string{}, autoApprove, skipPreflight, model, skipHealthcheck
	}

	command := filtered[0]
	rest := filtered[1:]

	switch command {
	case "help", "--help", "-help", "-h":
		return debug, "help", "", []string{}, autoApprove, skipPreflight, model, skipHealthcheck
	case "--version", "-version", "-v":
		return debug, "version", "", []string{}, autoApprove, skipPreflight, model, skipHealthcheck
	case "run", "daemon", "kill", "status", "list", "config", "doctor", "healthcheck":
		return debug, command, "", rest, autoApprove, skipPreflight, model, skipHealthcheck
	case "prompt", "spec", "scenario":
		if len(rest) == 0 {
			return debug, command, "", []string{}, autoApprove, skipPreflight, model, skipHealthcheck
		}
		return debug, command, rest[0], rest[1:], autoApprove, skipPreflight, model, skipHealthcheck
	}

	return debug, "unknown", "", filtered, autoApprove, skipPreflight, model, skipHealthcheck
}
