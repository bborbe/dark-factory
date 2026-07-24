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
#   scenario_setup [YAML]       — mktemp + git init + write .dark-factory.yaml + cd (empty sandbox)
#   setup_sandbox_copy [YAML] [SUBDIR] [SANDBOX_SRC] — mktemp + cp dark-factory-sandbox + write
#                                  .dark-factory.yaml + init bare remote + set origin + cd (full
#                                  sandbox copy with bare remote, suitable for scenarios that
#                                  exercise real prompts/specs against the canonical test repo)
#   cleanup_sandbox [WORK_DIR]  — rm -rf the given dir (or $WORK_DIR if omitted); use when an
#                                  explicit cleanup is desired and the EXIT trap is not enough
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
  # NOTE: no `trap ... EXIT` here — see the source-time trap at the bottom of this
  # file. In zsh, a trap set inside a function is function-local and fires when the
  # FUNCTION returns, which deleted $WORK_DIR out from under the running scenario.
  cd "$WORK_DIR" || exit 1
  git init -q .
  printf '%s' "$yaml" > .dark-factory.yaml
  mkdir -p prompts/in-progress prompts/completed prompts/log \
           specs/in-progress  specs/completed  specs/log
  git -c user.email=t@t -c user.name=t add -A
  git -c user.email=t@t -c user.name=t commit -qm init
  echo "→ sandbox: $WORK_DIR"
  echo "→ HOME:    $HOME"
}

setup_sandbox_copy() {
  # Copy the dark-factory-sandbox repo into a temp dir, write .dark-factory.yaml,
  # init a bare remote, redirect origin to it, and cd into the sandbox.
  # Sets WORK_DIR. Registers an EXIT trap to clean up.
  #
  # Usage:
  #   setup_sandbox_copy                                      # default yaml, default subdir "sandbox", default source
  #   setup_sandbox_copy "$(printf 'workflow: pr\n')"         # custom yaml
  #   setup_sandbox_copy "$YAML" "dark-factory-sandbox"       # name the subdir (some scenarios use this name)
  #   setup_sandbox_copy "$YAML" "sandbox" /path/to/sandbox   # override source path
  local yaml=${1:-$'workflow: direct\nautoRelease: false\nmaxContainers: 999\n'}
  local subdir=${2:-sandbox}
  local sandbox_src=${3:-${DARK_FACTORY_SANDBOX:-$HOME/Documents/workspaces/dark-factory-sandbox}}
  if [ ! -d "$sandbox_src" ]; then
    echo "setup_sandbox_copy: sandbox source missing: $sandbox_src" >&2
    return 1
  fi
  WORK_DIR=$(mktemp -d)
  # NOTE: no `trap ... EXIT` here — see the source-time trap at the bottom of this
  # file (zsh makes in-function traps function-local; it fired on return).
  cp -r "$sandbox_src" "$WORK_DIR/$subdir"
  cd "$WORK_DIR/$subdir" || exit 1
  printf '%s' "$yaml" > .dark-factory.yaml
  git init --bare "$WORK_DIR/remote.git" >/dev/null 2>&1
  git remote set-url origin "$WORK_DIR/remote.git"
  echo "→ sandbox: $WORK_DIR/$subdir"
  echo "→ remote:  $WORK_DIR/remote.git"
}

cleanup_sandbox() {
  # Explicit cleanup, in case the caller doesn't want to wait for the EXIT trap.
  # Usage:
  #   cleanup_sandbox             # uses $WORK_DIR
  #   cleanup_sandbox /path/dir
  local dir=${1:-${WORK_DIR:-}}
  if [ -n "$dir" ] && [ -d "$dir" ]; then
    rm -rf "$dir"
  fi
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

# --- sandbox cleanup trap (registered at SOURCE time, on purpose) --------------
#
# This must be registered here at top level, NOT inside scenario_setup /
# setup_sandbox_copy. In zsh, `trap ... EXIT` set inside a function is
# function-local: it runs when the FUNCTION returns, not when the shell exits.
# That deleted $WORK_DIR the instant setup returned, so the very next command in
# the scenario died with:
#
#     fatal: Unable to read current working directory: No such file or directory
#
# ...which reads like a dark-factory bug but is purely a harness artifact.
# Sourcing this file happens at the caller's top level, so the trap below is
# global in both bash and zsh. $WORK_DIR is resolved at trap time, so it cleans
# up whichever sandbox the setup functions created (or nothing, if unset).
trap 'rm -rf "${WORK_DIR:-}"' EXIT
