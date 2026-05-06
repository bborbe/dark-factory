// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/factory"
)

var _ = Describe("buildIdleLogger", func() {
	ctx := context.Background()
	cancel := func() {}

	newCounter := func() (*int32, func()) {
		var n int32
		return &n, func() { atomic.AddInt32(&n, 1) }
	}

	It("emits exactly once for a burst of concurrent calls", func() {
		const queueInterval = 10 * time.Millisecond
		const idleLogInterval = 50 * time.Millisecond
		count, emit := newCounter()
		fn := factory.BuildIdleLoggerForTest(idleLogInterval, queueInterval, emit)

		for i := 0; i < 4; i++ {
			fn(ctx, cancel)
		}

		Expect(atomic.LoadInt32(count)).To(Equal(int32(1)))
	})

	It("emits heartbeat after idleLogInterval elapses", func() {
		// queueInterval is large so 2*queueInterval >> sleep, preventing re-arm
		const queueInterval = 500 * time.Millisecond
		const idleLogInterval = 60 * time.Millisecond
		count, emit := newCounter()
		fn := factory.BuildIdleLoggerForTest(idleLogInterval, queueInterval, emit)

		fn(ctx, cancel)
		Expect(atomic.LoadInt32(count)).To(Equal(int32(1)))

		time.Sleep(idleLogInterval + 10*time.Millisecond)
		fn(ctx, cancel)

		Expect(atomic.LoadInt32(count)).To(Equal(int32(2)))
	})

	It("suppresses heartbeat when called before idleLogInterval elapses", func() {
		// queueInterval must be large so 2*queueInterval >> sleep, preventing re-arm
		const queueInterval = 500 * time.Millisecond
		const idleLogInterval = 200 * time.Millisecond
		count, emit := newCounter()
		fn := factory.BuildIdleLoggerForTest(idleLogInterval, queueInterval, emit)

		fn(ctx, cancel)
		time.Sleep(idleLogInterval / 2) // 100ms << 2*queueInterval (1s)
		fn(ctx, cancel)

		Expect(atomic.LoadInt32(count)).To(Equal(int32(1)))
	})

	It("disables heartbeat when idleLogInterval is zero", func() {
		const queueInterval = 10 * time.Millisecond
		count, emit := newCounter()
		fn := factory.BuildIdleLoggerForTest(0, queueInterval, emit)

		for i := 0; i < 5; i++ {
			fn(ctx, cancel)
		}

		Expect(atomic.LoadInt32(count)).To(Equal(int32(1)))
	})

	It("still emits first-entry when idleLogInterval is zero", func() {
		const queueInterval = 10 * time.Millisecond
		count, emit := newCounter()
		fn := factory.BuildIdleLoggerForTest(0, queueInterval, emit)

		fn(ctx, cancel)

		Expect(atomic.LoadInt32(count)).To(Equal(int32(1)))
	})

	It("re-arms first-entry after gap larger than 2*queueInterval", func() {
		const queueInterval = 10 * time.Millisecond
		const idleLogInterval = 50 * time.Millisecond
		count, emit := newCounter()
		fn := factory.BuildIdleLoggerForTest(idleLogInterval, queueInterval, emit)

		fn(ctx, cancel)
		Expect(atomic.LoadInt32(count)).To(Equal(int32(1)))

		time.Sleep(3 * queueInterval) // 30ms > 2*queueInterval (20ms)
		fn(ctx, cancel)

		Expect(atomic.LoadInt32(count)).To(Equal(int32(2)))
	})
})
