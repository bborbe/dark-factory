---
status: completed
container: dark-factory-043-fix-runner-no-inbox-normalize
dark-factory-version: v0.10.2
---





# Runner should not normalize inbox directory

## Bug

`pkg/runner/runner.go:117-126` — `normalizeFilenames()` calls `NormalizeFilenames` on the inbox directory. This renames user draft files in the inbox (e.g. `reset-failed-on-startup.md` → `041-reset-failed-on-startup.md`).

The inbox is a passive drop zone — files there are drafts. Only the queue directory should be normalized.

## Fix

In `pkg/runner/runner.go`, remove the inbox normalization from `normalizeFilenames()`. Only normalize the queue directory:

```go
func (r *runner) normalizeFilenames(ctx context.Context) error {
	renames, err := r.promptManager.NormalizeFilenames(ctx, r.queueDir)
	if err != nil {
		return errors.Wrap(ctx, err, "normalize queue filenames")
	}
	for _, rename := range renames {
		log.Printf("dark-factory: renamed %s -> %s",
			filepath.Base(rename.OldPath), filepath.Base(rename.NewPath))
	}
	return nil
}
```

Remove the `r.inboxDir` field from the runner struct if it's no longer used anywhere. Check all usages first.

Update the runner test to verify only queue dir is normalized (not inbox).

## Verification

Run `make precommit` — must pass.
