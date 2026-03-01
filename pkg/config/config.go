// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"

	"github.com/bborbe/errors"
	"github.com/bborbe/validation"
)

// Config holds the dark-factory configuration.
type Config struct {
	Workflow       Workflow `yaml:"workflow"`
	PromptsDir     string   `yaml:"promptsDir"`
	ContainerImage string   `yaml:"containerImage"`
	DebounceMs     int      `yaml:"debounceMs"`
}

// Defaults returns a Config with all default values.
func Defaults() Config {
	return Config{
		Workflow:       WorkflowDirect,
		PromptsDir:     "prompts",
		ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
		DebounceMs:     500,
	}
}

// Validate validates the config fields.
func (c Config) Validate(ctx context.Context) error {
	return validation.All{
		validation.Name("workflow", c.Workflow),
		validation.Name("promptsDir", validation.NotEmptyString(c.PromptsDir)),
		validation.Name("containerImage", validation.NotEmptyString(c.ContainerImage)),
		validation.Name("debounceMs", validation.HasValidationFunc(func(ctx context.Context) error {
			if c.DebounceMs <= 0 {
				return errors.Errorf(ctx, "debounceMs must be positive, got %d", c.DebounceMs)
			}
			return nil
		})),
	}.Validate(ctx)
}
