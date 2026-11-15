// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package launchpolicy

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bborbe/errors"
)

// EnvManagedMarker is the environment variable dark-factory sets on every
// container it launches. Its presence signals to processes inside the container
// that the dark-factory daemon manages this container's lifecycle — for example,
// the generate-prompts-for-spec command skips its host-only `spec mark-prompted`
// step because the host-side generator finalizes the spec after the container
// exits (and the dark-factory CLI is not present inside the container).
const EnvManagedMarker = "DARK_FACTORY_MANAGED"

// WithManagedMarker returns a copy of env with EnvManagedMarker set to "true".
// The marker is written after copying so it takes precedence over any inbound
// value for the same key. A new map is returned so the caller's map is never
// mutated, and a non-nil map is always returned so the marker is emitted even
// when env is nil.
func WithManagedMarker(env map[string]string) map[string]string {
	merged := make(map[string]string, len(env)+1)
	for k, v := range env {
		merged[k] = v
	}
	merged[EnvManagedMarker] = "true"
	return merged
}

// FormatEnvFile renders env as `--env-file` lines, sorted by key so the output
// is byte-stable for tests. docker does not interpolate or quote values in an
// env-file, so each value is written verbatim; a value containing a newline
// cannot be represented in this format and would produce two lines.
func FormatEnvFile(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(env[k])
		b.WriteString("\n")
	}
	return b.String()
}

// PrepareEnvFile materialises opts.Env into a 0600 file inside a fresh 0700 temp
// directory, points opts.EnvFile at it, and clears opts.Env so that no env value
// — credential or otherwise — can reach the docker argv.
//
// The container env reaches docker via `--env-file`, which the docker CLI reads
// on the HOST as the invoking user. The container never opens the file, so the
// file needs no container-visible path and 0600 is sufficient for docker to read
// it.
//
// 0600 is NOT an isolation boundary against processes running as the same uid.
// Another agent session on this machine can still read the file, and its path is
// in the argv, so the exposure is one `ps` plus one `cat` rather than a bare
// `ps`. What this change removes is the credential being printed by the process
// listing itself. Two residuals remain and are out of scope here: same-uid reads
// of the file, and the value transiting the docker API into the container config
// (readable with `docker inspect` for the container's lifetime).
//
// The returned cleanup removes the temp directory. Callers must defer it. A
// SIGKILL of the dark-factory process skips the cleanup and leaves a 0600 file
// in the user's own temp directory.
func PrepareEnvFile(ctx context.Context, opts *ContainerLaunchOpts) (func(), error) {
	dir, err := os.MkdirTemp("", "dark-factory-env-")
	if err != nil {
		return nil, errors.Wrap(ctx, err, "create env temp dir")
	}
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte(FormatEnvFile(WithManagedMarker(opts.Env))), 0600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, errors.Wrap(ctx, err, "write env file")
	}
	opts.EnvFile = path
	opts.Env = nil
	return func() { _ = os.RemoveAll(dir) }, nil
}

// secretEnvKeyParts are the substrings that mark an env key as credential-shaped.
// Used only by DropSecretEnvKeys, the fail-closed guard on the legacy `-e` path.
//
// This is a heuristic, not an exhaustive list: a credential named without one of
// these parts (e.g. AWS_ACCESS_KEY_ID before ACCESS_KEY was added, or a bespoke
// CLOUD_CRED) is not caught. It is defence in depth on a path production no
// longer takes — the structural guarantee is that `--env-file` is used at all.
var secretEnvKeyParts = []string{
	"TOKEN", "SECRET", "PASSWORD", "API_KEY", "APIKEY", "PRIVATE_KEY", "ACCESS_KEY",
}

// IsSecretEnvKey reports whether an env key looks like a credential. The test is
// deliberately over-inclusive: a false positive costs one env var moved to the
// env-file, a false negative puts a credential in the process table.
func IsSecretEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, part := range secretEnvKeyParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}

// DropSecretEnvKeys returns a copy of env with credential-shaped keys removed,
// plus the names of the keys it dropped (sorted). A copy is always returned when
// env is non-empty, so mutating the result never mutates the input. This is the
// fail-closed guard on the legacy `-e` path: a call site that forgets
// PrepareEnvFile loses the variable (loudly, via the returned names) instead of
// publishing it to `ps`.
func DropSecretEnvKeys(env map[string]string) (map[string]string, []string) {
	if len(env) == 0 {
		return nil, nil
	}
	kept := make(map[string]string, len(env))
	var dropped []string
	for k, v := range env {
		if IsSecretEnvKey(k) {
			dropped = append(dropped, k)
			continue
		}
		kept[k] = v
	}
	sort.Strings(dropped)
	return kept, dropped
}
