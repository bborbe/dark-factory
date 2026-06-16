// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healthcheckgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

//counterfeiter:generate -o ../../mocks/healthcheck-gate-cache.go --fake-name HealthcheckGateCache . Cache

// Cache stores success-only healthcheck results on the host filesystem.
type Cache interface {
	// Fresh reports whether a cached success exists and is younger than interval.
	// A missing, unreadable, malformed, or future-dated cache file is reported as
	// not-fresh (and, for unreadable/malformed/future cases, logs a single warning).
	Fresh(ctx context.Context, key string, interval time.Duration, now time.Time) bool
	// Write records a success at `now` under `key`. Errors are logged, not returned —
	// a cache-write failure must not abort daemon startup.
	Write(ctx context.Context, key string, now time.Time)
}

// NewFileCache returns a Cache rooted at the given directory.
// The factory (prompt 3) passes filepath.Join(home, ".dark-factory", "healthcheck-cache").
// Tests pass GinkgoT().TempDir().
func NewFileCache(root string) Cache {
	return &fileCache{root: root}
}

// fileCache implements Cache using the host filesystem.
type fileCache struct {
	root string
}

// cacheRecord is the JSON structure stored in the cache file.
type cacheRecord struct {
	CheckedAt string `json:"checkedAt"`
	Success   bool   `json:"success"`
}

// Fresh reports whether a cached success for `key` is fresher than `interval`.
func (f *fileCache) Fresh(
	_ context.Context,
	key string,
	interval time.Duration,
	now time.Time,
) bool {
	if interval <= 0 {
		return false
	}

	path := f.cachePath(key)
	// #nosec G304 -- path derived from internal root + hex-digest key, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		slog.Warn("healthcheck cache unreadable, re-running", "path", path, "error", err)
		return false
	}

	var rec cacheRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		slog.Warn("healthcheck cache unreadable, re-running", "path", path, "error", err)
		return false
	}

	checkedAt, err := time.Parse(time.RFC3339Nano, rec.CheckedAt)
	if err != nil {
		slog.Warn("healthcheck cache unreadable, re-running", "path", path, "error", err)
		return false
	}

	if now.Before(checkedAt) {
		slog.Warn("healthcheck cache timestamp in future, re-running", "path", path)
		return false
	}

	return now.Sub(checkedAt) < interval
}

// Write records a success at `now` under `key`. Errors are logged, never returned.
func (f *fileCache) Write(_ context.Context, key string, now time.Time) {
	if err := os.MkdirAll(f.root, 0750); err != nil {
		slog.Error("healthcheck cache: mkdir failed", "root", f.root, "error", err)
		return
	}

	rec := cacheRecord{
		CheckedAt: now.Format(time.RFC3339Nano),
		Success:   true,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		slog.Error("healthcheck cache: marshal failed", "error", err)
		return
	}

	path := f.cachePath(key)
	if err := os.WriteFile(path, data, 0600); err != nil {
		slog.Error("healthcheck cache: write failed", "path", path, "error", err)
	}
}

// cachePath returns the full path of the cache file for `key`.
func (f *fileCache) cachePath(key string) string {
	return filepath.Join(f.root, "healthcheck-"+key+".json")
}

// CacheKey returns the SHA256 hex digest of "<containerImage>:<projectName>:<intervalSeconds>".
// intervalSeconds is the interval expressed in whole seconds. Stable across runs for a
// given (image, project, interval) triple so different repos never collide.
func CacheKey(containerImage, projectName string, interval time.Duration) string {
	raw := fmt.Sprintf("%s:%s:%d", containerImage, projectName, int64(interval/time.Second))
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
