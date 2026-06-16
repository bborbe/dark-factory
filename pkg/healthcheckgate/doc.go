// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package healthcheckgate runs the existing healthcheck probe sequence once at
// daemon startup and caches success-only on the host filesystem so a restart
// within the cache window skips the probes entirely.
package healthcheckgate
