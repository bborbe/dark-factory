---
status: completed
summary: 'Verified HOST_CACHE_DIR auto-default: both /home/node/.cache/go-build and /home/node/.cache/golangci-lint are bind-mounted and non-empty; wrote RESULT: PASS to tmp/host-cache-dir-verify.txt'
container: dark-factory-286-verify-host-cache-dir
dark-factory-version: v0.107.6
created: "2026-04-09T17:30:00Z"
queued: "2026-04-09T16:41:08Z"
started: "2026-04-09T16:41:08Z"
completed: "2026-04-09T16:41:48Z"
---
<summary>
- Verifies HOST_CACHE_DIR auto-default works inside dark-factory containers
- Confirms /home/node/.cache/go-build is a real bind mount from host (not empty)
- Confirms /home/node/.cache/golangci-lint is a real bind mount from host
- Reports mount sources via /proc/self/mountinfo
</summary>

<objective>
Verify the HOST_CACHE_DIR auto-default feature (prompt 285) works end-to-end by inspecting the running container's mounts and confirming both Go build cache and golangci-lint cache are correctly bind-mounted from the host.
</objective>

<context>
Read CLAUDE.md.
The `.dark-factory.yaml` at the project root declares two extraMounts using `${HOST_CACHE_DIR}`. Without prompt 285's resolver, these would expand to literal `/go-build` and `/golangci-lint` (broken). With the resolver, they expand to host cache dirs (macOS: `$HOME/Library/Caches`, Linux: `$XDG_CACHE_HOME` or `$HOME/.cache`).
</context>

<requirements>

## 1. Create verification report

Create `tmp/host-cache-dir-verify.txt` containing:

1. Output of `mount | grep -E '/home/node/\.cache/(go-build|golangci-lint)'` — must show both bind mounts.
2. Output of `ls -la /home/node/.cache/go-build | head -5` — must list real cache contents (not empty).
3. Output of `ls -la /home/node/.cache/golangci-lint | head -5`.
4. A line `RESULT: PASS` if both mounts exist AND both directories are non-empty, else `RESULT: FAIL`.

## 2. Fail loudly on broken mount

If either `/home/node/.cache/go-build` or `/home/node/.cache/golangci-lint` is missing OR empty, exit with non-zero status after writing the report so dark-factory marks the prompt failed.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT modify any source code, configs, or .dark-factory.yaml
- Only writes are: `tmp/host-cache-dir-verify.txt` (create the `tmp/` dir if needed)
- Read-only inspection of /home/node/.cache/*
</constraints>

<verification>
`cat tmp/host-cache-dir-verify.txt` shows both mounts and `RESULT: PASS`.
</verification>
