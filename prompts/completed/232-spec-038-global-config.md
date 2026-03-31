---
status: completed
spec: [038-global-container-limit]
summary: Created pkg/globalconfig package with MaxContainers field, counterfeiter mock, full test coverage (96%), and updated dark-factory config output to show global section alongside project config
container: dark-factory-232-spec-038-global-config
dark-factory-version: v0.80.0-1-g2b37ac1
created: "2026-03-31T18:00:00Z"
queued: "2026-03-31T19:11:26Z"
started: "2026-03-31T19:22:57Z"
completed: "2026-03-31T19:52:53Z"
branch: dark-factory/global-container-limit
---

<summary>
- Users can configure a global dark-factory settings file at ~/.dark-factory/config.yaml
- Missing file is not an error — sensible defaults apply (max 3 containers)
- Invalid config produces a clear error message at load time
- `dark-factory config` output gains a global section showing the resolved settings
- New package is independent from per-project config
</summary>

<objective>
Create `pkg/globalconfig` — a minimal package for reading `~/.dark-factory/config.yaml` with a `MaxContainers` field — and update `dark-factory config` to display the global config alongside the project config.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — interface→constructor→struct, counterfeiter annotations, error wrapping.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega, suite setup, external test packages, ≥80% coverage.
Read `pkg/config/loader.go` — the existing per-project config loader pattern to follow.
Read `main.go` lines 151–156 — the existing `printConfig` function to modify.
Read `pkg/config/config.go` lines 1–30 — for import path (`github.com/bborbe/dark-factory/pkg/config`).
</context>

<requirements>
1. Create `pkg/globalconfig/doc.go`:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package globalconfig loads and validates the user-level dark-factory configuration
   // from ~/.dark-factory/config.yaml.
   package globalconfig
   ```

2. Create `pkg/globalconfig/globalconfig.go` with the following:

   **Struct:**
   ```go
   // GlobalConfig holds the user-level dark-factory configuration.
   // It is loaded from ~/.dark-factory/config.yaml once at daemon startup.
   // When the file does not exist or the field is omitted, defaults apply.
   type GlobalConfig struct {
       MaxContainers int `yaml:"maxContainers"`
   }
   ```

   **Defaults:**
   ```go
   // DefaultMaxContainers is the system-wide container limit when no config is set.
   const DefaultMaxContainers = 3

   // defaults returns a GlobalConfig with all default values.
   func defaults() GlobalConfig {
       return GlobalConfig{
           MaxContainers: DefaultMaxContainers,
       }
   }
   ```

   **Validation:**
   ```go
   // Validate validates the GlobalConfig fields.
   func (g GlobalConfig) Validate(ctx context.Context) error {
       if g.MaxContainers < 1 {
           return errors.Errorf(ctx, "globalconfig: maxContainers must be >= 1, got %d", g.MaxContainers)
       }
       return nil
   }
   ```

   **Interface + constructor:**
   ```go
   //counterfeiter:generate -o ../../mocks/global-config-loader.go --fake-name GlobalConfigLoader . Loader

   // Loader loads the global dark-factory configuration.
   type Loader interface {
       Load(ctx context.Context) (GlobalConfig, error)
   }

   // NewLoader creates a Loader that reads from ~/.dark-factory/config.yaml.
   func NewLoader() Loader {
       return &fileLoader{}
   }

   // fileLoader implements Loader by reading ~/.dark-factory/config.yaml.
   type fileLoader struct{}
   ```

   **Load method:**
   ```go
   // Load reads ~/.dark-factory/config.yaml, merges with defaults, validates, and returns the config.
   // If the file does not exist or is empty, defaults are returned without error.
   func (l *fileLoader) Load(ctx context.Context) (GlobalConfig, error) {
       cfg := defaults()

       home, err := os.UserHomeDir()
       if err != nil {
           return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: get home directory")
       }

       configPath := filepath.Join(home, ".dark-factory", "config.yaml")

       // #nosec G304 -- configPath is derived from user home dir, not user input
       data, err := os.ReadFile(configPath)
       if err != nil {
           if os.IsNotExist(err) {
               return cfg, nil
           }
           return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: read config file")
       }

       // Empty file → return defaults
       if len(bytes.TrimSpace(data)) == 0 {
           return cfg, nil
       }

       // partial struct to detect which fields were set (vs omitted)
       var partial struct {
           MaxContainers *int `yaml:"maxContainers"`
       }
       if err := yaml.Unmarshal(data, &partial); err != nil {
           return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: parse config file")
       }

       if partial.MaxContainers != nil {
           cfg.MaxContainers = *partial.MaxContainers
       }

       if err := cfg.Validate(ctx); err != nil {
           return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: validate")
       }

       return cfg, nil
   }
   ```

   **Required imports for `globalconfig.go`:**
   ```go
   import (
       "bytes"
       "context"
       "os"
       "path/filepath"

       "github.com/bborbe/errors"
       "gopkg.in/yaml.v3"
   )
   ```

3. Create `pkg/globalconfig/globalconfig_suite_test.go`:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package globalconfig_test

   import (
       "testing"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   func TestGlobalconfig(t *testing.T) {
       RegisterFailHandler(Fail)
       RunSpecs(t, "Globalconfig Suite")
   }
   ```

4. Create `pkg/globalconfig/globalconfig_test.go` with tests:

   Add a package-level variable `userHomeDir = os.UserHomeDir` so tests can override it. Update the `fileLoader.Load` to call `userHomeDir()` instead of `os.UserHomeDir()` directly.

   **Add to `globalconfig.go`:**
   ```go
   // userHomeDir is a variable so tests can override the home directory lookup.
   var userHomeDir = os.UserHomeDir
   ```
   Update `Load` to use `userHomeDir()` instead of `os.UserHomeDir()`.

   **Test file:**
   ```go
   package globalconfig_test

   import (
       "context"
       "os"
       "path/filepath"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/dark-factory/pkg/globalconfig"
   )
   ```

   Because `userHomeDir` is an unexported package variable in `globalconfig`, the tests in external package `globalconfig_test` cannot override it. Instead, **add an internal test file** `pkg/globalconfig/globalconfig_internal_test.go` with package `globalconfig` to override `userHomeDir`:

   ```go
   package globalconfig

   import (
       "context"
       "fmt"
       "os"
       "path/filepath"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   var _ = Describe("fileLoader.Load", func() {
       var (
           ctx     context.Context
           tmpDir  string
           origFn  func() (string, error)
       )

       BeforeEach(func() {
           ctx = context.Background()
           var err error
           tmpDir, err = os.MkdirTemp("", "globalconfig-test-*")
           Expect(err).NotTo(HaveOccurred())

           origFn = userHomeDir
           userHomeDir = func() (string, error) { return tmpDir, nil }
       })

       AfterEach(func() {
           userHomeDir = origFn
           Expect(os.RemoveAll(tmpDir)).To(Succeed())
       })

       writeConfig := func(content string) {
           dir := filepath.Join(tmpDir, ".dark-factory")
           Expect(os.MkdirAll(dir, 0750)).To(Succeed())
           path := filepath.Join(dir, "config.yaml")
           Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
       }

       It("returns defaults when file does not exist", func() {
           cfg, err := NewLoader().Load(ctx)
           Expect(err).NotTo(HaveOccurred())
           Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
       })

       It("returns defaults when file is empty", func() {
           writeConfig("")
           cfg, err := NewLoader().Load(ctx)
           Expect(err).NotTo(HaveOccurred())
           Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
       })

       It("returns defaults when file is only whitespace", func() {
           writeConfig("   \n  ")
           cfg, err := NewLoader().Load(ctx)
           Expect(err).NotTo(HaveOccurred())
           Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
       })

       It("reads maxContainers from file", func() {
           writeConfig("maxContainers: 5\n")
           cfg, err := NewLoader().Load(ctx)
           Expect(err).NotTo(HaveOccurred())
           Expect(cfg.MaxContainers).To(Equal(5))
       })

       It("returns default maxContainers when field is omitted", func() {
           writeConfig("# no fields\n")
           cfg, err := NewLoader().Load(ctx)
           Expect(err).NotTo(HaveOccurred())
           Expect(cfg.MaxContainers).To(Equal(DefaultMaxContainers))
       })

       It("returns error for invalid YAML", func() {
           writeConfig("maxContainers: [not an int\n")
           _, err := NewLoader().Load(ctx)
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("parse config file"))
       })

       It("returns error when maxContainers is 0", func() {
           writeConfig("maxContainers: 0\n")
           _, err := NewLoader().Load(ctx)
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("maxContainers must be >= 1"))
       })

       It("returns error when maxContainers is negative", func() {
           writeConfig(fmt.Sprintf("maxContainers: %d\n", -1))
           _, err := NewLoader().Load(ctx)
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("maxContainers must be >= 1"))
       })

       It("returns error when home dir lookup fails", func() {
           userHomeDir = func() (string, error) { return "", fmt.Errorf("no home") }
           _, err := NewLoader().Load(ctx)
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("get home directory"))
       })
   })
   ```

   Also add a test for `GlobalConfig.Validate` in an external test file `globalconfig_test.go`:
   ```go
   package globalconfig_test

   import (
       "context"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/dark-factory/pkg/globalconfig"
   )

   var _ = Describe("GlobalConfig.Validate", func() {
       var ctx context.Context
       BeforeEach(func() { ctx = context.Background() })

       It("returns nil for valid config", func() {
           cfg := globalconfig.GlobalConfig{MaxContainers: 3}
           Expect(cfg.Validate(ctx)).To(Succeed())
       })

       It("returns error when MaxContainers is 0", func() {
           cfg := globalconfig.GlobalConfig{MaxContainers: 0}
           Expect(cfg.Validate(ctx)).To(HaveOccurred())
       })

       It("returns error when MaxContainers is negative", func() {
           cfg := globalconfig.GlobalConfig{MaxContainers: -1}
           Expect(cfg.Validate(ctx)).To(HaveOccurred())
       })

       It("returns nil for MaxContainers of 1", func() {
           cfg := globalconfig.GlobalConfig{MaxContainers: 1}
           Expect(cfg.Validate(ctx)).To(Succeed())
       })
   })
   ```

5. Run `make generate` to create the counterfeiter mock:
   ```bash
   cd /workspace && make generate
   ```
   Verify `mocks/global-config-loader.go` is created.

6. Update `main.go` — modify `printConfig` to load and include the global config:

   Change the signature to:
   ```go
   func printConfig(cfg config.Config) error {
       ctx := context.Background()
       globalCfg, err := globalconfig.NewLoader().Load(ctx)
       if err != nil {
           return err
       }

       type output struct {
           Global  globalconfig.GlobalConfig `yaml:"global"`
           Project config.Config             `yaml:"project"`
       }
       out := output{
           Global:  globalCfg,
           Project: cfg,
       }

       enc := yaml.NewEncoder(os.Stdout)
       enc.SetIndent(2)
       defer enc.Close()
       return enc.Encode(out)
   }
   ```

   Add `"github.com/bborbe/dark-factory/pkg/globalconfig"` to imports in `main.go`.

   Note: wrapping project config under a `project:` key is intentional — it separates global from per-project config clearly in the output.

   Expected output of `dark-factory config`:
   ```yaml
   global:
     maxContainers: 3
   project:
     projectName: ""
     workflow: direct
     ...
   ```

7. Update CHANGELOG.md — add to `## Unreleased` section (create if missing):
   ```
   - feat: Add `pkg/globalconfig` package to load ~/.dark-factory/config.yaml with maxContainers field
   - feat: Show global config section in `dark-factory config` output
   ```
</requirements>

<constraints>
- Global config is fully independent of per-project `.dark-factory.yaml` — do NOT add any field to `config.Config`
- Missing `~/.dark-factory/config.yaml` is not an error; return defaults silently
- Empty file or file with only whitespace returns defaults without error
- Invalid YAML or `maxContainers < 1` returns a descriptive error
- `userHomeDir` package variable is the only test seam — do not add any other injection mechanism
- Use `errors.Wrap(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Use `gopkg.in/yaml.v3` (already in vendor)
- Do NOT commit — dark-factory handles git
- All existing tests must continue to pass
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm new package files exist
ls pkg/globalconfig/doc.go pkg/globalconfig/globalconfig.go

# Confirm counterfeiter mock generated
ls mocks/global-config-loader.go

# Run package tests with coverage
go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/globalconfig/...
go tool cover -func=/tmp/cover.out | grep total
# Expected: ≥80%

# Confirm global section appears in config output
dark-factory config 2>/dev/null | grep -A2 "global:"
# Expected: global: section with maxContainers

# Confirm no globalconfig import in pkg/config (no circular deps)
grep -r "globalconfig" pkg/config/
# Expected: no output
```
</verification>
