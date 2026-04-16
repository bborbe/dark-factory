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
	// System init fields (type == "system", subtype == "init"):
	SessionID string   `json:"session_id"`
	Model     string   `json:"model"`
	Cwd       string   `json:"cwd"`
	Tools     []string `json:"tools"`
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
