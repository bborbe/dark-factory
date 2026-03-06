---
spec: ["024"]
status: created
created: "2026-03-06T00:00:00Z"
---
<summary>
- Adds two nested config structs: `PromptsConfig` (inboxDir, inProgressDir, completedDir, logDir) and `SpecsConfig` (inboxDir, inProgressDir, completedDir, logDir)
- Replaces flat top-level `InboxDir`, `QueueDir`, `CompletedDir`, `LogDir`, `SpecDir` fields with `cfg.Prompts.*` and `cfg.Specs.*`
- Updates `Defaults()` so `prompts.inProgressDir` is `prompts/in-progress` and `specs.*` dirs are under `specs/`
- Updates `partialConfig` and loader to handle nested YAML sections, preserving default-override semantics
- Updates `Validate()` to validate nested fields
- Updates every Create* function in `pkg/factory/factory.go` to use the new config fields
- Updates `pkg/runner/runner.go` to pass `inProgressDir` where `queueDir` was
- Updates `pkg/watcher/watcher.go` internal field name from `queueDir` to `inProgressDir`
- All existing tests must still pass
</summary>

<objective>
Restructure `Config` in `pkg/config/config.go` to use nested `PromptsConfig` and `SpecsConfig` structs and wire all consumers to use the new fields. The flat `QueueDir`, `InboxDir`, `CompletedDir`, `LogDir`, and `SpecDir` top-level fields are removed. Every package that currently reads `cfg.QueueDir`, `cfg.InboxDir`, etc. must switch to `cfg.Prompts.InProgressDir`, `cfg.Prompts.InboxDir`, `cfg.Specs.InboxDir`, etc.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md`.
Read `pkg/config/config.go` — current flat Config struct, Defaults(), Validate().
Read `pkg/config/loader.go` — partialConfig and merging logic.
Read `pkg/config/config_test.go` — tests to update.
Read `pkg/factory/factory.go` — all Create* functions that use cfg fields.
Read `pkg/runner/runner.go` — inboxDir/queueDir/completedDir fields and createDirectories.
Read `pkg/watcher/watcher.go` — queueDir field used for watching.
Read `pkg/review/poller.go` — to see if it accesses cfg fields directly.
</context>

<requirements>
1. In `pkg/config/config.go`, replace the flat fields with nested structs:

   ```go
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
       ProjectName      string        `yaml:"projectName"`
       Workflow         Workflow      `yaml:"workflow"`
       Prompts          PromptsConfig `yaml:"prompts"`
       Specs            SpecsConfig   `yaml:"specs"`
       ContainerImage   string        `yaml:"containerImage"`
       Model            string        `yaml:"model"`
       DebounceMs       int           `yaml:"debounceMs"`
       ServerPort       int           `yaml:"serverPort"`
       AutoMerge        bool          `yaml:"autoMerge"`
       AutoRelease      bool          `yaml:"autoRelease"`
       AutoReview       bool          `yaml:"autoReview"`
       MaxReviewRetries int           `yaml:"maxReviewRetries"`
       AllowedReviewers []string      `yaml:"allowedReviewers,omitempty"`
       UseCollaborators bool          `yaml:"useCollaborators"`
       PollIntervalSec  int           `yaml:"pollIntervalSec"`
       GitHub           GitHubConfig  `yaml:"github"`
   }
   ```

   Remove the following fields entirely: `InboxDir`, `QueueDir`, `CompletedDir`, `LogDir`, `SpecDir`.

2. Update `Defaults()` in `pkg/config/config.go`:

   ```go
   func Defaults() Config {
       return Config{
           Workflow: WorkflowDirect,
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
           ContainerImage:   "docker.io/bborbe/claude-yolo:v0.2.2",
           Model:            "claude-sonnet-4-6",
           DebounceMs:       500,
           ServerPort:       0,
           AutoMerge:        false,
           AutoRelease:      false,
           AutoReview:       false,
           MaxReviewRetries: 3,
           PollIntervalSec:  60,
           UseCollaborators: false,
           GitHub: GitHubConfig{
               Token: "${DARK_FACTORY_GITHUB_TOKEN}",
           }, // #nosec G101 -- env var reference, not a credential
       }
   }
   ```

3. Update `Validate()` in `pkg/config/config.go` to validate the nested fields:
   - Replace `validation.Name("inboxDir", ...)` → validate `c.Prompts.InboxDir`
   - Replace `validation.Name("queueDir", ...)` → validate `c.Prompts.InProgressDir`
   - Replace `validation.Name("completedDir", ...)` → validate `c.Prompts.CompletedDir`
   - Replace `validation.Name("logDir", ...)` → validate `c.Prompts.LogDir`
   - The uniqueness check: `c.Prompts.CompletedDir != c.Prompts.InProgressDir` and `c.Prompts.CompletedDir != c.Prompts.InboxDir`
   - Keep all other validations (workflow, autoMerge, autoRelease, autoReview, serverPort, debounceMs) unchanged

4. In `pkg/config/loader.go`, replace `partialConfig` with nested partial structs:

   ```go
   type partialPromptsConfig struct {
       InboxDir      *string `yaml:"inboxDir"`
       InProgressDir *string `yaml:"inProgressDir"`
       CompletedDir  *string `yaml:"completedDir"`
       LogDir        *string `yaml:"logDir"`
   }

   type partialSpecsConfig struct {
       InboxDir      *string `yaml:"inboxDir"`
       InProgressDir *string `yaml:"inProgressDir"`
       CompletedDir  *string `yaml:"completedDir"`
       LogDir        *string `yaml:"logDir"`
   }

   type partialConfig struct {
       Workflow        *Workflow             `yaml:"workflow"`
       Prompts         *partialPromptsConfig `yaml:"prompts"`
       Specs           *partialSpecsConfig   `yaml:"specs"`
       ContainerImage  *string               `yaml:"containerImage"`
       DebounceMs      *int                  `yaml:"debounceMs"`
       ServerPort      *int                  `yaml:"serverPort"`
       AutoMerge       *bool                 `yaml:"autoMerge"`
       AutoRelease     *bool                 `yaml:"autoRelease"`
       GitHub          *GitHubConfig         `yaml:"github"`
   }
   ```

   Update the merge block in `Load()` to apply nested fields:
   ```go
   if partial.Prompts != nil {
       if partial.Prompts.InboxDir != nil {
           cfg.Prompts.InboxDir = *partial.Prompts.InboxDir
       }
       if partial.Prompts.InProgressDir != nil {
           cfg.Prompts.InProgressDir = *partial.Prompts.InProgressDir
       }
       if partial.Prompts.CompletedDir != nil {
           cfg.Prompts.CompletedDir = *partial.Prompts.CompletedDir
       }
       if partial.Prompts.LogDir != nil {
           cfg.Prompts.LogDir = *partial.Prompts.LogDir
       }
   }
   if partial.Specs != nil {
       if partial.Specs.InboxDir != nil {
           cfg.Specs.InboxDir = *partial.Specs.InboxDir
       }
       if partial.Specs.InProgressDir != nil {
           cfg.Specs.InProgressDir = *partial.Specs.InProgressDir
       }
       if partial.Specs.CompletedDir != nil {
           cfg.Specs.CompletedDir = *partial.Specs.CompletedDir
       }
       if partial.Specs.LogDir != nil {
           cfg.Specs.LogDir = *partial.Specs.LogDir
       }
   }
   ```
   Remove the old `InboxDir`, `QueueDir`, `CompletedDir`, `LogDir` merge lines. Keep `Workflow`, `ContainerImage`, `DebounceMs`, `ServerPort`, `AutoMerge`, `AutoRelease`, `GitHub` merges.

5. Update `pkg/factory/factory.go` — replace every `cfg.InboxDir`, `cfg.QueueDir`, `cfg.CompletedDir`, `cfg.LogDir`, `cfg.SpecDir` reference:

   | Old | New |
   |-----|-----|
   | `cfg.InboxDir` | `cfg.Prompts.InboxDir` |
   | `cfg.QueueDir` | `cfg.Prompts.InProgressDir` |
   | `cfg.CompletedDir` | `cfg.Prompts.CompletedDir` |
   | `cfg.LogDir` | `cfg.Prompts.LogDir` |
   | `cfg.SpecDir` | `cfg.Specs.InboxDir` (for now — later prompts will split this further) |

   Specifically in `CreateRunner`:
   - `inboxDir := cfg.Prompts.InboxDir`
   - `inProgressDir := cfg.Prompts.InProgressDir` (rename local var from `queueDir`)
   - `completedDir := cfg.Prompts.CompletedDir`
   - All downstream calls use `inProgressDir` where `queueDir` was

   In `CreateSpecGenerator`:
   - `cfg.Prompts.InboxDir`, `cfg.Prompts.CompletedDir`, `cfg.Specs.InboxDir`, `cfg.Specs.LogDir`

   In `CreateSpecWatcher`:
   - `cfg.Specs.InboxDir` (the dir being watched — later prompt will change to InProgressDir)

   In `CreateReviewPoller`:
   - `cfg.Prompts.InProgressDir`, `cfg.Prompts.InboxDir`

   In `CreateStatusCommand`, `CreateQueueCommand`, `CreateListCommand`, `CreateRequeueCommand`, `CreateApproveCommand`:
   - Use `cfg.Prompts.InProgressDir`, `cfg.Prompts.InboxDir`, `cfg.Prompts.CompletedDir`, `cfg.Prompts.LogDir`

   In `CreateSpecListCommand`, `CreateSpecStatusCommand`, `CreateSpecApproveCommand`, `CreateSpecVerifyCommand`:
   - Use `cfg.Specs.InboxDir` (single dir for now — later prompts add multi-dir)

   In `CreateCombinedStatusCommand`, `CreateCombinedListCommand`:
   - Use `cfg.Prompts.*` fields for prompt dirs, `cfg.Specs.InboxDir` for spec lister

   In `CreateServer`:
   - `cfg.Prompts.InboxDir`, `cfg.Prompts.InProgressDir`, `cfg.Prompts.CompletedDir`, `cfg.Prompts.LogDir`

6. In `pkg/runner/runner.go`:
   - Rename the field `queueDir string` → `inProgressDir string` in the `runner` struct
   - Update `NewRunner` parameter `queueDir string` → `inProgressDir string`
   - Update all uses of `r.queueDir` → `r.inProgressDir`
   - Update `createDirectories` to use `r.inProgressDir`
   - Update the log message: `"watching for queued prompts", "dir", r.inProgressDir`

7. In `pkg/watcher/watcher.go`:
   - Rename the field `queueDir string` → `inProgressDir string` in the `watcher` struct
   - Update `NewWatcher` parameter `queueDir string` → `inProgressDir string`
   - Update all uses of `w.queueDir` → `w.inProgressDir`
   - Update `getQueueDir()` → `getInProgressDir()` method name
   - Update log messages to say `inProgressDir` instead of `queueDir`

8. In `pkg/watcher/watcher_test.go`:
   - Update any references to `queueDir` parameter → `inProgressDir`

9. Update `pkg/config/config_test.go`:
   - Update `Defaults()` assertions to use `cfg.Prompts.InboxDir`, etc.
   - Update any test that sets `cfg.QueueDir`, `cfg.InboxDir`, etc. to use nested fields
   - Update validation tests to reference the new field names

10. Run `make generate` to regenerate any counterfeiter mocks that changed (check if any interface signatures changed — Runner, Watcher interfaces are unchanged so mocks likely don't need regeneration; config.Loader interface is unchanged).
</requirements>

<constraints>
- Do NOT add a migration path — that is handled in prompt 121
- The `prompts/queue/` directory is NOT renamed by this prompt — only the config field name changes
- Keep `createDirectories` in runner.go working (it uses the dir fields)
- Feature flags (`autoMerge`, `autoRelease`, `autoReview`) remain in the flat top-level Config (not moved to nested)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm old flat fields are gone
grep -rn 'cfg\.QueueDir\|cfg\.InboxDir\|cfg\.CompletedDir\|cfg\.LogDir\|cfg\.SpecDir' pkg/ && echo "FAIL: old fields remain" || echo "OK"

# Confirm new nested fields are used
grep -n 'cfg\.Prompts\.' pkg/factory/factory.go | head -20

# Confirm runner uses inProgressDir
grep -n 'inProgressDir' pkg/runner/runner.go

# Confirm watcher uses inProgressDir
grep -n 'inProgressDir' pkg/watcher/watcher.go

# Confirm defaults are correct
go test ./pkg/config/... -v -run TestDefaults
```
</verification>
