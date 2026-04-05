---
status: completed
summary: Flipped ExtraMount default from read-only to read-write, renamed field from Readonly to ReadOnly with new YAML tag readOnly, and updated all tests and documentation accordingly.
container: dark-factory-253-extramount-default-readwrite
dark-factory-version: v0.95.0
created: "2026-04-05T00:00:00Z"
queued: "2026-04-05T20:34:04Z"
started: "2026-04-05T20:34:06Z"
completed: "2026-04-05T20:41:49Z"
---

<summary>
- Extra mounts default to read-write instead of read-only when no explicit flag is set
- The mount access mode field is renamed for Go naming consistency, with a camelCase YAML tag
- Mounts without an explicit access mode setting default to read-write
- Existing configs with an explicit setting continue to work via the new YAML tag
- All tests and documentation are updated to reflect the new default and field name
</summary>

<objective>
Flip the ExtraMount default from read-only to read-write. Rename the field from Readonly to ReadOnly for Go naming consistency and update the YAML tag from `readonly` to `readOnly` for camelCase consistency. All tests must be updated to match.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making any changes:
- `pkg/config/config.go` — `ExtraMount` struct (around line 67), `IsReadonly` method (around line 74)
- `pkg/config/config_test.go` — ExtraMount tests: YAML loading test (around line 1439), `ExtraMount.IsReadonly` tests (around line 1799)
- `pkg/executor/executor_internal_test.go` — extra mount docker command tests (around line 547)
- `pkg/executor/executor.go` — `buildDockerCommand` method where `IsReadonly()` is called (around line 427)
</context>

<requirements>
1. **Rename field in `pkg/config/config.go`** in the `ExtraMount` struct:

Old:
```go
Readonly *bool  `yaml:"readonly,omitempty"` // nil defaults to true
```

New:
```go
ReadOnly *bool  `yaml:"readOnly,omitempty"` // nil defaults to false (read-write)
```

2. **Flip default in `IsReadonly` method** in `pkg/config/config.go`:

Old:
```go
// IsReadonly returns true if the mount is read-only (default when Readonly is nil).
func (m ExtraMount) IsReadonly() bool {
	if m.Readonly == nil {
		return true
	}
	return *m.Readonly
}
```

New:
```go
// IsReadonly returns true if the mount is read-only (default when ReadOnly is nil is false = read-write).
func (m ExtraMount) IsReadonly() bool {
	if m.ReadOnly == nil {
		return false
	}
	return *m.ReadOnly
}
```

3. **Update tests in `pkg/config/config_test.go`** — YAML loading test (the `"loads extraMounts from config file"` It block):

Change the YAML content from `readonly: false` to `readOnly: true` (to test explicit read-only), and update assertions:
- The first mount (no readOnly field) should now assert `IsReadonly()` is `BeFalse()` (was `BeTrue()`)
- The second mount (with `readOnly: true`) should assert `IsReadonly()` is `BeTrue()`

Old YAML in test:
```yaml
extraMounts:
  - src: /some/host/path
    dst: /container/path
  - src: ~/docs
    dst: /docs
    readonly: false
```

New YAML in test:
```yaml
extraMounts:
  - src: /some/host/path
    dst: /container/path
  - src: ~/docs
    dst: /docs
    readOnly: true
```

Old assertions:
```go
Expect(cfg.ExtraMounts[0].IsReadonly()).To(BeTrue())
Expect(cfg.ExtraMounts[1].IsReadonly()).To(BeFalse())
```

New assertions:
```go
Expect(cfg.ExtraMounts[0].IsReadonly()).To(BeFalse())
Expect(cfg.ExtraMounts[1].IsReadonly()).To(BeTrue())
```

4. **Update `ExtraMount.IsReadonly` unit tests** in `pkg/config/config_test.go` (the `"ExtraMount.IsReadonly"` Describe block):

- `"returns true when Readonly is nil (default)"` — rename to `"returns false when ReadOnly is nil (default)"`, change assertion from `BeTrue()` to `BeFalse()`
- `"returns true when Readonly is explicitly true"` — rename to `"returns true when ReadOnly is explicitly true"`, change `Readonly:` to `ReadOnly:` in struct literal
- `"returns false when Readonly is false"` — rename to `"returns false when ReadOnly is explicitly false"`, change `Readonly:` to `ReadOnly:` in struct literal

5. **Update executor tests in `pkg/executor/executor_internal_test.go`**:

- The `"adds extra mount with :ro suffix when IsReadonly is true (nil Readonly)"` test: Since nil now means read-write (not read-only), this test's premise is wrong. Rename to `"adds extra mount without :ro suffix when ReadOnly is nil (default read-write)"` and change assertion from `ContainElement(srcDir + ":/docs:ro")` to `ContainElement(srcDir + ":/docs")`. Also add a negative check that `:ro` is NOT present (same pattern as the existing false test).

- The `"adds extra mount without :ro suffix when Readonly is false"` test: Rename to `"adds extra mount with :ro suffix when ReadOnly is explicitly true"`. Change the boolean from `f := false` to `t := true`, change struct field from `Readonly: &f` to `ReadOnly: &t`, and change the assertion to expect `ContainElement(srcDir + ":/docs:ro")`.

- All other tests in that file that reference `Readonly:` in ExtraMount struct literals: change field name to `ReadOnly:`.

6. **Update `pkg/executor/executor.go`** — no logic change needed here since `IsReadonly()` method is used (not the field directly), but verify this is the case. The method name `IsReadonly` stays the same.

7. **Search the entire codebase** for any remaining references to the old field name `Readonly` (with lowercase 'o') on ExtraMount and the old YAML tag `readonly`. Update any found references. Use: `grep -rn 'Readonly\|"readonly"' .` (search full repo, not just `pkg/`)

8. **Update `docs/configuration.md`** — the Extra Mounts section documents the old YAML tag `readonly` and old default `true`. Update:
   - Change YAML tag from `readonly` to `readOnly` in the table and examples
   - Change default from `true` to `false` (read-write)
   - Update the field description to reflect that mounts are read-write by default
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass after all changes
- The method name `IsReadonly()` stays unchanged — only the field name and default change
- YAML tag changes from `readonly` to `readOnly` — this is a breaking change for existing `.dark-factory.yaml` files using the old tag, which is intentional
- Use `errors.Wrap(ctx, err, "message")` — never `fmt.Errorf`
</constraints>

<verification>
```bash
# Confirm field renamed
grep -n 'ReadOnly \*bool' pkg/config/config.go

# Confirm new YAML tag
grep -n 'yaml:"readOnly' pkg/config/config.go

# Confirm default flipped
grep -A3 'func (m ExtraMount) IsReadonly' pkg/config/config.go

# Confirm no old references remain
grep -rn '\.Readonly' --include='*.go' pkg/ | grep -v ReadOnly && echo "STALE REFERENCES FOUND" || echo "OK"

# Confirm docs updated
grep -n 'readOnly' docs/configuration.md

# Run full precommit
make precommit
```
All must pass with no errors.
</verification>
