#!/usr/bin/env bash
#
# Full runner for scenarios/013-config-layering.md (sub-scenarios A through M).
# Verifies the config layering feature (specs 060, 061, 062): default ← global ← project ← arg.
#
# Usage:
#   scenarios/helper/run-013-all.sh
#
# Exit code: 0 if all assertions pass, 1 if any fail.
#
# Isolates global config by exporting HOME=$WORK_DIR/home — the user's real
# ~/.dark-factory/config.yaml is never touched.

set -uo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=lib.sh
source "$HERE/lib.sh"

build_binary
scenario_setup $'pr: false\nworktree: false\nmaxContainers: 999\n'

# Baseline global config (matches scenario 013 Setup section).
write_global_config $'model: claude-opus-4-7\nhideGit: true\n'

echo
echo "── Scenario A: global model applies when project is silent ──"
scenario_run > a.log 2>&1 || true
assert_contains a.log "model=claude-opus-4-7"  "A: model from global"
assert_contains a.log "modelSource=global"     "A: modelSource=global"
assert_contains a.log "hideGit=true"           "A: hideGit from global"
assert_contains a.log "hideGitSource=global"   "A: hideGitSource=global"

echo
echo "── Scenario B: project model beats global ──"
echo "model: claude-sonnet-4-6" >> .dark-factory.yaml
scenario_run > b.log 2>&1 || true
assert_contains b.log "model=claude-sonnet-4-6" "B: project model wins"
assert_contains b.log "modelSource=project"    "B: modelSource=project"
assert_contains b.log "hideGitSource=global"   "B: hideGit still global"
reset_yaml_field model

echo
echo "── Scenario C: --model arg beats both ──"
scenario_run --model claude-haiku-4-5 > c.log 2>&1 || true
assert_contains c.log "model=claude-haiku-4-5" "C: arg model wins"
assert_contains c.log "modelSource=arg"        "C: modelSource=arg"

echo
echo "── Scenario D: --set hideGit=false beats global hideGit=true ──"
scenario_run --set hideGit=false > d.log 2>&1 || true
assert_contains d.log "hideGit=false"        "D: arg hideGit wins"
assert_contains d.log "hideGitSource=arg"    "D: hideGitSource=arg"

echo
echo "── Scenario E: invalid global config fails startup ──"
write_global_config $'dirtyFileThreshold: -5\n'
scenario_run > e.log 2>&1
assert_exit_nonzero $? "E: invalid global rejected"
assert_contains    e.log "dirtyFileThreshold" "E: error names field"
assert_contains    e.log "globalconfig"        "E: error names file context"
write_global_config $'model: claude-opus-4-7\nhideGit: true\n'

echo
echo "── Scenario F: --set dirtyFileThreshold int override ──"
scenario_run --set dirtyFileThreshold=10 > f.log 2>&1 || true
assert_contains f.log "dirtyFileThreshold=10"           "F: arg int override"
assert_contains f.log "dirtyFileThresholdSource=arg"    "F: source=arg"

echo
echo "── Scenario G: --model with shell metachar rejected ──"
scenario_run --model 'claude;rm -rf /' > g.log 2>&1
assert_exit_nonzero $? "G: shell metachar rejected"
assert_contains    g.log "[Ii]nvalid|does not match" "G: validation error"

echo
echo "── Scenario H: no global config → defaults apply ──"
remove_global_config
scenario_run > h.log 2>&1 || true
assert_contains h.log "model=claude-sonnet-4-6"  "H: default model"
assert_contains h.log "modelSource=default"      "H: source=default"
write_global_config $'model: claude-opus-4-7\nhideGit: true\n'

echo
echo "── Scenario I: removed --hide-git / --no-hide-git flags ──"
scenario_run --hide-git > i.log 2>&1
assert_exit_nonzero $? "I: --hide-git rejected"
scenario_run --no-hide-git >> i.log 2>&1
assert_exit_nonzero $? "I: --no-hide-git rejected"
assert_contains    i.log "unknown flag" "I: error names removed flag"
assert_not_contains i.log "hideGitSource=arg" "I: removed flag had no effect"

echo
echo "── Scenario J: --set workflow=branch --set pr=true ──"
reset_yaml_field workflow pr autoMerge
scenario_run --set workflow=branch --set pr=true > j.log 2>&1 || true
assert_contains j.log "workflow=branch workflowSource=arg" "J: workflow=branch from arg"
assert_contains j.log "pr=true prSource=arg"               "J: pr=true from arg"

echo
echo "── Scenario K: --set autoMerge=true on pr=true project ──"
reset_yaml_field workflow pr autoMerge
{ echo "workflow: branch"; echo "pr: true"; } >> .dark-factory.yaml
scenario_run --set autoMerge=true > k.log 2>&1 || true
assert_contains     k.log "autoMerge=true autoMergeSource=arg" "K: autoMerge=true from arg"
assert_not_contains k.log "autoMergeSource=project"            "K: source NOT project"

echo
echo "── Scenario L: --set workflow=direct --set pr=true rejected ──"
reset_yaml_field workflow pr autoMerge
scenario_run --set workflow=direct --set pr=true > l.log 2>&1
assert_exit_nonzero $? "L: validator rejects"
assert_contains    l.log "incompatible" "L: incompatible error"

echo
echo "── Scenario M: --set autoMerge=true without pr rejected ──"
reset_yaml_field workflow pr autoMerge
scenario_run --set autoMerge=true > m.log 2>&1
assert_exit_nonzero $? "M: validator rejects"
assert_contains    m.log "autoMerge requires pr: true" "M: autoMerge gate"

echo
echo "── Bonus: bad input rejection ──"
scenario_run --set workflow=invalid > bad-enum.log 2>&1
assert_exit_nonzero $? "invalid enum exits non-zero"
assert_contains    bad-enum.log "unknown workflow" "invalid enum error"

scenario_run --set workflow=pr > legacy.log 2>&1
assert_exit_nonzero $? "legacy workflow=pr exits non-zero"
assert_contains    legacy.log "legacy workflow value" "legacy enum rejection"

scenario_run --set pr=yes > bad-bool.log 2>&1
assert_exit_nonzero $? "pr=yes exits non-zero"
assert_contains    bad-bool.log "invalid bool" "bool format error"

echo
echo "── Bonus: last-wins + composition ──"
scenario_run --set workflow=branch --set workflow=clone > lw.log 2>&1 || true
assert_contains lw.log "workflow=clone workflowSource=arg" "last-wins"

scenario_run --set workflow=clone --set pr=true --set autoMerge=true > all3.log 2>&1 || true
assert_contains all3.log "workflow=clone workflowSource=arg"  "compose: workflow"
assert_contains all3.log "pr=true prSource=arg"               "compose: pr"
assert_contains all3.log "autoMerge=true autoMergeSource=arg" "compose: autoMerge"

scenario_done
