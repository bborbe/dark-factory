---
status: approved
spec: [095-healthcheck-cli]
created: "2026-06-16T13:00:10Z"
queued: "2026-06-16T13:18:11Z"
branch: dark-factory/healthcheck-cli
---

<summary>
- Three external probes are added under `pkg/cmd/healthcheck/`: `claude`, `gh`, and `notifications`, each as a new exported constructor returning the `Probe` interface.
- The `claude` probe runs `claude -p "reply with exactly: OK"` inside a one-shot container with a hard 10-second timeout; it asserts the literal `OK` substring is present in stdout.
- The `gh` probe runs `gh auth status` via the existing `subproc.Runner` with a 5-second timeout; it returns nil on exit 0, an error otherwise.
- The `notifications` probe posts a fixed minimal JSON payload to the configured channel URL via `net/http` with a 5-second client timeout, asserting a 2xx response. The bot token / webhook URL is NEVER logged at INFO or ERROR; only the host of the URL and the status code are surfaced.
- The `HealthcheckCommand.Run` orchestrates the full seven-probe sequence in fixed order: docker → image → boot → claude → mount → gh → notifications. The `--no-claude` flag skips only the claude probe. Probes 6 and 7 are config-gated (`pr: true` for gh, presence of any notifications channel for notifications) — no flag controls them.
- Unit tests for each new probe cover the success and failure paths using a fake `subproc.Runner` and a fake HTTP transport.
- The full integration path is covered by a new test in `pkg/cmd/healthcheck_test.go` that wires a complete probe slice and asserts the seven categories appear in the slog output in order.
</summary>

<objective>
Add the three external probes (claude, gh, notifications) to the `healthcheck` command, complete the seven-probe sequence with proper config-gating, and verify wall-clock and fail-fast behavior.
</objective>

<context>
Read these files first (paths absolute):
- `/workspace/pkg/cmd/healthcheck.go` — output of prompt 2a; extend the orchestration in `Run`
- `/workspace/pkg/cmd/healthcheck/probes.go` — extend with three new constructor functions
- `/workspace/pkg/cmd/healthcheck/probes_test.go` — extend with three new test groups
- `/workspace/pkg/cmd/healthcheck_test.go` — extend the orchestration test
- `/workspace/pkg/notifier/telegram.go` and `/workspace/pkg/notifier/discord.go` — for the notifications HTTP POST shape (Content-Type, JSON body, 2xx check); re-implement inline, do NOT import (the existing notifier packages log the channel and token via slog)
- `/workspace/pkg/config/config.go` lines 38-42 (NotificationsConfig), lines 84-127 (full Config), lines 711-736 (ResolvedTelegramBotToken, ResolvedDiscordWebhook, ResolvedTelegramChatID)
- `/workspace/pkg/subproc/subproc.go` — `subproc.Runner.RunWithWarnAndTimeout` signature
- Coding plugin docs (in-container): `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
</context>

<requirements>

1. Add three new exported probe constructors to `/workspace/pkg/cmd/healthcheck/probes.go`:
   - `NewClaudeProbe(containerImage string, projectName string, subproc.Runner) Probe`
     - Spawns `docker run --rm --name <random> <image> claude -p "reply with exactly: OK"`. Container name uses prefix `projectName + "-healthcheck-claude-" + <random>` (8 random hex chars; use `crypto/rand`).
     - Hard timeout: 10 seconds. Use `subproc.NewRunnerWithThresholds(2*time.Second, 10*time.Second)`.
     - On success: stdout MUST contain the literal substring `OK`. If missing OR exit non-zero, return `errors.Errorf(ctx, "claude session probe failed: stdout=%q", trunc(stdout,200))`.
     - The Claude prompt content is a hard-coded constant `claudeProbePrompt = "reply with exactly: OK"` defined in this file. No user input is ever interpolated (per the spec's Security section).
   - `NewGhProbe(subproc.Runner) Probe`
     - Runs `gh auth status` via `subproc.Runner.RunWithWarnAndTimeout(ctx, "gh auth status", "gh", "auth", "status")` with 3s/5s thresholds.
     - `subproc.Runner.RunWithWarnAndTimeout(...) ([]byte, error)` returns ONLY stdout — the `[]byte` is the captured stdout. **`gh auth status` writes its diagnostic output to stdout**, so the wrapper uses the returned `[]byte`. On non-zero exit, return `errors.Wrapf(ctx, err, "gh auth status failed: stdout=%q", trunc(stdout,200))`. The probe does NOT log the gh token (none is read here).
     - The probe is unconditional from its own perspective; the config-gating (`pr: true`) is enforced by the orchestrator in step 3.
   - `NewNotificationsProbe(cfg config.Config, client *http.Client) Probe` — **final signature**; the `*http.Client` parameter enables test-double injection (see req 5 tests). The factory passes `&http.Client{Timeout: 5*time.Second}`.
     - At construction time, resolve the active channel. If `cfg.Notifications.Telegram.BotTokenEnv` is set AND `cfg.ResolvedTelegramBotToken() != ""`, store the Telegram URL `https://api.telegram.org/bot<token>/sendMessage` (the FULL URL stays in memory but is never logged). If `cfg.Notifications.Discord.WebhookEnv` is set AND `cfg.ResolvedDiscordWebhook() != ""`, store the Discord webhook URL. If NEITHER is set, the constructor still returns a probe whose `Run` early-returns `nil` with an `slog.Debug` line `"notifications probe: no channel configured, skipping"`. The orchestrator's `NotificationsConfigured` gate (step 2) will normally omit this probe from the iteration list when not configured; the early-return in `Run` is a defensive safety net.
     - On `Run`: build the JSON payload (see below), POST it with the injected `client` (whose timeout is set by the factory), assert 2xx. The URL host (parsed via `url.Parse(req.URL.String()).Host`) is logged at INFO/ERROR; the URL path (which contains the token for Telegram) is NEVER logged.
     - Telegram payload: `{"chat_id":"<chatID>","text":"dark-factory healthcheck"}` (a fixed minimal body).
     - Discord payload: `{"content":"dark-factory healthcheck"}`.
     - Use `net/http.NewRequestWithContext` with the injected `client` parameter. Do NOT use `http.DefaultClient` (per the spec's hard-timeout requirement).
     - On non-2xx, read up to 200 chars of response body and return `errors.Errorf(ctx, "notifications POST failed: status=%d body=%q", resp.StatusCode, trunc(body,200))`.
     - On transport error, return `errors.Wrap(ctx, err, "notifications POST transport error")`.

2. Add a sentinel `NotificationsConfigured(cfg config.Config) bool` helper to the same file:
   ```go
   func NotificationsConfigured(cfg config.Config) bool {
       if cfg.Notifications.Telegram.BotTokenEnv != "" && cfg.ResolvedTelegramBotToken() != "" {
           return true
       }
       if cfg.Notifications.Discord.WebhookEnv != "" && cfg.ResolvedDiscordWebhook() != "" {
           return true
       }
       return false
   }
   ```
   This is the config-gate the orchestrator uses. The Telegram channel is "configured" if the env var name is set; "operational" requires the env var to be non-empty. The probe handles both cases by no-oping when the URL is empty.

3. Modify `/workspace/pkg/cmd/healthcheck.go` `Run` to execute the full seven-probe sequence:
   - Probe list in fixed order: `docker`, `image`, `boot`, `claude`, `mount`, `gh`, `notifications`.
   - If `--no-claude` is in args, OMIT the claude probe from the iteration list. The remaining six run unchanged. (The flag is consumed by the parser; do not pass it to the probe as a parameter.)
   - The `gh` probe is OMITTED from the iteration list when `!cfg.PR`. The `notifications` probe is OMITTED when `!NotificationsConfigured(cfg)`. Both are config-gated, not flag-gated — there are no `--no-gh` or `--no-notifications` flags.
   - Iterate with fail-fast semantics (same as prompt 2a). On first error, log the failing category, print the categorized table, return the error.
   - On all pass, print `all probes passed` and return nil.
   - The new flag-parsing logic in `Run` is unchanged from prompt 2a: `--no-claude` is consumed; `--help`/`-h` print help; unknown flags error.

4. Update `/workspace/pkg/factory/factory.go` `CreateHealthcheckCommand` to:
   - Build the probe slice in the seven-probe order, with the config-gating logic from step 3 baked into the slice construction (the factory is the right place — it has `cfg` in scope and can decide which probes to instantiate). The factory returns a fully-configured `[]healthcheck.Probe` that the command iterates as-is.
   - The factory stays zero-logic: it constructs the slice and passes it to `cmd.NewHealthcheckCommand`. The flag-parsing and iteration live in `cmd.HealthcheckCommand.Run`.

5. Add tests to `/workspace/pkg/cmd/healthcheck/probes_test.go`:
   - For `NewClaudeProbe`: one `It` that drives a fake `subproc.Runner` returning stdout=`"OK\n"` and exit 0 → expect `nil`; one that returns stdout=`"unexpected\n"` → expect error containing `claude session probe failed`. The fake must support `RunWithWarnAndTimeout` returning a non-zero exit (verify by reading the existing fake at `/workspace/mocks/subproc-runner.go`).
   - For `NewGhProbe`: one `It` that drives the fake returning exit 0 → nil; one returning exit 1 → error wrapping `gh auth status failed`.
   - For `NewNotificationsProbe(cfg, client)`: one `It` covering the no-config case (returns nil silently, no HTTP call) — pass a zero `cfg.Notifications` and any client; assert `client` was not invoked. One `It` covering a 200 response via a fake `http.RoundTripper` injected into the `http.Client` (tests pass a `*http.Client` wrapping a `RoundTripperFunc` test double). One `It` covering a 401 response with a 200-char body snippet check.

6. Extend `/workspace/pkg/cmd/healthcheck_test.go`:
   - New `It` block: "executes all seven probes in fixed order when all are configured and enabled" — wire seven `&mocks.HealthcheckProbe{}` whose `NameReturns` returns the seven names in order. Pass `args=[]` (no `--no-claude`), `cfg.PR=true`, and a config with Telegram configured. Call `cmd.Run`. Capture slog output (use the same `slog.SetDefault` trick from the existing test file at `/workspace/pkg/cmd/doctor_test.go`, or a test-double logger). Assert the seven names appear in the captured output in the exact order: docker, image, boot, claude, mount, gh, notifications.
   - New `It`: "skips claude when --no-claude is passed" — same setup but args=`["--no-claude"]`. Assert the captured slog output does NOT contain the substring `probe=claude`.
   - New `It`: "skips gh when pr is false" — args=`[]`, `cfg.PR=false`. Assert the captured slog output does NOT contain `probe=gh`.
   - New `It`: "skips notifications when no channel configured" — args=`[]`, `cfg.PR=true`, but `cfg.Notifications` is the zero value. Assert the captured slog output does NOT contain `probe=notifications`.

7. The "boot" probe in this prompt is the one already implemented in prompt 1 (`runner.BootContainerProbe`). Do NOT duplicate its logic. The factory in step 4 instantiates one instance via the existing constructor from prompt 1.

8. Wall-clock budget is enforced by the timeouts in the probe constructors:
   - docker: 10s timeout (subproc)
   - image: 10s timeout (subproc)
   - boot: 30s (subproc, in `runner/probe.go`) + 15s (`WaitUntilRunning`) = ~45s worst case
   - claude: 10s timeout
   - mount: 15s timeout
   - gh: 5s timeout
   - notifications: 5s timeout
   - Sum worst-case: ~100s. The spec's AC says "≤ 15s full run / ≤ 5s --no-claude" on a warm sandbox with image already pulled. Do not change the timeouts; the AC is a property of the green path, not the worst case.

9. Run `go generate ./...`, then `make test`, then `make precommit`.

10. End-to-end smoke (if Docker is available in the YOLO container):
    ```bash
    go build -o /tmp/df-095b .
    /tmp/df-095b healthcheck --help
    /tmp/df-095b healthcheck --no-claude   # green sandbox → exit 0
    /tmp/df-095b healthcheck               # green sandbox → may take ~10s for claude; exit 0
    ```
    If Docker is NOT available in the YOLO container, document this in `## Improvements` and rely on the unit tests for verification.

</requirements>

<constraints>
- The shape of `dark-factory doctor`'s exit semantics (0 = clean, non-zero = findings) and table layout must be preserved by `healthcheck` — operators read both outputs and the mental model must stay one model.
- `pkg/runner/health_check.go` (spec 043, periodic container-liveness check) is not modified by this spec. It is a different surface; both can co-exist.
- The container-boot helper extracted under `pkg/runner/` must remain callable from both the `healthcheck` command and the scenario-003 harness; the helper must not depend on Cobra, on the `pkg/cmd/` package, or on the scenarios package — only on `pkg/runner/` and below.
- The `.dark-factory.yaml` schema is not extended by this spec (no new fields, no new defaults).
- All new Go code conforms to the project's coding rules: `errors.Wrapf` from `github.com/bborbe/errors` for every error path; no bare `return err`; no `fmt.Errorf`; `log/slog` to stderr.
- The probe execution must respect context cancellation: `Ctrl-C` aborts within ~1s and the command exits with a non-zero code distinct from a probe failure (agent decides exact code at impl time).
- See `docs/troubleshooting.md` for the operator-facing diagnostic flow this command slots into; the doc update lives in prompt 3.
- See `docs/running.md` for the operator-facing "how to run dark-factory" surface this command is added to.
- The notifications probe MUST NOT log the channel token (full Telegram bot token or Discord webhook URL). The URL host is acceptable; the URL path is not. The constructor stores the full URL in memory but every `slog` call logs only the parsed `url.Host`.
- Per-probe hard timeouts (Claude ≤ 10s, notifications HTTP ≤ 5s) prevent unbounded hangs from a malicious or broken external endpoint.
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
```bash
make test
make precommit
```
Manual:
- `git diff origin/master..HEAD -- '*.go' | grep -nE '^\+\s*return err$'` returns 0 lines.
- `git diff origin/master..HEAD -- '*.go' | grep -nE '^\+.*fmt\.Errorf'` returns 0 lines in files under `pkg/cmd/healthcheck` or `pkg/runner/probe`.
- `grep -rE 'probe=docker|probe=image|probe=boot|probe=claude|probe=mount|probe=gh|probe=notifications' pkg/cmd/healthcheck/` returns 7 matches (one per probe in its log line).
- `grep -rE 'bot[0-9]+:[A-Za-z0-9_-]+' pkg/cmd/healthcheck/probes.go` must return 0 lines outside the URL construction block (the token is concatenated into a URL string but never appears in a log line).
- Run `go build -o /tmp/df-095b . && /tmp/df-095b healthcheck --help | grep -c -- '--no-claude'` returns 1.
</verification>
