// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package containerlock provides a system-wide file lock that serializes the
// "count running containers → start container" sequence across multiple
// dark-factory daemon instances.
package containerlock
