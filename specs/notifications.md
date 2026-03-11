---
status: idea
---

# Human-Attention Notifications

## Problem

Multiple dark-factory instances run across different projects in different terminals. When a factory needs human attention (failed prompt, spec ready for verification, review loop exhausted) there is no signal — the human must poll each project manually.

## Idea

Dark-factory detects human-attention events and pushes a notification to a configured channel. The human gets reached wherever they are without watching any terminal.

## Events

- Prompt transitions to `failed` — needs manual fix
- Spec transitions to `verifying` — needs verification commands
- Review-fix loop hits retry limit — needs eyes on the PR
- Spec auto-generation failed — still `approved` after timeout

## Notification channels

- Telegram bot (first — best mobile experience, simple API)
- Discord webhook (second — single HTTP POST, no bot needed)

## Config sketch

```yaml
notify:
  channel: telegram
  botToken: ${TELEGRAM_BOT_TOKEN}
  chatID: ${TELEGRAM_CHAT_ID}
```

```yaml
notify:
  channel: discord
  webhook: ${DISCORD_WEBHOOK_URL}
```

## Notes

- `Notifier` interface + one implementation per channel
- Called at lifecycle transition points in processor
- Message should include: project name, event, direct link (PR URL, spec path)
