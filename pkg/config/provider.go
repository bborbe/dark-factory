// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"

	"github.com/bborbe/collection"
	"github.com/bborbe/errors"
	"github.com/bborbe/validation"
)

// Provider selects the git hosting provider for PR operations.
const (
	ProviderGitHub          Provider = "github"
	ProviderBitbucketServer Provider = "bitbucket-server"
)

// AvailableProviders contains all valid provider values.
var AvailableProviders = Providers{ProviderGitHub, ProviderBitbucketServer}

// Provider is a string-based enum for git provider types.
type Provider string

// String returns the string representation of the Provider.
func (p Provider) String() string {
	return string(p)
}

// Validate checks that the Provider is a known value.
func (p Provider) Validate(ctx context.Context) error {
	if !AvailableProviders.Contains(p) {
		return errors.Wrapf(
			ctx,
			validation.Error,
			"unknown provider %q, valid values: github, bitbucket-server",
			p,
		)
	}
	return nil
}

// Ptr returns a pointer to the Provider value.
func (p Provider) Ptr() *Provider {
	return &p
}

// Providers is a collection of Provider values.
type Providers []Provider

// Contains returns true if the given provider is in the collection.
func (p Providers) Contains(provider Provider) bool {
	return collection.Contains(p, provider)
}
