// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseArgs", func() {
	type result struct {
		debug       bool
		command     string
		subcommand  string
		args        []string
		autoApprove bool
	}
	parse := func(rawArgs []string) result {
		debug, command, subcommand, args, autoApprove := ParseArgs(rawArgs)
		return result{debug, command, subcommand, args, autoApprove}
	}

	It("returns run command with --help in args (validation at dispatch)", func() {
		r := parse([]string{"run", "--help"})
		Expect(r.command).To(Equal("run"))
		Expect(r.args).To(ContainElement("--help"))
	})

	It("returns daemon command with -h in args (validation at dispatch)", func() {
		r := parse([]string{"daemon", "-h"})
		Expect(r.command).To(Equal("daemon"))
		Expect(r.args).To(ContainElement("-h"))
	})

	It("returns prompt command with approve subcommand and --help in args", func() {
		r := parse([]string{"prompt", "approve", "--help"})
		Expect(r.command).To(Equal("prompt"))
		Expect(r.subcommand).To(Equal("approve"))
		Expect(r.args).To(ContainElement("--help"))
	})

	It("returns run command when given unknown positional arg (validation at dispatch)", func() {
		r := parse([]string{"run", "banana"})
		Expect(r.command).To(Equal("run"))
		Expect(r.args).To(ContainElement("banana"))
	})
})

var _ = Describe("validateNoArgs", func() {
	ctx := context.Background()
	noop := func() {}

	It("returns nil for empty args", func() {
		Expect(validateNoArgs(ctx, []string{}, noop)).To(Succeed())
	})

	It("returns error for unexpected positional arg", func() {
		Expect(validateNoArgs(ctx, []string{"banana"}, noop)).To(HaveOccurred())
	})

	It("returns error for unknown flag", func() {
		Expect(validateNoArgs(ctx, []string{"--foo"}, noop)).To(HaveOccurred())
	})
})

var _ = Describe("validateOneArg", func() {
	ctx := context.Background()
	noop := func() {}

	It("returns nil for exactly one arg", func() {
		Expect(validateOneArg(ctx, []string{"my-slug"}, noop)).To(Succeed())
	})

	It("returns error for missing arg", func() {
		Expect(validateOneArg(ctx, []string{}, noop)).To(HaveOccurred())
	})

	It("returns error for too many args", func() {
		Expect(validateOneArg(ctx, []string{"a", "b"}, noop)).To(HaveOccurred())
	})

	It("returns error for unknown flag", func() {
		Expect(validateOneArg(ctx, []string{"--foo"}, noop)).To(HaveOccurred())
	})
})
