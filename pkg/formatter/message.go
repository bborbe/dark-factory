// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package formatter

import "encoding/json"

// StreamMessage is the top-level envelope for every newline-delimited event.
type StreamMessage struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message json.RawMessage `json:"message"`
	// Result event fields (type == "result"):
	DurationMs   *float64 `json:"duration_ms"`
	TotalCostUSD *float64 `json:"total_cost_usd"`
	Usage        *Usage   `json:"usage"`
	Result       *string  `json:"result"`
	IsError      *bool    `json:"is_error"`
	// RateLimitInfo is populated when Type == "rate_limit_event".
	RateLimitInfo *RateLimitInfo `json:"rate_limit_info"`
	// System init fields (type == "system", subtype == "init"):
	SessionID string   `json:"session_id"`
	Model     string   `json:"model"`
	Cwd       string   `json:"cwd"`
	Tools     []string `json:"tools"`
}

// RateLimitInfo contains the details of a rate_limit_event message.
type RateLimitInfo struct {
	// Status is the rate-limit status string, e.g. "allowed_warning" or "blocked".
	Status string `json:"status"`
	// ResetsAt is the unix epoch (seconds) when the bucket resets. Zero means unknown.
	ResetsAt int64 `json:"resetsAt"`
	// RateLimitType identifies the bucket, e.g. "seven_day" or "five_hour".
	RateLimitType string `json:"rateLimitType"`
	// Utilization is the fraction of the bucket consumed, 0.0–1.0.
	Utilization float64 `json:"utilization"`
	// IsUsingOverage indicates overage consumption.
	IsUsingOverage bool `json:"isUsingOverage"`
	// SurpassedThreshold indicates the threshold has been crossed.
	SurpassedThreshold bool `json:"surpassedThreshold"`
}

// Usage captures the token counts from the final result event.
type Usage struct {
	InputTokens          *int `json:"input_tokens"`
	OutputTokens         *int `json:"output_tokens"`
	CacheReadInputTokens *int `json:"cache_read_input_tokens"`
}

// AssistantMessage is the decoded form of StreamMessage.Message for type == "assistant".
type AssistantMessage struct {
	Content []ContentBlock `json:"content"`
}

// UserMessage is the decoded form of StreamMessage.Message for type == "user".
type UserMessage struct {
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents one element of a message's content array.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
	// tool_use fields:
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// tool_result fields:
	ToolUseID string          `json:"tool_use_id"`
	IsError   *bool           `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}
