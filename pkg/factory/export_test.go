// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"reflect"
	"strings"
	"time"

	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/config"
)

// BuildIdleLoggerForTest exposes buildIdleLogger for unit testing.
var BuildIdleLoggerForTest = func(
	idleLogInterval time.Duration,
	queueInterval time.Duration,
	emit func(),
) func(context.Context, context.CancelFunc) {
	return buildIdleLogger(idleLogInterval, queueInterval, emit)
}

// ResolveHideGitForTest exposes the package-private helper for
// black-box tests in factory_test.go.
var ResolveHideGitForTest = resolveHideGit

// ProviderDepsBackendForTest reports the backend ("github" or "bitbucket")
// the createProviderDeps dispatcher chose for the given config. Used by
// black-box tests in factory_test.go to assert the dispatch wiring without
// having to expose the unexported providerDeps struct.
//
// Implementation detail: inspects the package path of the returned
// prCreator's underlying type (pkg/git/* → "github",
// pkg/gitprovider/bitbucket/* → "bitbucket"). Returns "unknown" if neither
// matches.
var ProviderDepsBackendForTest = func(
	ctx context.Context,
	cfg config.Config,
	getter libtime.CurrentDateTimeGetter,
) string {
	deps := createProviderDeps(ctx, cfg, getter)
	pkgPath := reflect.TypeOf(deps.prCreator).Elem().PkgPath()
	switch {
	case strings.Contains(pkgPath, "/pkg/gitprovider/bitbucket"):
		return "bitbucket"
	case strings.Contains(pkgPath, "/pkg/git"):
		return "github"
	default:
		return "unknown"
	}
}
