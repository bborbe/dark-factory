// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/factory"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/globalconfig"
	"github.com/bborbe/dark-factory/pkg/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	debug, command, subcommand, args, autoApprove := ParseArgs(os.Args[1:])

	switch command {
	case "help":
		printHelp()
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

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	slog.Info("dark-factory starting", "version", version.Version)

	gitRoot, err := git.ResolveGitRoot(ctx)
	if err != nil {
		return err
	}
	slog.Debug("resolved git root", "root", gitRoot)
	if err := os.Chdir(gitRoot); err != nil {
		return errors.Wrap(ctx, err, "chdir to git root")
	}

	loader := config.NewLoader()
	cfg, err := loader.Load(ctx)
	if err != nil {
		return err
	}

	return runCommand(ctx, cfg, command, subcommand, args, autoApprove)
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
	}
}

func runCommand(
	ctx context.Context,
	cfg config.Config,
	command, subcommand string,
	args []string,
	autoApprove bool,
) error {
	switch command {
	case "prompt":
		return runPromptCommand(ctx, cfg, subcommand, args)
	case "spec":
		return runSpecCommand(ctx, cfg, subcommand, args)
	case "status":
		if err := validateNoArgs(ctx, args, printStatusHelp); err != nil {
			return err
		}
		return factory.CreateCombinedStatusCommand(cfg).Run(ctx, args)
	case "list":
		if err := validateNoArgs(ctx, args, printListHelp); err != nil {
			return err
		}
		return factory.CreateCombinedListCommand(cfg).Run(ctx, args)
	case "config":
		if err := validateNoArgs(ctx, args, printConfigHelp); err != nil {
			return err
		}
		return printConfig(cfg)
	case "kill":
		if err := validateNoArgs(ctx, args, printKillHelp); err != nil {
			return err
		}
		return factory.CreateKillCommand(cfg).Run(ctx, args)
	case "run":
		if err := validateNoArgs(ctx, args, printRunHelp); err != nil {
			return err
		}
		return factory.CreateOneShotRunner(cfg, version.Version, autoApprove).Run(ctx)
	case "daemon":
		if err := validateNoArgs(ctx, args, printDaemonHelp); err != nil {
			return err
		}
		return factory.CreateRunner(cfg, version.Version).Run(ctx)
	default:
		return errors.Errorf(ctx, "unknown command: %s", command)
	}
}

func runPromptCommand(
	ctx context.Context,
	cfg config.Config,
	subcommand string,
	args []string,
) error {
	switch subcommand {
	case "", "--help", "-h", "help":
		printPromptHelp()
		return nil
	case "status":
		if err := validateNoArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateStatusCommand(cfg).Run(ctx, args)
	case "list":
		if err := validateNoArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateListCommand(cfg).Run(ctx, args)
	case "approve":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateApproveCommand(cfg).Run(ctx, args)
	case "requeue":
		if err := validateRequeueArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateRequeueCommand(cfg).Run(ctx, args)
	case "cancel":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateCancelCommand(cfg).Run(ctx, args)
	case "retry":
		if err := validateNoArgs(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateRequeueCommand(cfg).Run(ctx, []string{"--failed"})
	case "complete":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreatePromptCompleteCommand(cfg).Run(ctx, args)
	case "unapprove":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreateUnapproveCommand(cfg).Run(ctx, args)
	case "show":
		if err := validateOneArg(ctx, args, printPromptHelp); err != nil {
			return err
		}
		return factory.CreatePromptShowCommand(cfg).Run(ctx, args)
	default:
		return errors.Errorf(ctx, "unknown prompt subcommand: %s", subcommand)
	}
}

func runSpecCommand(
	ctx context.Context,
	cfg config.Config,
	subcommand string,
	args []string,
) error {
	switch subcommand {
	case "", "--help", "-h", "help":
		printSpecHelp()
		return nil
	case "list":
		if err := validateNoArgs(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecListCommand(cfg).Run(ctx, args)
	case "status":
		if err := validateNoArgs(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecStatusCommand(cfg).Run(ctx, args)
	case "approve":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecApproveCommand(cfg).Run(ctx, args)
	case "unapprove":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecUnapproveCommand(cfg).Run(ctx, args)
	case "complete":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecCompleteCommand(cfg).Run(ctx, args)
	case "show":
		if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
			return err
		}
		return factory.CreateSpecShowCommand(cfg).Run(ctx, args)
	default:
		return errors.Errorf(ctx, "unknown spec subcommand: %s", subcommand)
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

func printConfig(cfg config.Config) error {
	ctx := context.Background()
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

func printHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory [options] <command [subcommand]>\n\nCommands:\n"+
			"  run                    Process all queued prompts and exit\n"+
			"  daemon                 Watch for queued prompts and execute them (long-running)\n"+
			"  kill                   Stop the running daemon\n"+
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
			"  prompt show <id>       Show details for a single prompt\n\n"+
			"  spec list              List specs\n"+
			"  spec status            Show spec status\n"+
			"  spec approve <id>      Approve a spec\n"+
			"  spec unapprove <id>    Unapprove a spec (move back to inbox, reset to draft)\n"+
			"  spec complete <id>     Mark a verified spec as completed\n"+
			"  spec show <id>         Show details for a single spec\n\n"+
			"Options:\n  -debug  Enable debug logging\n\n"+
			"Flags:\n  --help, -h       Show this help\n  --version, -v    Show version\n",
	)
}

func printRunHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory run [--auto-approve]\n\n"+
			"Process all queued prompts and exit.\n\n"+
			"Flags:\n"+
			"  --auto-approve  Automatically approve new prompts found during run\n"+
			"  --help, -h      Show this help\n",
	)
}

func printDaemonHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory daemon\n\n"+
			"Watch for queued prompts and execute them (long-running).\n\n"+
			"Flags:\n"+
			"  --help, -h  Show this help\n",
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
			"  complete <id>   Complete a prompt (triggers commit/push)\n"+
			"  unapprove <id>  Unapprove a prompt (move back to inbox, reset to draft)\n"+
			"  show <id>       Show details for a single prompt\n",
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
			"  show <id>       Show details for a single spec\n",
	)
}

// ParseArgs parses command line arguments (without program name) and returns
// (debug, command, subcommand, args, autoApprove).
// The -debug flag can appear anywhere and is extracted before parsing.
// The --auto-approve flag is extracted for the "run" command.
// No args → command="unknown" (an explicit subcommand is required)
// Unknown command → command="unknown", args[0]=the unrecognized command
// Two-level: "prompt list" → command="prompt", subcommand="list"
// Top-level: "status", "list", "run", "daemon" → command=<cmd>, subcommand=""
func ParseArgs(rawArgs []string) (bool, string, string, []string, bool) {
	debug := false
	autoApprove := false
	filtered := make([]string, 0, len(rawArgs))
	for _, arg := range rawArgs {
		switch arg {
		case "-debug":
			debug = true
		case "--auto-approve":
			autoApprove = true
		default:
			filtered = append(filtered, arg)
		}
	}

	if len(filtered) == 0 {
		return debug, "unknown", "", []string{}, autoApprove
	}

	command := filtered[0]
	rest := filtered[1:]

	switch command {
	case "--help", "-help", "-h":
		return debug, "help", "", []string{}, autoApprove
	case "--version", "-version", "-v":
		return debug, "version", "", []string{}, autoApprove
	case "run", "daemon", "kill", "status", "list", "config":
		return debug, command, "", rest, autoApprove
	case "prompt", "spec":
		if len(rest) == 0 {
			return debug, command, "", []string{}, autoApprove
		}
		return debug, command, rest[0], rest[1:], autoApprove
	}

	return debug, "unknown", "", filtered, autoApprove
}
