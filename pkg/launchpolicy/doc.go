// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package launchpolicy materialises, exactly once per daemon process, the set of
// launch-shape inputs that are intrinsic to "this is a dark-factory container":
// image, project identity, mounts, base environment, netrc/gitconfig paths,
// hide-git, and the canonical Linux capability set (NET_ADMIN, NET_RAW).
//
// Two call sites consume it: the executor's prompt-run path
// (pkg/executor.dockerExecutor.buildDockerCommand) and the healthcheck probes'
// container-launch path (pkg/cmd/healthcheck.runContainerProbe). Both derive
// their executor.ContainerLaunchOpts from the same Policy value via
// Policy.BuildOpts; per-invocation differences (container name, entrypoint,
// command, env overlay, label overlay) flow through the Extras argument.
//
// A reader who wants to add a new launch-shape concern (new cap, new mount,
// new base env var) adds it here once. Both the executor and the probes pick
// it up with no further changes. The canonical container-startup-site
// inventory is maintained in the comment block on Policy itself; the
// inventory's enumerated set of file:line pairs must equal the UNION of:
//
//	grep -rnE 'exec\.Command(Context)?\(.*"docker"' pkg/ | grep -v _test.go
//	grep -rnE 'RunWithWarnAndTimeout\([^,]+,[^,]+,[^,]+"docker"' pkg/ | grep -v _test.go
//
// (Dark-factory invokes `docker` two ways: directly via `exec.Command*` and
// indirectly via `subproc.Runner.RunWithWarnAndTimeout`. The inventory must
// cover both. Any drift between the inventory and the union of the two grep
// outputs is an unresolved architectural divergence and must be fixed in the
// same change that introduced it.)
package launchpolicy
