---
status: completed
container: dark-factory-215-test-resume-verification
dark-factory-version: v0.63.0
created: "2026-03-21T20:15:29Z"
queued: "2026-03-21T20:15:29Z"
started: "2026-03-21T20:15:30Z"
completed: "2026-03-21T20:19:00Z"
---

<summary>
- Test prompt to verify resume-on-restart works
</summary>

<objective>
Sleep for 120 seconds then create a file `test-resume-marker.txt` in the repo root with content "completed". This prompt exists to test that dark-factory can resume monitoring after a restart.
</objective>

<context>
Read nothing — this is a test prompt.
</context>

<requirements>
1. Run `sleep 120` to simulate a long-running task.
2. After sleeping, create a file `test-resume-marker.txt` with content "completed".
</requirements>

<constraints>
- Do NOT commit anything.
</constraints>

<verification>
The file `test-resume-marker.txt` should exist with content "completed" after the prompt finishes.
</verification>
