// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package specnum_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSpecnum(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Specnum Suite")
}
