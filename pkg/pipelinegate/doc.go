// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pipelinegate refuses daemon startup when `dark-factory doctor`
// reports pipeline findings, naming the offending prompt numbers so a blocked
// queue cannot masquerade as an idle one.
package pipelinegate
