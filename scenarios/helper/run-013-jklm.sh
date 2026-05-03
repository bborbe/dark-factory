#!/usr/bin/env bash
#
# Spec 062 sub-scenarios J/K/L/M for scenarios/013-config-layering.md.
# Verifies --set support for workflow, pr, autoMerge.
#
# Usage:
#   scenarios/helper/run-013-jklm.sh
#
# Exit code: 0 if all assertions pass, 1 if any fail.

set -uo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=lib.sh
source "$HERE/lib.sh"

build_binary
scenario_setup

echo
echo "── Scenario J: --set workflow=branch --set pr=true overrides project ──"
scenario_run --set workflow=branch --set pr=true > j.log 2>&1 || true
assert_contains j.log "workflow=branch workflowSource=arg" "J: workflow=branch from arg"
assert_contains j.log "pr=true prSource=arg"               "J: pr=true from arg"

echo
echo "── Scenario K: --set autoMerge=true on pr=true project ──"
reset_yaml_workflow_pr
{ echo "workflow: branch"; echo "pr: true"; } >> .dark-factory.yaml
scenario_run --set autoMerge=true > k.log 2>&1 || true
assert_contains     k.log "autoMerge=true autoMergeSource=arg" "K: autoMerge=true from arg"
assert_not_contains k.log "autoMergeSource=project"            "K: source NOT project"

echo
echo "── Scenario L: --set workflow=direct --set pr=true rejected ──"
reset_yaml_workflow_pr
scenario_run --set workflow=direct --set pr=true > l.log 2>&1
assert_exit_nonzero $? "L: validator rejects"
assert_contains    l.log "incompatible" "L: incompatible error message"

echo
echo "── Scenario M: --set autoMerge=true without pr rejected ──"
reset_yaml_workflow_pr
scenario_run --set autoMerge=true > m.log 2>&1
assert_exit_nonzero $? "M: validator rejects"
assert_contains    m.log "autoMerge requires pr: true" "M: autoMerge gate error"

echo
echo "── Bonus: bad input rejection ──"
scenario_run --set workflow=invalid > bad-enum.log 2>&1
assert_exit_nonzero $? "invalid enum exits non-zero"
assert_contains    bad-enum.log "unknown workflow" "invalid enum error message"

scenario_run --set workflow=pr > legacy.log 2>&1
assert_exit_nonzero $? "legacy workflow=pr exits non-zero"
assert_contains    legacy.log "legacy workflow value" "legacy enum rejection"

scenario_run --set pr=yes > bad-bool.log 2>&1
assert_exit_nonzero $? "pr=yes exits non-zero"
assert_contains    bad-bool.log "invalid bool" "bool format error"

echo
echo "── Bonus: last-wins + composition ──"
scenario_run --set workflow=branch --set workflow=clone > lw.log 2>&1 || true
assert_contains lw.log "workflow=clone workflowSource=arg" "last-wins: clone wins over branch"

scenario_run --set workflow=clone --set pr=true --set autoMerge=true > all3.log 2>&1 || true
assert_contains all3.log "workflow=clone workflowSource=arg"   "compose: workflow"
assert_contains all3.log "pr=true prSource=arg"                "compose: pr"
assert_contains all3.log "autoMerge=true autoMergeSource=arg"  "compose: autoMerge"

scenario_done
