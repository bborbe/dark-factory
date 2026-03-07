// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/factory"
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
	debug, command, subcommand, args := ParseArgs(os.Args[1:])

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

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	slog.Info("dark-factory starting", "version", version.Version)

	loader := config.NewLoader()
	cfg, err := loader.Load(ctx)
	if err != nil {
		return err
	}

	switch command {
	case "prompt":
		return runPromptCommand(ctx, cfg, subcommand, args)
	case "spec":
		return runSpecCommand(ctx, cfg, subcommand, args)
	case "status":
		return factory.CreateCombinedStatusCommand(cfg).Run(ctx, args)
	case "list":
		return factory.CreateCombinedListCommand(cfg).Run(ctx, args)
	case "run":
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
	case "status":
		return factory.CreateStatusCommand(cfg).Run(ctx, args)
	case "list":
		return factory.CreateListCommand(cfg).Run(ctx, args)
	case "approve":
		return factory.CreateApproveCommand(cfg).Run(ctx, args)
	case "requeue":
		return factory.CreateRequeueCommand(cfg).Run(ctx, args)
	case "retry":
		return factory.CreateRequeueCommand(cfg).Run(ctx, []string{"--failed"})
	case "show":
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
	case "list":
		return factory.CreateSpecListCommand(cfg).Run(ctx, args)
	case "status":
		return factory.CreateSpecStatusCommand(cfg).Run(ctx, args)
	case "approve":
		return factory.CreateSpecApproveCommand(cfg).Run(ctx, args)
	case "complete":
		return factory.CreateSpecCompleteCommand(cfg).Run(ctx, args)
	case "show":
		return factory.CreateSpecShowCommand(cfg).Run(ctx, args)
	default:
		return errors.Errorf(ctx, "unknown spec subcommand: %s", subcommand)
	}
}

func printHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory [options] [command [subcommand]]\n\nCommands:\n"+
			"  run                    Watch for queued prompts and execute them (default)\n"+
			"  status                 Show combined status of prompts and specs\n"+
			"  list                   List all prompts and specs with their status\n\n"+
			"  prompt list            List prompts with their status\n"+
			"  prompt status          Show prompt status\n"+
			"  prompt approve <id>    Approve a prompt (move from inbox to queue)\n"+
			"  prompt requeue <id>    Reset a prompt's status to queued\n"+
			"  prompt retry           Shorthand for prompt requeue --failed\n"+
			"  prompt show <id>       Show details for a single prompt\n\n"+
			"  spec list              List specs\n"+
			"  spec status            Show spec status\n"+
			"  spec approve <id>      Approve a spec\n"+
			"  spec complete <id>     Mark a verified spec as completed\n"+
			"  spec show <id>         Show details for a single spec\n\n"+
			"Options:\n  -debug  Enable debug logging\n\n"+
			"Flags:\n  --help, -h       Show this help\n  --version, -v    Show version\n",
	)
}

// ParseArgs parses command line arguments (without program name) and returns
// (debug, command, subcommand, args).
// The -debug flag can appear anywhere and is extracted before parsing.
// No args → command="run"
// Unknown command → command="unknown", args[0]=the unrecognized command
// Two-level: "prompt list" → command="prompt", subcommand="list"
// Top-level: "status", "list", "run" → command=<cmd>, subcommand=""
func ParseArgs(rawArgs []string) (bool, string, string, []string) {
	debug := false
	filtered := make([]string, 0, len(rawArgs))
	for _, arg := range rawArgs {
		if arg == "-debug" {
			debug = true
		} else {
			filtered = append(filtered, arg)
		}
	}

	if len(filtered) == 0 {
		return debug, "run", "", []string{}
	}

	command := filtered[0]
	rest := filtered[1:]

	switch command {
	case "--help", "-help", "-h":
		return debug, "help", "", []string{}
	case "--version", "-version", "-v":
		return debug, "version", "", []string{}
	case "run", "status", "list":
		return debug, command, "", rest
	case "prompt", "spec":
		if len(rest) == 0 {
			return debug, command, "", []string{}
		}
		return debug, command, rest[0], rest[1:]
	}

	return debug, "unknown", "", filtered
}
