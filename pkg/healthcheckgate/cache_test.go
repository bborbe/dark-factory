// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healthcheckgate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/healthcheckgate"
)

var _ = Describe("FileCache", func() {
	var (
		ctx     context.Context
		root    string
		cache   healthcheckgate.Cache
		baseKey string
		now     time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		root = GinkgoT().TempDir()
		cache = healthcheckgate.NewFileCache(root)
		baseKey = "testkey"
		now = time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	})

	Describe("Write then Fresh within interval", func() {
		It("returns true", func() {
			cache.Write(ctx, baseKey, now)
			Expect(cache.Fresh(ctx, baseKey, time.Hour, now.Add(time.Minute))).To(BeTrue())
		})
	})

	Describe("Fresh after interval elapsed", func() {
		It("returns false", func() {
			cache.Write(ctx, baseKey, now)
			later := now.Add(2 * time.Hour)
			Expect(cache.Fresh(ctx, baseKey, time.Hour, later)).To(BeFalse())
		})
	})

	Describe("missing file", func() {
		It("returns false without warning", func() {
			Expect(cache.Fresh(ctx, "no-such-key", time.Hour, now)).To(BeFalse())
		})
	})

	Describe("corrupt file", func() {
		It("returns false", func() {
			path := filepath.Join(root, "healthcheck-"+baseKey+".json")
			Expect(os.WriteFile(path, []byte("{garbage"), 0600)).To(Succeed())
			Expect(cache.Fresh(ctx, baseKey, time.Hour, now)).To(BeFalse())
		})
	})

	Describe("future timestamp", func() {
		It("returns false when cached timestamp is ahead of now", func() {
			future := now.Add(time.Hour)
			cache.Write(ctx, baseKey, future)
			Expect(cache.Fresh(ctx, baseKey, 24*time.Hour, now)).To(BeFalse())
		})
	})

	Describe("interval <= 0", func() {
		It("returns false", func() {
			cache.Write(ctx, baseKey, now)
			Expect(cache.Fresh(ctx, baseKey, 0, now.Add(time.Second))).To(BeFalse())
		})
	})

	Describe("Write creates the cache file with correct content", func() {
		It("stores a success record", func() {
			cache.Write(ctx, baseKey, now)
			path := filepath.Join(root, "healthcheck-"+baseKey+".json")
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			var rec struct {
				CheckedAt string `json:"checkedAt"`
			}
			Expect(json.Unmarshal(data, &rec)).To(Succeed())
			t, err := time.Parse(time.RFC3339Nano, rec.CheckedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(t).To(BeTemporally("~", now, time.Second))
		})
	})
})

var _ = Describe("CacheKey", func() {
	It("is deterministic for the same inputs", func() {
		k1 := healthcheckgate.CacheKey("img:v1", "myproject", time.Hour)
		k2 := healthcheckgate.CacheKey("img:v1", "myproject", time.Hour)
		Expect(k1).To(Equal(k2))
	})

	It("differs for different container images", func() {
		k1 := healthcheckgate.CacheKey("img:v1", "myproject", time.Hour)
		k2 := healthcheckgate.CacheKey("img:v2", "myproject", time.Hour)
		Expect(k1).NotTo(Equal(k2))
	})

	It("differs for different project names", func() {
		k1 := healthcheckgate.CacheKey("img:v1", "project-a", time.Hour)
		k2 := healthcheckgate.CacheKey("img:v1", "project-b", time.Hour)
		Expect(k1).NotTo(Equal(k2))
	})

	It("differs for different intervals", func() {
		k1 := healthcheckgate.CacheKey("img:v1", "myproject", time.Hour)
		k2 := healthcheckgate.CacheKey("img:v1", "myproject", 2*time.Hour)
		Expect(k1).NotTo(Equal(k2))
	})
})
