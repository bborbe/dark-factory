// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package promptstate is the single owner of prompt-state interpretation.
// It declares the seven canonical in-memory State values (distinct from the
// on-disk pkg/prompt.PromptStatus), a transition table with IsValidTransition,
// and the pure function InterpretTuple that maps the four observable inputs
// (filesystem location, frontmatter status, container name, docker liveness)
// to an authoritative State. No existing consumer is wired to this package yet;
// that migration happens in later prompts.
package promptstate
