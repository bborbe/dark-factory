---
status: executing
spec: ["031"]
container: dark-factory-174-spec-031-notifier-package
dark-factory-version: v0.46.1-3-g7588a45-dirty
created: "2026-03-11T22:30:00Z"
queued: "2026-03-11T22:04:59Z"
started: "2026-03-11T22:13:16Z"
---
<summary>
- Two notification channels are supported: Telegram and Discord — both optional, both additive
- Config uses env var name indirection (not secrets): `botTokenEnv`, `chatIDEnv`, `webhookEnv` fields in `.dark-factory.yaml`
- A channel is active only when its env var resolves to a non-empty value at runtime
- Discord webhook URLs must use HTTPS — HTTP URLs are rejected at config validation time
- A new `pkg/notifier/` package provides a single `Notifier` interface for all callers
- Telegram sends a message via HTTP POST to `api.telegram.org/bot<token>/sendMessage`
- Discord sends a message via HTTP POST to the configured webhook URL
- A multi-notifier fans out to all active channels sequentially and logs warnings on delivery failure
- Failed delivery never propagates an error — processing continues unaffected
- When no channels are configured (both env vars empty) the multi-notifier is a no-op with zero overhead
</summary>

<objective>
Create the `pkg/notifier/` package with a `Notifier` interface, Telegram and Discord implementations, and a multi-notifier that fans out to all active channels. Add the notification config structs to `pkg/config/config.go` with HTTPS validation for Discord webhook URLs.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — add `TelegramConfig`, `DiscordConfig`, and `NotificationsConfig` structs and a `Notifications` field to `Config`. Follow the existing `BitbucketConfig.TokenEnv` pattern: the config file stores the env var name, not the secret.
Read `/home/node/.claude/docs/go-patterns.md` for interface/constructor/counterfeiter patterns.
Read `/home/node/.claude/docs/go-testing.md` for Ginkgo/Gomega and coverage requirements.
Read `/home/node/.claude/docs/go-security-linting.md` for gosec rules.
</context>

<requirements>
1. In `pkg/config/config.go`, add the following structs and field:

   ```go
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
   ```

   Add `Notifications NotificationsConfig \`yaml:"notifications"\`` to the `Config` struct.

   Add a helper method to `Config`:
   ```go
   // ResolvedTelegramBotToken reads the Telegram bot token from the env var named in BotTokenEnv.
   func (c Config) ResolvedTelegramBotToken() string

   // ResolvedTelegramChatID reads the Telegram chat ID from the env var named in ChatIDEnv.
   func (c Config) ResolvedTelegramChatID() string

   // ResolvedDiscordWebhook reads the Discord webhook URL from the env var named in WebhookEnv.
   func (c Config) ResolvedDiscordWebhook() string
   ```

   Each helper: if the env var name is empty, return "". Otherwise call `os.Getenv(envVarName)`. Do not log a warning when the env var is empty (unlike `ResolvedBitbucketToken` — notifications are optional). Never log the resolved value.

2. Add Discord webhook HTTPS validation to `Config.Validate()`:
   ```go
   validation.Name("notifications", validation.HasValidationFunc(c.validateNotifications)),
   ```

   ```go
   func (c Config) validateNotifications(ctx context.Context) error {
       webhook := c.ResolvedDiscordWebhook()
       if webhook != "" && !strings.HasPrefix(webhook, "https://") {
           return errors.Errorf(ctx, "discord webhook URL must use HTTPS")
       }
       return nil
   }
   ```

3. Create `pkg/notifier/notifier.go`:

   ```go
   package notifier

   // Event holds the data for a single notification.
   type Event struct {
       ProjectName string
       EventType   string // "prompt_failed", "prompt_partial", "spec_verifying", "review_limit", "stuck_container"
       PromptName  string // filename without path, empty if not applicable
       PRURL       string // empty if not applicable
   }

   //counterfeiter:generate -o ../../mocks/notifier.go --fake-name Notifier . Notifier

   // Notifier sends a notification for a lifecycle event.
   type Notifier interface {
       Notify(ctx context.Context, event Event) error
   }
   ```

   Implement `noopNotifier` (returned when no channels are configured):
   ```go
   type noopNotifier struct{}
   func (n *noopNotifier) Notify(_ context.Context, _ Event) error { return nil }
   ```

4. Create `pkg/notifier/telegram.go`:

   Implement `telegramNotifier` that:
   - Holds `botToken`, `chatID` (resolved values, not env var names)
   - On `Notify`: formats the message as:
     ```
     🔔 [<ProjectName>] <EventType>
     Prompt: <PromptName>   (omit line if empty)
     PR: <PRURL>            (omit line if empty)
     ```
     (Use plain text with minimal markdown — Telegram parse_mode: omitted)
   - HTTP POSTs to `https://api.telegram.org/bot<botToken>/sendMessage` with JSON body `{"chat_id": "<chatID>", "text": "<message>"}`
   - Uses a 5-second timeout on the HTTP request
   - On non-2xx response: returns an error with the status code
   - **NEVER logs the bot token or full webhook URL** — only log the chat_id and a truncated label

5. Create `pkg/notifier/discord.go`:

   Implement `discordNotifier` that:
   - Holds `webhookURL` (resolved value, not env var name)
   - On `Notify`: formats the same message as Telegram (plain text)
   - HTTP POSTs to `webhookURL` with JSON body `{"content": "<message>"}`
   - Uses a 5-second timeout
   - On non-2xx response: returns an error with the status code
   - **NEVER logs the webhook URL** — only log `"discord"` and the event type

6. Create `pkg/notifier/multi.go`:

   Implement `multiNotifier` that:
   - Holds a slice of `Notifier` implementations
   - On `Notify`: calls each notifier sequentially; logs a warning on error but does not return the error
   - Returns nil always

   Constructor:
   ```go
   // NewMultiNotifier returns a Notifier that fans out to all provided notifiers.
   // If notifiers is empty, returns a no-op Notifier.
   func NewMultiNotifier(notifiers ...Notifier) Notifier
   ```

7. Create `pkg/notifier/notifier_test.go` covering:
   - `NewMultiNotifier()` with no args → `Notify` returns nil and does not call any sub-notifier
   - `NewMultiNotifier(a, b)` where `a` fails → `b` still called, error logged, `Notify` returns nil
   - `telegramNotifier.Notify` → makes HTTP POST to the expected URL with correct JSON body (use `httptest.NewServer`)
   - `telegramNotifier.Notify` with non-2xx response → returns error
   - `discordNotifier.Notify` → makes HTTP POST with correct JSON body (use `httptest.NewServer`)
   - `discordNotifier.Notify` with non-2xx response → returns error

8. Add tests to `pkg/config/config_test.go` (not `pkg/notifier/` — these test `Config` methods) covering:
   - `ResolvedDiscordWebhook` returns empty when `WebhookEnv` is empty
   - `Config.validateNotifications` returns error when webhook resolves to an HTTP URL
   - `Config.validateNotifications` returns nil when webhook is empty
   - `Config.validateNotifications` returns nil when webhook is HTTPS

9. Run `make generate` to create the counterfeiter mock.
</requirements>

<constraints>
- Package name: `notifier`
- Use `github.com/bborbe/errors` for error wrapping (not `fmt.Errorf`)
- Bot tokens and webhook URLs must NEVER appear in log output, even at debug level
- Webhook URLs must use HTTPS only — reject HTTP at validation time
- Notification delivery failure logs a warning but does not fail or block the caller
- A channel is active if and only if its resolved env var is non-empty — no explicit enable flag
- No retry/backoff for failed delivery
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
