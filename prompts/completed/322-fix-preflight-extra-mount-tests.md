---
status: completed
summary: Updated two buildPreflightDockerArgs extra-mount tests to use GinkgoT().TempDir() as mount source so os.Stat succeeds and mounts are emitted in args.
container: dark-factory-322-fix-preflight-extra-mount-tests
dark-factory-version: v0.125.1
created: "2026-04-19T00:00:00Z"
queued: "2026-04-19T17:27:40Z"
started: "2026-04-19T17:29:05Z"
completed: "2026-04-19T17:31:17Z"
---

<summary>
- Fixes 2 failing tests in the `buildPreflightDockerArgs` suite that assert extra-mount behavior
- Tests fail today because they use a hardcoded container path (`/workspace`) as the mount source; the implementation stats the source on the host and skips missing paths, so the mount never appears in the generated args
- Switches the two failing tests to use a real host directory via `GinkgoT().TempDir()` so `os.Stat` succeeds and the mount is actually emitted
- Updates only the affected assertions to reference the tempdir path instead of the literal `/workspace` string
- The existing "skips extra mount when src does not exist" test is deliberately left untouched — it relies on a nonexistent path
- Pure test fix — no production code changes
</summary>

<objective>
Make the two `buildPreflightDockerArgs` extra-mount tests (read-write and read-only) pass by using an existing host directory as the mount source, so the `os.Stat` check inside `BuildPreflightDockerArgs` succeeds and the expected `-v src:dst[:ro]` mount argument is appended.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/prompt-writing.md` for prompt conventions.

Key files:
- `pkg/preflight/preflight_test.go` — the only file to modify. Target `Describe("buildPreflightDockerArgs", ...)` starting around line 123. The two failing `It` blocks are "appends read-write extra mount when src exists" (~line 152) and "appends read-only extra mount when ReadOnly is true" (~line 168).
- `pkg/preflight/preflight.go` — DO NOT modify. `BuildPreflightDockerArgs` (~line 190) calls `os.Stat(src)` on each extra mount; if stat fails, it logs a warning and `continue`s, skipping the mount. That behavior is correct and covered by the "skips extra mount when src does not exist" test at line 183.

Existing precedent for `GinkgoT().TempDir()` in this repo (use the same pattern):
- `pkg/globalconfig/globalconfig_internal_test.go:119` — `tmpDir = GinkgoT().TempDir()`
- `pkg/processor/processor_internal_test.go:1814, 1913, 1991, 2024, 2096, 2188, 2326, 2464, 2620, 2649, 2673` — all use `GinkgoT().TempDir()` directly inside `It` blocks or `BeforeEach`.

Why the tests currently fail:
- Both tests pass `Src: "/workspace"` as the extra-mount source.
- `BuildPreflightDockerArgs` runs `os.Stat("/workspace")` on the host (not inside any container). On the test host `/workspace` does not exist, so stat returns an error, the implementation logs "preflight: extraMounts src does not exist, skipping" and the `-v /workspace:/host` argument is never appended.
- Assertions `Expect(args).To(ContainElement("/workspace:/host"))` and `Expect(args).To(ContainElement("/workspace:/host:ro"))` therefore fail.
</context>

<requirements>

## 1. Modify only `pkg/preflight/preflight_test.go`

Do not touch any other file. In particular, do NOT modify `pkg/preflight/preflight.go`.

## 2. Update the `It("appends read-write extra mount when src exists", ...)` block (~line 152)

Replace the current body so it uses a host tempdir as the mount source. The assertion must match the tempdir-based mount string.

```go
// Before:
It("appends read-write extra mount when src exists", func() {
    ro := false
    mounts := []config.ExtraMount{{Src: "/workspace", Dst: "/host", ReadOnly: &ro}}
    args := preflight.BuildPreflightDockerArgs(
        projectRoot,
        containerImage,
        command,
        mounts,
        home,
        lookup,
        goos,
    )
    Expect(args).To(ContainElement("/workspace:/host"))
    Expect(args).NotTo(ContainElement("/workspace:/host:ro"))
})

// After:
It("appends read-write extra mount when src exists", func() {
    tempDir := GinkgoT().TempDir()
    ro := false
    mounts := []config.ExtraMount{{Src: tempDir, Dst: "/host", ReadOnly: &ro}}
    args := preflight.BuildPreflightDockerArgs(
        projectRoot,
        containerImage,
        command,
        mounts,
        home,
        lookup,
        goos,
    )
    Expect(args).To(ContainElement(tempDir + ":/host"))
    Expect(args).NotTo(ContainElement(tempDir + ":/host:ro"))
})
```

## 3. Update the `It("appends read-only extra mount when ReadOnly is true", ...)` block (~line 168)

Same substitution — tempdir as Src, assertion uses tempdir.

```go
// Before:
It("appends read-only extra mount when ReadOnly is true", func() {
    ro := true
    mounts := []config.ExtraMount{{Src: "/workspace", Dst: "/host", ReadOnly: &ro}}
    args := preflight.BuildPreflightDockerArgs(
        projectRoot,
        containerImage,
        command,
        mounts,
        home,
        lookup,
        goos,
    )
    Expect(args).To(ContainElement("/workspace:/host:ro"))
})

// After:
It("appends read-only extra mount when ReadOnly is true", func() {
    tempDir := GinkgoT().TempDir()
    ro := true
    mounts := []config.ExtraMount{{Src: tempDir, Dst: "/host", ReadOnly: &ro}}
    args := preflight.BuildPreflightDockerArgs(
        projectRoot,
        containerImage,
        command,
        mounts,
        home,
        lookup,
        goos,
    )
    Expect(args).To(ContainElement(tempDir + ":/host:ro"))
})
```

## 4. Leave all other tests in the file untouched

Specifically:
- `It("produces correct base args without extra mounts", ...)` at line 133 — do NOT change.
- `It("skips extra mount when src does not exist", ...)` at line 183 — do NOT change. This test intentionally uses `/nonexistent/path/abc123` to verify the skip-on-missing behavior.
- `It("expands tilde in extra mount src", ...)` at line 198 — do NOT change.
- Everything outside `Describe("buildPreflightDockerArgs", ...)` — do NOT change.

## 5. Imports

`GinkgoT()` is already available because the file already imports `. "github.com/onsi/ginkgo/v2"` (see line 12). Do NOT add new imports. Do NOT introduce `os`, `path/filepath`, `ioutil`, or anything else — `GinkgoT().TempDir()` alone is sufficient and auto-cleans up.

## 6. Do not change the shared `Describe` closure variables

The `var (...)` block at lines 124-131 defines `projectRoot`, `containerImage`, `command`, `home`, `lookup`, `goos`. Leave all of those as-is. In particular, keep `projectRoot = "/workspace"` — only the extra-mount `Src` changes.

## 7. Run the full precommit check

```bash
make precommit
```

All tests in the repo — including the three other `buildPreflightDockerArgs` tests — must pass. The two previously failing tests must now pass.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT modify `pkg/preflight/preflight.go` — the stat-and-skip behavior is correct and is tested by the "skips extra mount when src does not exist" case
- Do NOT modify any test other than the two named `It` blocks
- Do NOT add new imports, new helpers, or a `BeforeEach` — inline `GinkgoT().TempDir()` inside each `It` is what the rest of the repo does (see `pkg/processor/processor_internal_test.go`)
- Do NOT change `projectRoot = "/workspace"` in the shared `var (...)` block — the base-args assertion at line 143 depends on it
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Keep the diff minimal: exactly two `It` blocks change, nothing else
</constraints>

<verification>
Run `make precommit` — must pass cleanly end-to-end.

Targeted spot checks:

```bash
# Run only the preflight package tests
go test ./pkg/preflight/...

# Run only the buildPreflightDockerArgs describe block
go test -v -run 'TestPreflight' ./pkg/preflight/... 2>&1 | grep -E 'buildPreflightDockerArgs|PASS|FAIL'
```

Expected: all five `It` blocks under `buildPreflightDockerArgs` pass, including the two that previously failed. The "skips extra mount when src does not exist" and "expands tilde in extra mount src" tests still pass unchanged.
</verification>
