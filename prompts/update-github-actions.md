---
status: created
---

<objective>
Update `actions/checkout` from `@v4` to `@v6` in CI workflows and align CI trigger pattern.
</objective>

<context>
Read `.github/workflows/ci.yml` — uses `actions/checkout@v4`.
Read `.github/workflows/claude-code-review.yml` — uses `actions/checkout@v4`.
Read `.github/workflows/claude.yml` — reference for current patterns.
</context>

<requirements>
1. In `.github/workflows/ci.yml`:
   - Replace `actions/checkout@v4` with `actions/checkout@v6`

2. In `.github/workflows/claude-code-review.yml`:
   - Replace `actions/checkout@v4` with `actions/checkout@v6`

3. In `.github/workflows/claude.yml`:
   - If it uses `actions/checkout@v4`, update to `@v6`
   - If already `@v6`, no change needed
</requirements>

<constraints>
- Only change the checkout action version — do not modify triggers, jobs, or steps
- Do NOT commit — dark-factory handles git
- `make precommit` must pass (if applicable — workflow files don't affect Go build)
</constraints>

<verification>
Verify all workflows use v6:
```bash
grep -r "actions/checkout@" .github/workflows/
# Expected: all show @v6
```

Validate YAML syntax:
```bash
for f in .github/workflows/*.yml; do python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "$f: OK"; done
```
</verification>

<success_criteria>
- All `actions/checkout` references are `@v6`
- YAML syntax valid
- No other changes to workflow files
</success_criteria>
