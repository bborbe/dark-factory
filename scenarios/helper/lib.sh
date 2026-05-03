# shellcheck shell=bash
#
# Shared helpers for scenario runner scripts.
# Source this file from a scenario runner:
#
#     source "$(dirname "$0")/../helper/lib.sh"
#     scenario_setup
#     scenario_run --set workflow=branch --set pr=true > j.log 2>&1 || true
#     assert_contains j.log "workflow=branch workflowSource=arg" "J: workflow override"
#     scenario_done
#
# Exposes:
#   BIN          — path to the built binary (default: /tmp/new-dark-factory)
#   WORK_DIR     — temporary sandbox dir (created by scenario_setup, removed at exit)
#   PASS_COUNT, FAIL_COUNT — counters tracked by assert_*
#
# Functions:
#   build_binary [SRC_DIR]      — go build -o $BIN .
#   scenario_setup [YAML]       — mktemp + git init + write .dark-factory.yaml + cd
#   scenario_run ARGS...        — run "$BIN run ARGS..." with a timeout
#   scenario_run_command ARGS...— run "$BIN ARGS..." (no implicit "run" subcommand)
#   reset_yaml_workflow_pr      — strip workflow/pr/autoMerge lines from .dark-factory.yaml
#   assert_contains FILE PATTERN LABEL
#   assert_not_contains FILE PATTERN LABEL
#   assert_exit_zero EXIT LABEL
#   assert_exit_nonzero EXIT LABEL
#   scenario_done               — print summary and exit non-zero on any failure

set -uo pipefail

BIN=${BIN:-/tmp/new-dark-factory}
PASS_COUNT=0
FAIL_COUNT=0

build_binary() {
  local src_dir=${1:-${DARK_FACTORY_SRC:-$HOME/Documents/workspaces/dark-factory}}
  echo "→ building $BIN from $src_dir"
  go build -C "$src_dir" -o "$BIN" .
}

scenario_setup() {
  local yaml=${1:-$'workflow: direct\nautoRelease: false\n'}
  WORK_DIR=$(mktemp -d)
  # Isolate global config: override HOME so ~/.dark-factory points into the sandbox.
  export HOME="$WORK_DIR/home"
  mkdir -p "$HOME/.dark-factory"
  trap 'rm -rf "$WORK_DIR"' EXIT
  cd "$WORK_DIR"
  git init -q .
  printf '%s' "$yaml" > .dark-factory.yaml
  mkdir -p prompts/in-progress prompts/completed prompts/log \
           specs/in-progress  specs/completed  specs/log
  git -c user.email=t@t -c user.name=t add -A
  git -c user.email=t@t -c user.name=t commit -qm init
  echo "→ sandbox: $WORK_DIR"
  echo "→ HOME:    $HOME"
}

write_global_config() {
  printf '%s' "$1" > "$HOME/.dark-factory/config.yaml"
}

remove_global_config() {
  rm -f "$HOME/.dark-factory/config.yaml"
}

reset_yaml_field() {
  # Strip lines starting with any of the supplied field names from .dark-factory.yaml.
  # Usage: reset_yaml_field model workflow pr autoMerge
  local pattern="" f
  for f in "$@"; do
    pattern+="${pattern:+|}^${f}:"
  done
  grep -vE "$pattern" .dark-factory.yaml > .dark-factory.yaml.tmp || true
  mv .dark-factory.yaml.tmp .dark-factory.yaml
}

scenario_run() {
  timeout 5 "$BIN" run "$@"
}

scenario_run_command() {
  timeout 5 "$BIN" "$@"
}

reset_yaml_workflow_pr() {
  grep -v '^workflow:\|^pr:\|^autoMerge:' .dark-factory.yaml > .dark-factory.yaml.tmp
  mv .dark-factory.yaml.tmp .dark-factory.yaml
}

assert_contains() {
  local file=$1 pattern=$2 label=$3
  if grep -qE "$pattern" "$file" 2>/dev/null; then
    echo "  PASS  $label"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo "  FAIL  $label"
    echo "        pattern: $pattern"
    echo "        file:    $file"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

assert_not_contains() {
  local file=$1 pattern=$2 label=$3
  if grep -qE "$pattern" "$file" 2>/dev/null; then
    echo "  FAIL  $label (unexpected match)"
    echo "        pattern: $pattern"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  else
    echo "  PASS  $label"
    PASS_COUNT=$((PASS_COUNT + 1))
  fi
}

assert_exit_zero() {
  local exit_code=$1 label=$2
  if [ "$exit_code" -eq 0 ]; then
    echo "  PASS  $label (exit 0)"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo "  FAIL  $label (exit $exit_code, expected 0)"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

assert_exit_nonzero() {
  local exit_code=$1 label=$2
  if [ "$exit_code" -ne 0 ]; then
    echo "  PASS  $label (exit $exit_code)"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo "  FAIL  $label (exit 0, expected non-zero)"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

scenario_done() {
  echo
  echo "Result: $PASS_COUNT passed, $FAIL_COUNT failed"
  [ "$FAIL_COUNT" -eq 0 ]
}
