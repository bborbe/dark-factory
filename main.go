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

	// Parse command line arguments
	debug, command, args := parseArgs()

	switch command {
	case "help":
		fmt.Fprintf(
			os.Stdout,
			"Usage: dark-factory [options] [command]\n\nCommands:\n  run     Watch for queued prompts and execute them (default)\n  status  Show status of prompts\n  queue   Queue a prompt for execution\n\nOptions:\n  -debug  Enable debug logging\n\nFlags:\n  --help, -h       Show this help\n  --version, -v    Show version\n",
		)
		return nil
	case "version":
		fmt.Fprintf(os.Stdout, "dark-factory %s\n", version.Version)
		return nil
	}

	// Configure slog
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	slog.Info("dark-factory starting", "version", version.Version)

	// Load configuration
	loader := config.NewLoader()
	cfg, err := loader.Load(ctx)
	if err != nil {
		return err
	}

	// Execute command
	switch command {
	case "status":
		statusCmd := factory.CreateStatusCommand(cfg)
		return statusCmd.Run(ctx, args)
	case "queue":
		queueCmd := factory.CreateQueueCommand(cfg)
		return queueCmd.Run(ctx, args)
	case "run":
		r := factory.CreateRunner(cfg, version.Version)
		return r.Run(ctx)
	default:
		return errors.Errorf(ctx, "unknown command: %s", command)
	}
}

// parseArgs parses command line arguments and returns (debug, command, args).
// The -debug flag can appear anywhere and is extracted before parsing the command.
// No args or "run" → run command
// "status" → status command
// "queue" → queue command
func parseArgs() (bool, string, []string) {
	// Filter out -debug flag from args
	debug := false
	filteredArgs := []string{os.Args[0]} // Keep program name
	for _, arg := range os.Args[1:] {
		if arg == "-debug" {
			debug = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	if len(filteredArgs) <= 1 {
		return debug, "run", []string{}
	}

	command := filteredArgs[1]
	args := []string{}
	if len(filteredArgs) > 2 {
		args = filteredArgs[2:]
	}

	switch command {
	case "run", "status", "queue":
		return debug, command, args
	case "--help", "-help", "-h":
		return debug, "help", []string{}
	case "--version", "-version", "-v":
		return debug, "version", []string{}
	}

	// Unknown command - default to run and treat as args
	return debug, "run", filteredArgs[1:]
}
