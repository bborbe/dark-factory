---
status: idea
tags:
  - dark-factory
  - spec
---

## Summary

- Add optional `maxContainers` field to per-project `.dark-factory.yaml`
- Overrides the global `~/.dark-factory/config.yaml` limit for this project
- Lower values restrict the project to fewer slots (low-priority repos)
- Higher values allow priority projects to run more containers than the global default

## Problem

The global `maxContainers` limit applies uniformly to all projects. When running 40 repos, some are high-priority (trading strategies, active features) and some are low-priority (dependency updates, maintenance). There's no way to say "this project can use 5 slots" or "this project should never use more than 1 slot."

## Goal

After this work, per-project `.dark-factory.yaml` can override the global container limit. Priority projects get more slots, background projects get fewer. The global limit remains the default when no per-project value is set.

## Do-Nothing Option

All projects share the global limit equally. A low-priority dependency update project can starve a high-priority feature project. Users must manually stop/start daemons to prioritize.
