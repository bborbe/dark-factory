---
status: failed
spec: ["030"]
container: dark-factory-171-spec-030-config-and-url-parser
dark-factory-version: v0.44.0
created: "2026-03-11T10:00:00Z"
queued: "2026-03-11T13:26:27Z"
started: "2026-03-11T13:26:29Z"
completed: "2026-03-11T13:26:29Z"
branch: dark-factory/bitbucket-server-pr-workflow
---
<summary>
- A new `provider` config field selects the git provider — `github` (default) or `bitbucket-server`
- `provider: invalid` fails config validation at startup with a clear error message
- A new `bitbucket:` config section holds `baseURL` and `tokenEnv` (env var name, default: `BITBUCKET_TOKEN`) for Bitbucket Server connection
- When `provider: bitbucket-server`, the config requires `bitbucket.baseURL` — missing it fails validation
- A utility parses Bitbucket Server git remote URLs (both SSH and HTTPS formats) and extracts the project key and repo slug
- A `ResolvedBitbucketToken()` helper reads the token from the env var named in `tokenEnv` (default: `BITBUCKET_TOKEN`)
- All existing GitHub config and behavior is unchanged when `provider` is `github` or omitted
</summary>

<objective>
Add `provider` and `bitbucket:` fields to the dark-factory config, implement the provider enum with validation, add a Bitbucket-specific token resolver, and implement a Bitbucket remote URL parser that extracts project key and repo slug. This is the config foundation for the Bitbucket Server integration — subsequent prompts build the API client and wire it into the factory.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — interface/constructor/struct pattern, error wrapping.
Read `/home/node/.claude/docs/go-enum-pattern.md` — how to define string enums with `Available*`, `Validate()`, plural type, `Contains()`.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega test patterns, external test packages.

Read these files before making any changes:
- `pkg/config/config.go` — `Config` struct, `GitHubConfig`, `Defaults()`, `Validate()`, `resolveEnvVar()`, `ResolvedGitHubToken()`
- `pkg/config/workflow.go` — `Workflow` enum pattern to follow for `Provider`
- `pkg/config/config_suite_test.go` — test suite setup
- `docs/bitbucket-server-api-reference.md` — remote URL format examples (SSH and HTTPS)
</context>

<requirements>
**Step 1: Provider enum in `pkg/config/provider.go`**

Create `pkg/config/provider.go` following the same pattern as `pkg/config/workflow.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
    "context"

    "github.com/bborbe/collection"
    "github.com/bborbe/errors"
    "github.com/bborbe/validation"
)

// Provider selects the git hosting provider for PR operations.
const (
    ProviderGitHub          Provider = "github"
    ProviderBitbucketServer Provider = "bitbucket-server"
)

// AvailableProviders contains all valid provider values.
var AvailableProviders = Providers{ProviderGitHub, ProviderBitbucketServer}

// Provider is a string-based enum for git provider types.
type Provider string

// String returns the string representation of the Provider.
func (p Provider) String() string {
    return string(p)
}

// Validate checks that the Provider is a known value.
func (p Provider) Validate(ctx context.Context) error {
    if !AvailableProviders.Contains(p) {
        return errors.Wrapf(ctx, validation.Error, "unknown provider %q, valid values: github, bitbucket-server", p)
    }
    return nil
}

// Ptr returns a pointer to the Provider value.
func (p Provider) Ptr() *Provider {
    return &p
}

// Providers is a collection of Provider values.
type Providers []Provider

// Contains returns true if the given provider is in the collection.
func (p Providers) Contains(provider Provider) bool {
    return collection.Contains(p, provider)
}
```

**Step 2: BitbucketConfig and Config changes in `pkg/config/config.go`**

1. Add `BitbucketConfig` struct immediately after `GitHubConfig`:
   ```go
   // BitbucketConfig holds Bitbucket Server-specific configuration.
   type BitbucketConfig struct {
       BaseURL  string `yaml:"baseURL"`
       TokenEnv string `yaml:"tokenEnv"`
   }
   ```

2. Add two fields to the `Config` struct, immediately after `GitHub GitHubConfig`:
   ```go
   Provider  Provider        `yaml:"provider"`
   Bitbucket BitbucketConfig `yaml:"bitbucket"`
   ```

3. In `Defaults()`, set the default provider:
   ```go
   Provider:  ProviderGitHub,
   Bitbucket: BitbucketConfig{TokenEnv: "BITBUCKET_TOKEN"},
   ```
   Add this after `GitHub: GitHubConfig{}`.

4. In `Validate()`, add two new validation rules in the `validation.All{}` slice:
   ```go
   validation.Name("provider", c.Provider),
   validation.Name("bitbucket", validation.HasValidationFunc(c.validateBitbucketConfig)),
   ```

5. Add the `validateBitbucketConfig` method:
   ```go
   // validateBitbucketConfig validates the bitbucket configuration when provider is bitbucket-server.
   func (c Config) validateBitbucketConfig(ctx context.Context) error {
       if c.Provider != ProviderBitbucketServer {
           return nil
       }
       if c.Bitbucket.BaseURL == "" {
           return errors.Errorf(ctx, "bitbucket.baseURL is required when provider is bitbucket-server")
       }
       return nil
   }
   ```

6. Add `ResolvedBitbucketToken()` method immediately after `ResolvedGitHubToken()`:
   ```go
   // ResolvedBitbucketToken reads the Bitbucket token from the env var named in TokenEnv.
   // Returns empty string when not configured or env var is empty.
   // Uses os.Getenv directly (not resolveEnvVar) because resolveEnvVar is GitHub-specific.
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
   ```

**Step 3: Bitbucket remote URL parser in `pkg/git/bitbucket_remote.go`**

Create `pkg/git/bitbucket_remote.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
    "context"
    "os/exec"
    "regexp"
    "strings"

    "github.com/bborbe/errors"
)

// BitbucketRemoteCoords holds the project key and repo slug parsed from a Bitbucket Server remote URL.
type BitbucketRemoteCoords struct {
    // Project is the Bitbucket project key (uppercased, e.g. "BRO").
    Project string
    // Repo is the repository slug (lowercased, e.g. "sentinel").
    Repo string
}

// ParseBitbucketRemoteURL parses a Bitbucket Server git remote URL and returns project key and repo slug.
//
// Supported formats:
//   - SSH:   ssh://bitbucket.example.com:7999/bro/sentinel.git
//   - HTTPS: https://bitbucket.example.com/scm/bro/sentinel.git
//
// The project key is uppercased and the repo slug is lowercased (matching Bitbucket Server conventions).
func ParseBitbucketRemoteURL(ctx context.Context, remoteURL string) (*BitbucketRemoteCoords, error) {
    // SSH format: ssh://host:port/project/repo.git
    sshRe := regexp.MustCompile(`^ssh://[^/]+/([^/]+)/([^/]+?)(?:\.git)?$`)
    if m := sshRe.FindStringSubmatch(remoteURL); m != nil {
        return &BitbucketRemoteCoords{
            Project: strings.ToUpper(m[1]),
            Repo:    strings.ToLower(strings.TrimSuffix(m[2], ".git")),
        }, nil
    }

    // HTTPS format: https://host/scm/project/repo.git
    httpsRe := regexp.MustCompile(`^https?://[^/]+/scm/([^/]+)/([^/]+?)(?:\.git)?$`)
    if m := httpsRe.FindStringSubmatch(remoteURL); m != nil {
        return &BitbucketRemoteCoords{
            Project: strings.ToUpper(m[1]),
            Repo:    strings.ToLower(strings.TrimSuffix(m[2], ".git")),
        }, nil
    }

    return nil, errors.Errorf(ctx, "unrecognized Bitbucket Server remote URL format: %q (expected ssh://host:port/project/repo.git or https://host/scm/project/repo.git)", remoteURL)
}

// ParseBitbucketRemoteFromGit reads the git remote URL for the given remote name and parses it.
// Uses `git remote get-url <remoteName>` to fetch the URL.
func ParseBitbucketRemoteFromGit(ctx context.Context, remoteName string) (*BitbucketRemoteCoords, error) {
    // #nosec G204 -- remoteName is a fixed string ("origin"), not user input
    cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remoteName)
    output, err := cmd.Output()
    if err != nil {
        return nil, errors.Wrap(ctx, err, "get git remote url")
    }
    remoteURL := strings.TrimSpace(string(output))
    return ParseBitbucketRemoteURL(ctx, remoteURL)
}
```

**Step 4: Tests for config validation**

Create `pkg/config/provider_test.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/bborbe/dark-factory/pkg/config"
)

var _ = Describe("Provider", func() {
    var ctx context.Context
    BeforeEach(func() {
        ctx = context.Background()
    })

    DescribeTable("Validate",
        func(provider config.Provider, expectErr bool) {
            err := provider.Validate(ctx)
            if expectErr {
                Expect(err).To(HaveOccurred())
            } else {
                Expect(err).NotTo(HaveOccurred())
            }
        },
        Entry("github is valid", config.ProviderGitHub, false),
        Entry("bitbucket-server is valid", config.ProviderBitbucketServer, false),
        Entry("empty string is invalid", config.Provider(""), true),
        Entry("invalid value is rejected", config.Provider("invalid"), true),
        Entry("gitlab is not supported", config.Provider("gitlab"), true),
    )
})
```

Add tests for the new config validation rules to `pkg/config/config_test.go` (or create it if it doesn't exist):

```go
var _ = Describe("Config.Validate", func() {
    // existing tests...

    Context("provider field", func() {
        It("accepts github provider", func() {
            cfg := config.Defaults()
            cfg.Provider = config.ProviderGitHub
            Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
        })

        It("accepts bitbucket-server provider with baseURL", func() {
            cfg := config.Defaults()
            cfg.Provider = config.ProviderBitbucketServer
            cfg.Bitbucket.BaseURL = "https://bitbucket.example.com"
            cfg.Bitbucket.TokenEnv = "BITBUCKET_TOKEN"
            Expect(cfg.Validate(ctx)).NotTo(HaveOccurred())
        })

        It("rejects bitbucket-server without baseURL", func() {
            cfg := config.Defaults()
            cfg.Provider = config.ProviderBitbucketServer
            cfg.Bitbucket.BaseURL = ""
            Expect(cfg.Validate(ctx)).To(HaveOccurred())
        })

        It("rejects invalid provider", func() {
            cfg := config.Defaults()
            cfg.Provider = config.Provider("invalid")
            Expect(cfg.Validate(ctx)).To(HaveOccurred())
        })

        It("default provider is github", func() {
            cfg := config.Defaults()
            Expect(cfg.Provider).To(Equal(config.ProviderGitHub))
        })
    })
})
```

If `pkg/config/config_test.go` already has validation tests, append the `Context("provider field", ...)` block to the existing `Describe`.

**Step 5: Tests for Bitbucket remote URL parser**

Create `pkg/git/bitbucket_remote_test.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("ParseBitbucketRemoteURL", func() {
    var ctx context.Context
    BeforeEach(func() {
        ctx = context.Background()
    })

    DescribeTable("SSH format",
        func(url, expectedProject, expectedRepo string) {
            coords, err := git.ParseBitbucketRemoteURL(ctx, url)
            Expect(err).NotTo(HaveOccurred())
            Expect(coords.Project).To(Equal(expectedProject))
            Expect(coords.Repo).To(Equal(expectedRepo))
        },
        Entry("lowercase project and repo", "ssh://bitbucket.example.com:7999/bro/sentinel.git", "BRO", "sentinel"),
        Entry("uppercase project key", "ssh://bitbucket.example.com:7999/BRO/sentinel.git", "BRO", "sentinel"),
        Entry("without .git suffix", "ssh://bitbucket.example.com:7999/bro/sentinel", "BRO", "sentinel"),
        Entry("mixed case repo slug is lowercased", "ssh://bitbucket.example.com:7999/bro/MyRepo.git", "BRO", "myrepo"),
    )

    DescribeTable("HTTPS format",
        func(url, expectedProject, expectedRepo string) {
            coords, err := git.ParseBitbucketRemoteURL(ctx, url)
            Expect(err).NotTo(HaveOccurred())
            Expect(coords.Project).To(Equal(expectedProject))
            Expect(coords.Repo).To(Equal(expectedRepo))
        },
        Entry("standard HTTPS with /scm/ path", "https://bitbucket.example.com/scm/bro/sentinel.git", "BRO", "sentinel"),
        Entry("http (not https) also accepted", "http://bitbucket.example.com/scm/bro/sentinel.git", "BRO", "sentinel"),
        Entry("without .git suffix", "https://bitbucket.example.com/scm/bro/sentinel", "BRO", "sentinel"),
    )

    DescribeTable("invalid formats return error",
        func(url string) {
            _, err := git.ParseBitbucketRemoteURL(ctx, url)
            Expect(err).To(HaveOccurred())
        },
        Entry("github SSH URL", "git@github.com:owner/repo.git"),
        Entry("github HTTPS URL", "https://github.com/owner/repo.git"),
        Entry("empty string", ""),
        Entry("plain host only", "ssh://bitbucket.example.com:7999"),
        Entry("https without /scm/ segment", "https://bitbucket.example.com/bro/sentinel.git"),
    )
})
```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change existing `GitHubConfig`, `ResolvedGitHubToken()`, or any existing GitHub behavior
- `provider` field defaults to `github` in `Defaults()` so existing configs without a `provider` field continue to work (YAML zero-value of `Provider` is `""`, which will fail validation — set default to `ProviderGitHub` in `Defaults()`)
- `ParseBitbucketRemoteURL` is a pure function (no exec calls) — test it without mocks
- `ParseBitbucketRemoteFromGit` uses exec — it will be tested indirectly through integration; unit tests cover `ParseBitbucketRemoteURL` only
- Follow existing error wrapping: `errors.Wrap(ctx, err, "message")`
- All existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# Provider enum exists
grep -n "ProviderGitHub\|ProviderBitbucketServer" pkg/config/provider.go

# BitbucketConfig in Config struct
grep -n "Bitbucket\|Provider" pkg/config/config.go | head -20

# validateBitbucketConfig exists
grep -n "validateBitbucketConfig" pkg/config/config.go

# ResolvedBitbucketToken exists
grep -n "ResolvedBitbucketToken" pkg/config/config.go

# URL parser exists
grep -n "ParseBitbucketRemoteURL" pkg/git/bitbucket_remote.go

make precommit
```
Must pass with no errors.
</verification>
