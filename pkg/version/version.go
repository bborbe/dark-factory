// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

// Version is the dark-factory version, overridden at build time via ldflags.
var Version = "dev"

// Getter returns the current version.
//
//counterfeiter:generate -o ../../mocks/version-getter.go --fake-name VersionGetter . Getter
type Getter interface {
	Get() string
}

// versionGetter implements Getter.
type versionGetter struct {
	version string
}

// NewGetter creates a Getter that returns the provided version.
func NewGetter(version string) Getter {
	return &versionGetter{
		version: version,
	}
}

// Get returns the version.
func (v *versionGetter) Get() string {
	return v.version
}
