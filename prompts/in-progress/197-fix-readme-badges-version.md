---
status: approved
created: "2026-03-11T20:22:56Z"
queued: "2026-03-11T20:22:56Z"
---

<summary>
- README gets standard Go project badges (CI, Go Reference, Go Report Card)
- Go version prerequisite updated to match go.mod
- Container image version updated to match pkg/const.go default
- No functional code changes
</summary>

<objective>
Add standard badges to README.md and fix version discrepancies.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `README.md` — the current file has no badges and says "Go 1.24+" in prerequisites.
Read `go.mod` — check the actual Go version requirement.
Read `pkg/const.go` — check the `DefaultContainerImage` constant for the current image version.
Read `.github/workflows/ci.yml` — confirm the CI workflow filename for the badge URL.
</context>

<requirements>
1. Add badges after the `# Dark Factory` title line, before the description:
   ```
   [![CI](https://github.com/bborbe/dark-factory/actions/workflows/ci.yml/badge.svg)](https://github.com/bborbe/dark-factory/actions/workflows/ci.yml)
   [![Go Reference](https://pkg.go.dev/badge/github.com/bborbe/dark-factory.svg)](https://pkg.go.dev/github.com/bborbe/dark-factory)
   [![Go Report Card](https://goreportcard.com/badge/github.com/bborbe/dark-factory)](https://goreportcard.com/report/github.com/bborbe/dark-factory)
   ```
2. Update the Go version in Prerequisites to match `go.mod` (e.g., "Go 1.26+" if go.mod says 1.26.1)
3. Update the container image version in Prerequisites and Configuration sections to match `DefaultContainerImage` in `pkg/const.go`
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Only modify README.md
- Check actual values in go.mod and pkg/const.go — do not hardcode
</constraints>

<verification>
Run `grep -c "badge" README.md` — should return 3 or more.
Run `grep "Go 1\." README.md` — should show version matching go.mod.
</verification>
