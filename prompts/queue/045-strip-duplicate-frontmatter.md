---
status: queued
---

# Strip duplicate/empty frontmatter from prompt content

When the processor reads a prompt file, it may contain a duplicate or empty frontmatter block in the content body. This happens when a prompt was created with empty frontmatter (`---\n---`) and the processor later prepended its own frontmatter — leaving the original empty block as content.

Example of broken file:
```
---
status: queued
---



---
---

# Actual prompt title
```

The `---\n---` in the content body gets passed to the YOLO container as the prompt, which interprets the leading `---` as a CLI flag and fails with `error: unknown option '---'`.

## Implementation

In `pkg/prompt/prompt.go`, in the `Content` function (which extracts the prompt body after frontmatter):
1. After splitting off the frontmatter, trim the remaining content
2. If the content starts with `---\n---\n` (an empty frontmatter block), strip it
3. More generally: if content starts with `---\n` followed by lines until another `---\n` where all lines between are empty or whitespace, strip that block

A simple approach:
```go
// After extracting content (everything after first frontmatter block):
// Strip any additional empty frontmatter blocks at the start
content = strings.TrimLeft(content, "\n")
if strings.HasPrefix(content, "---\n---") || strings.HasPrefix(content, "---\r\n---") {
    // Find end of this empty frontmatter block and skip it
    idx := strings.Index(content[3:], "---")
    if idx >= 0 {
        remaining := content[3+idx+3:]
        content = strings.TrimLeft(remaining, "\n\r")
    }
}
```

Also add a test case for this scenario.

## Verification

Run `make precommit` — must pass.
