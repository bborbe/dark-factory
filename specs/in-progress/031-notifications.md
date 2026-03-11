---
status: verifying
approved: "2026-03-11T21:55:09Z"
prompted: "2026-03-11T21:57:50Z"
verifying: "2026-03-11T22:47:35Z"
branch: dark-factory/notifications
---

## Summary

- Dark-factory sends notifications when human attention is needed
- Telegram and Discord are supported — active if env var resolves to non-empty
- Both channels can fire simultaneously — no selector needed
- Config follows the `tokenEnv` pattern (env var name, not secret) so `.dark-factory.yaml` stays public
- No notification is sent for success — only events that require human action
- Processing continues regardless of notification delivery

## Problem

Multiple dark-factory instances run across different projects in different terminals. When a factory needs human attention (failed prompt, spec ready for verification, review loop exhausted) there is no signal — the human must poll each project manually. With 5+ active factories, a missed failure can sit unnoticed for hours.

## Goal

After this work, every dark-factory instance notifies configured channels (Telegram, Discord, or both) whenever human attention is required, without any polling. Messages include the project name, event type, and relevant context (prompt name, PR URL). Processing continues regardless of notification delivery.

## Non-Goals

- Success notifications (prompt completed) — too noisy
- Retry/backoff for failed notification delivery — log warning and continue
- Message formatting beyond plain text with markdown
- Interactive bot commands (reply to retry, etc.)
- Notification deduplication or rate limiting

## Desired Behavior

1. When a prompt fails, a message appears in all configured channels within 5 seconds
2. When a prompt completes with `status: partial` (make precommit failed), a notification fires
3. When a spec transitions to `verifying`, a notification reminds the human to run verification
4. When the review-fix loop exhausts its retry limit, a notification fires
5. When a stuck container is detected, a notification fires
6. Each message includes: project name, event type, prompt/spec name, and PR URL (if available)
7. If a channel is unconfigured or fails to deliver, processing continues unaffected — a warning is logged for failures, no error for unconfigured

## Assumptions

- Outbound HTTPS to `api.telegram.org` and Discord webhook URLs is permitted from the runtime environment
- All attention-required events have observable lifecycle signals available for triggering notifications
- Env vars are available at startup and do not change during runtime

## Constraints

- Config shape is a frozen contract — Telegram uses `botTokenEnv` + `chatIDEnv`, Discord uses `webhookEnv` (env var names, not secrets):
  ```yaml
  telegram:
    botTokenEnv: TELEGRAM_BOT_TOKEN
    chatIDEnv: TELEGRAM_CHAT_ID
  discord:
    webhookEnv: DISCORD_WEBHOOK_URL
  ```
- A channel is active if its env var resolves to non-empty — no explicit enable/disable flag
- No secrets in `.dark-factory.yaml` — only env var names (follows `tokenEnv` pattern from Bitbucket config)
- Notification delivery must not block the main processing loop — short timeout (5s)
- Failed notification delivery logs a warning but does not fail the prompt/spec lifecycle
- Telegram API: pure outbound HTTP POST, no webhook setup needed
- Discord webhook: single HTTP POST, no bot token or OAuth needed

## Security

- Bot tokens and webhook URLs must never appear in log output, even at debug level
- Webhook URLs must use HTTPS scheme only — reject HTTP to prevent credential leakage
- Notification messages must not echo raw prompt content that may contain secrets
- Env var names (not values) are the only credential-related data in config files

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Telegram API unreachable | Log warning, continue processing | Next event retries naturally |
| Discord webhook URL invalid | Log warning, continue processing | Fix env var, restart |
| Both env vars empty | No notifier active — zero overhead | Configure env vars when ready |
| Malformed bot token | Telegram returns 401 — log warning | Fix env var |
| Webhook URL uses HTTP (not HTTPS) | Reject at config validation | Fix URL to use HTTPS |

## Do-Nothing Option

The human continues to poll terminals manually. With 5+ factories, each checked every 30 minutes, that's 10+ terminal switches per hour. A single missed failure can idle a factory for hours until noticed.

## Acceptance Criteria

- [ ] Telegram notifications arrive when a prompt fails
- [ ] Discord notifications arrive when a prompt fails
- [ ] Both channels fire independently when both configured
- [ ] No error or overhead when no channels are configured
- [ ] Config uses `botTokenEnv`/`chatIDEnv`/`webhookEnv` pattern (env var names, not secrets)
- [ ] Notifications fire on: prompt failure, partial completion, spec verifying, review limit, stuck container
- [ ] Failed notification delivery logs warning but does not block processing
- [ ] Webhook URLs must be HTTPS
- [ ] Bot tokens and webhook URLs never appear in logs
- [ ] `make precommit` passes
- [ ] No behavioral changes to existing processing — notifications are additive

## Verification

Run `make precommit` — must pass.
