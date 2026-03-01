// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"

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
	command, args := parseArgs()

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
		return fmt.Errorf("unknown command: %s", command)
	}
}

// parseArgs parses command line arguments and returns (command, args).
// No args or "run" → run command
// "status" → status command
// "queue" → queue command
func parseArgs() (string, []string) {
	if len(os.Args) <= 1 {
		return "run", []string{}
	}

	command := os.Args[1]
	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}

	if command == "run" || command == "status" || command == "queue" {
		return command, args
	}

	// Unknown command - default to run and treat as args
	return "run", os.Args[1:]
}
