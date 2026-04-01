// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bborbe/errors"
	"github.com/bborbe/validation"

	"github.com/bborbe/dark-factory/pkg"
)

// GitHubConfig holds GitHub-specific configuration.
type GitHubConfig struct {
	Token string `yaml:"token"`
}

// TelegramConfig holds Telegram notification configuration.
type TelegramConfig struct {
	BotTokenEnv string `yaml:"botTokenEnv"`
	ChatIDEnv   string `yaml:"chatIDEnv"`
}

// DiscordConfig holds Discord notification configuration.
type DiscordConfig struct {
	WebhookEnv string `yaml:"webhookEnv"`
}

// NotificationsConfig holds notification channel configuration.
type NotificationsConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Discord  DiscordConfig  `yaml:"discord"`
}

// BitbucketConfig holds Bitbucket Server-specific configuration.
type BitbucketConfig struct {
	BaseURL  string `yaml:"baseURL"`
	TokenEnv string `yaml:"tokenEnv"`
}

// PromptsConfig holds directories for the prompt lifecycle.
type PromptsConfig struct {
	InboxDir      string `yaml:"inboxDir"`
	InProgressDir string `yaml:"inProgressDir"`
	CompletedDir  string `yaml:"completedDir"`
	LogDir        string `yaml:"logDir"`
}

// SpecsConfig holds directories for the spec lifecycle.
type SpecsConfig struct {
	InboxDir      string `yaml:"inboxDir"`
	InProgressDir string `yaml:"inProgressDir"`
	CompletedDir  string `yaml:"completedDir"`
	LogDir        string `yaml:"logDir"`
}

// Config holds the dark-factory configuration.
type Config struct {
	ProjectName       string              `yaml:"projectName"`
	Workflow          Workflow            `yaml:"workflow"`
	PR                bool                `yaml:"pr,omitempty"`
	Worktree          bool                `yaml:"worktree,omitempty"`
	DefaultBranch     string              `yaml:"defaultBranch"`
	Prompts           PromptsConfig       `yaml:"prompts"`
	Specs             SpecsConfig         `yaml:"specs"`
	ContainerImage    string              `yaml:"containerImage"`
	NetrcFile         string              `yaml:"netrcFile"`
	GitconfigFile     string              `yaml:"gitconfigFile"`
	Model             string              `yaml:"model"`
	ValidationCommand string              `yaml:"validationCommand"`
	ValidationPrompt  string              `yaml:"validationPrompt"`
	DebounceMs        int                 `yaml:"debounceMs"`
	ServerPort        int                 `yaml:"serverPort"`
	AutoMerge         bool                `yaml:"autoMerge"`
	AutoRelease       bool                `yaml:"autoRelease"`
	VerificationGate  bool                `yaml:"verificationGate"`
	AutoReview        bool                `yaml:"autoReview"`
	MaxReviewRetries  int                 `yaml:"maxReviewRetries"`
	AllowedReviewers  []string            `yaml:"allowedReviewers,omitempty"`
	UseCollaborators  bool                `yaml:"useCollaborators"`
	PollIntervalSec   int                 `yaml:"pollIntervalSec"`
	GitHub            GitHubConfig        `yaml:"github"`
	Provider          Provider            `yaml:"provider"`
	Bitbucket         BitbucketConfig     `yaml:"bitbucket"`
	Notifications     NotificationsConfig `yaml:"notifications"`
	Env               map[string]string   `yaml:"env,omitempty"`
	ClaudeDir         string              `yaml:"claudeDir"`
	GenerateCommand   string              `yaml:"generateCommand"`
}

// Defaults returns a Config with all default values.
func Defaults() Config {
	return Config{
		Workflow: WorkflowDirect,
		PR:       false,
		Worktree: false,
		Prompts: PromptsConfig{
			InboxDir:      "prompts",
			InProgressDir: "prompts/in-progress",
			CompletedDir:  "prompts/completed",
			LogDir:        "prompts/log",
		},
		Specs: SpecsConfig{
			InboxDir:      "specs",
			InProgressDir: "specs/in-progress",
			CompletedDir:  "specs/completed",
			LogDir:        "specs/log",
		},
		ContainerImage:    pkg.DefaultContainerImage,
		Model:             "claude-sonnet-4-6",
		ValidationCommand: "make precommit",
		DebounceMs:        500,
		ServerPort:        0,
		AutoMerge:         false,
		AutoRelease:       false,
		AutoReview:        false,
		MaxReviewRetries:  3,
		PollIntervalSec:   60,
		UseCollaborators:  false,
		GitHub:            GitHubConfig{},
		Provider:          ProviderGitHub,
		Bitbucket:         BitbucketConfig{TokenEnv: "BITBUCKET_TOKEN"},
		ClaudeDir:         "~/.claude-yolo",
		GenerateCommand:   "/dark-factory:generate-prompts-for-spec",
	}
}

// Validate validates the config fields.
func (c Config) Validate(ctx context.Context) error {
	return validation.All{
		validation.Name("workflow", c.Workflow),
		validation.Name("inboxDir", validation.NotEmptyString(c.Prompts.InboxDir)),
		validation.Name("inProgressDir", validation.NotEmptyString(c.Prompts.InProgressDir)),
		validation.Name("completedDir", validation.NotEmptyString(c.Prompts.CompletedDir)),
		validation.Name("logDir", validation.NotEmptyString(c.Prompts.LogDir)),
		validation.Name("containerImage", validation.NotEmptyString(c.ContainerImage)),
		validation.Name("model", validation.NotEmptyString(c.Model)),
		validation.Name("debounceMs", validation.HasValidationFunc(func(ctx context.Context) error {
			if c.DebounceMs <= 0 {
				return errors.Errorf(ctx, "debounceMs must be positive, got %d", c.DebounceMs)
			}
			return nil
		})),
		validation.Name("serverPort", validation.HasValidationFunc(func(ctx context.Context) error {
			if c.ServerPort < 0 || c.ServerPort > 65535 {
				return errors.Errorf(
					ctx,
					"serverPort must be 0 (disabled) or 1-65535, got %d",
					c.ServerPort,
				)
			}
			return nil
		})),
		validation.Name(
			"completedDir",
			validation.HasValidationFunc(func(ctx context.Context) error {
				if c.Prompts.CompletedDir == c.Prompts.InProgressDir {
					return errors.Errorf(ctx, "completedDir cannot equal inProgressDir")
				}
				if c.Prompts.CompletedDir == c.Prompts.InboxDir {
					return errors.Errorf(ctx, "completedDir cannot equal inboxDir")
				}
				return nil
			}),
		),
		validation.Name("autoMerge", validation.HasValidationFunc(func(ctx context.Context) error {
			if c.AutoMerge && !c.PR {
				return errors.Errorf(ctx, "autoMerge requires pr: true")
			}
			return nil
		})),
		validation.Name("autoReview", validation.HasValidationFunc(c.validateAutoReview)),
		validation.Name("provider", validation.HasValidationFunc(func(ctx context.Context) error {
			provider := c.Provider
			if provider == "" {
				provider = ProviderGitHub
			}
			return provider.Validate(ctx)
		})),
		validation.Name("bitbucket", validation.HasValidationFunc(c.validateBitbucketConfig)),
		validation.Name("netrcFile", validation.HasValidationFunc(c.validateNetrcFile)),
		validation.Name("gitconfigFile", validation.HasValidationFunc(c.validateGitconfigFile)),
		validation.Name("env", validation.HasValidationFunc(c.validateEnv)),
		validation.Name("notifications", validation.HasValidationFunc(c.validateNotifications)),
		validation.Name("github.token", validation.HasValidationFunc(c.validateGitHubToken)),
		validation.Name(
			"validationPrompt",
			validation.HasValidationFunc(c.validateValidationPrompt),
		),
	}.Validate(ctx)
}

// validateAutoReview validates the autoReview configuration.
func (c Config) validateAutoReview(ctx context.Context) error {
	if !c.AutoReview {
		return nil
	}
	if !c.PR {
		return errors.Errorf(ctx, "autoReview requires pr: true")
	}
	if !c.AutoMerge {
		return errors.Errorf(ctx, "autoReview requires autoMerge")
	}
	if len(c.AllowedReviewers) == 0 && !c.UseCollaborators {
		return errors.Errorf(ctx, "autoReview requires allowedReviewers or useCollaborators: true")
	}
	return nil
}

// validateBitbucketConfig validates the bitbucket configuration when provider is bitbucket-server.
func (c Config) validateBitbucketConfig(ctx context.Context) error {
	provider := c.Provider
	if provider == "" {
		provider = ProviderGitHub
	}
	if provider != ProviderBitbucketServer {
		return nil
	}
	if c.Bitbucket.BaseURL == "" {
		return errors.Errorf(ctx, "bitbucket.baseURL is required when provider is bitbucket-server")
	}
	return nil
}

// validateNetrcFile validates the netrcFile configuration.
func (c Config) validateNetrcFile(ctx context.Context) error {
	if c.NetrcFile == "" {
		return nil
	}
	resolved := resolveFilePath(c.NetrcFile)
	if _, err := os.Stat(resolved); err != nil {
		return errors.Errorf(ctx, "netrcFile %q does not exist: %v", resolved, err)
	}
	return nil
}

// validateGitconfigFile validates the gitconfigFile configuration.
func (c Config) validateGitconfigFile(ctx context.Context) error {
	if c.GitconfigFile == "" {
		return nil
	}
	resolved := resolveFilePath(c.GitconfigFile)
	if _, err := os.Stat(resolved); err != nil {
		return errors.Errorf(ctx, "gitconfigFile %q does not exist: %v", resolved, err)
	}
	return nil
}

// reservedEnvKeys are env var names set internally by the executor and cannot be overridden.
var reservedEnvKeys = []string{"YOLO_PROMPT_FILE", "ANTHROPIC_MODEL"}

// validateEnv validates the env map keys and values.
func (c Config) validateEnv(ctx context.Context) error {
	for k, v := range c.Env {
		if k == "" {
			return errors.Errorf(ctx, "env key must not be empty")
		}
		for _, reserved := range reservedEnvKeys {
			if k == reserved {
				return errors.Errorf(ctx, "env key %q is reserved and cannot be overridden", k)
			}
		}
		if strings.ContainsAny(v, "\x00\n\r") {
			return errors.Errorf(ctx, "env value for %q contains invalid characters", k)
		}
	}
	return nil
}

var envVarPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)\}$`)

var githubTokenEnvVarPattern = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

// resolveEnvVar resolves environment variable references in the form ${VAR_NAME}.
// If the value matches the pattern, it returns the environment variable value.
// Otherwise, it returns the value as-is.
func resolveEnvVar(value string) string {
	matches := envVarPattern.FindStringSubmatch(value)
	if len(matches) == 2 {
		return os.Getenv(matches[1])
	}
	return value
}

// validateGitHubToken validates that the GitHub token is either empty or an env var reference.
func (c Config) validateGitHubToken(ctx context.Context) error {
	if c.GitHub.Token == "" {
		return nil
	}
	if githubTokenEnvVarPattern.MatchString(c.GitHub.Token) {
		return nil
	}
	return errors.Errorf(
		ctx,
		"github.token must be an env var reference like ${GITHUB_TOKEN}, not a literal value",
	)
}

// resolveFilePath resolves a file path by expanding ${VAR} env vars and leading ~/ to home dir.
func resolveFilePath(value string) string {
	value = resolveEnvVar(value)
	if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			value = home + value[1:]
		}
	}
	return value
}

// ResolvedClaudeDir returns the claude-yolo config directory with ~ expanded.
func (c Config) ResolvedClaudeDir() string {
	return resolveFilePath(c.ClaudeDir)
}

// ResolvedGitHubToken returns the GitHub token with environment variables resolved.
// Returns empty string when not configured, letting gh use its own auth.
func (c Config) ResolvedGitHubToken() string {
	if c.GitHub.Token == "" {
		return ""
	}
	token := resolveEnvVar(c.GitHub.Token)
	if token == "" {
		slog.Warn("github.token configured but env var is empty, using default gh auth")
	}
	return token
}

// ResolvedBitbucketToken reads the Bitbucket token from the env var named in TokenEnv.
// Returns empty string when not configured or env var is empty.
// Uses os.Getenv directly (not resolveEnvVar) because tokenEnv holds the env var name
// (e.g. "BITBUCKET_TOKEN"), not a ${VAR} reference that resolveEnvVar expects.
func (c Config) ResolvedBitbucketToken() string {
	if c.Bitbucket.TokenEnv == "" {
		return ""
	}
	token := os.Getenv(c.Bitbucket.TokenEnv)
	if token == "" {
		slog.Warn("bitbucket.tokenEnv configured but env var is empty", "env", c.Bitbucket.TokenEnv)
	}
	return token
}

// ResolvedTelegramBotToken reads the Telegram bot token from the env var named in BotTokenEnv.
// Returns empty string when not configured or env var is empty.
func (c Config) ResolvedTelegramBotToken() string {
	if c.Notifications.Telegram.BotTokenEnv == "" {
		return ""
	}
	return os.Getenv(c.Notifications.Telegram.BotTokenEnv)
}

// ResolvedTelegramChatID reads the Telegram chat ID from the env var named in ChatIDEnv.
// Returns empty string when not configured or env var is empty.
func (c Config) ResolvedTelegramChatID() string {
	if c.Notifications.Telegram.ChatIDEnv == "" {
		return ""
	}
	return os.Getenv(c.Notifications.Telegram.ChatIDEnv)
}

// ResolvedDiscordWebhook reads the Discord webhook URL from the env var named in WebhookEnv.
// Returns empty string when not configured or env var is empty.
func (c Config) ResolvedDiscordWebhook() string {
	if c.Notifications.Discord.WebhookEnv == "" {
		return ""
	}
	return os.Getenv(c.Notifications.Discord.WebhookEnv)
}

// validateValidationPrompt rejects absolute paths and paths that traverse outside the project root.
func (c Config) validateValidationPrompt(ctx context.Context) error {
	v := c.ValidationPrompt
	if v == "" {
		return nil
	}
	if filepath.IsAbs(v) {
		return errors.Errorf(
			ctx,
			"validationPrompt must be a relative path or inline text, got absolute path: %q",
			v,
		)
	}
	// Reject paths with .. that would escape the project root
	cleaned := filepath.Clean(v)
	if strings.HasPrefix(cleaned, "..") {
		return errors.Errorf(
			ctx,
			"validationPrompt path must not traverse outside the project root: %q",
			v,
		)
	}
	return nil
}

// validateNotifications validates the notifications configuration.
func (c Config) validateNotifications(ctx context.Context) error {
	webhook := c.ResolvedDiscordWebhook()
	if webhook != "" && !strings.HasPrefix(webhook, "https://") {
		return errors.Errorf(ctx, "discord webhook URL must use HTTPS")
	}
	return nil
}
