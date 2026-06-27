// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package claudeargv_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/claudeargv"
)

var _ = Describe("EnvOverlay", func() {
	It("drops every empty field", func() {
		Expect(claudeargv.EnvOverlay(claudeargv.Options{})).To(BeEmpty())
	})

	It("emits the executor-shape (prompt run via mounted file)", func() {
		env := claudeargv.EnvOverlay(claudeargv.Options{
			Model:      "claude-sonnet-4-6",
			Output:     claudeargv.OutputJSON,
			PromptFile: "/tmp/prompt.md",
		})
		Expect(env).To(HaveKeyWithValue(claudeargv.EnvAnthropicModel, "claude-sonnet-4-6"))
		Expect(env).To(HaveKeyWithValue(claudeargv.EnvYoloOutput, "json"))
		Expect(env).To(HaveKeyWithValue(claudeargv.EnvYoloPromptFile, "/tmp/prompt.md"))
		Expect(env).NotTo(HaveKey(claudeargv.EnvYoloPrompt))
	})

	It("emits the probe-shape (inline prompt, no mount)", func() {
		env := claudeargv.EnvOverlay(claudeargv.Options{
			Prompt: "reply with exactly: OK",
			Output: claudeargv.OutputJSON,
		})
		Expect(env).To(HaveKeyWithValue(claudeargv.EnvYoloPrompt, "reply with exactly: OK"))
		Expect(env).To(HaveKeyWithValue(claudeargv.EnvYoloOutput, "json"))
		Expect(env).NotTo(HaveKey(claudeargv.EnvAnthropicModel))
		Expect(env).NotTo(HaveKey(claudeargv.EnvYoloPromptFile))
	})

	It(
		"does not allocate impossible combinations — both PromptFile and Prompt set is the caller's choice",
		func() {
			// Permissive: callers that pass both get both keys. entrypoint.sh
			// prefers YOLO_PROMPT_FILE when present, so this is the documented
			// override path if a caller ever wants it.
			env := claudeargv.EnvOverlay(claudeargv.Options{
				Prompt:     "fallback",
				PromptFile: "/tmp/p.md",
			})
			Expect(env).To(HaveKeyWithValue(claudeargv.EnvYoloPrompt, "fallback"))
			Expect(env).To(HaveKeyWithValue(claudeargv.EnvYoloPromptFile, "/tmp/p.md"))
		},
	)

	It("returns a fresh map each call (no shared state)", func() {
		a := claudeargv.EnvOverlay(claudeargv.Options{Model: "x"})
		b := claudeargv.EnvOverlay(claudeargv.Options{Model: "y"})
		Expect(a).To(HaveKeyWithValue(claudeargv.EnvAnthropicModel, "x"))
		Expect(b).To(HaveKeyWithValue(claudeargv.EnvAnthropicModel, "y"))
	})
})

var _ = Describe("ReservedKeys", func() {
	It("returns the keys validateEnv must reject in operator env blocks", func() {
		Expect(claudeargv.ReservedKeys()).To(ConsistOf(
			claudeargv.EnvYoloPromptFile,
			claudeargv.EnvAnthropicModel,
		))
	})

	It("does NOT reserve YOLO_OUTPUT (operator-settable in global env)", func() {
		Expect(claudeargv.ReservedKeys()).NotTo(ContainElement(claudeargv.EnvYoloOutput))
	})

	It("does NOT reserve YOLO_PROMPT (probe-only, not an operator-set concept)", func() {
		Expect(claudeargv.ReservedKeys()).NotTo(ContainElement(claudeargv.EnvYoloPrompt))
	})
})
