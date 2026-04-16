// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package formatter

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxLine        = 200 // per-line truncation for rendered input previews
	maxPromptChars = 160 // Task/Agent prompt preview
	maxTailLines   = 20  // Bash failure tail length
)

// renderSystemInit renders the system init event.
func renderSystemInit(msg StreamMessage) string {
	if msg.Subtype != "init" {
		return formatTimestamp() + " [system: " + msg.Subtype + "]\n"
	}
	session := msg.SessionID
	if len(session) > 8 {
		session = session[:8]
	}
	if session == "" {
		session = "(unknown)"
	}
	return fmt.Sprintf("%s [init] session=%s model=%s cwd=%s tools=%d\n",
		formatTimestamp(), session, msg.Model, msg.Cwd, len(msg.Tools))
}

// renderResult renders the final result event.
func renderResult(msg StreamMessage) string {
	var parts []string
	if msg.Result != nil {
		parts = append(parts, "result: "+*msg.Result)
	}
	if msg.DurationMs != nil {
		parts = append(parts, fmt.Sprintf("duration: %.0fms", *msg.DurationMs))
	}
	if msg.TotalCostUSD != nil {
		parts = append(parts, fmt.Sprintf("cost: $%.4f", *msg.TotalCostUSD))
	}
	if msg.Usage != nil && msg.Usage.InputTokens != nil && msg.Usage.OutputTokens != nil {
		parts = append(parts, fmt.Sprintf("tokens: %d in / %d out",
			*msg.Usage.InputTokens, *msg.Usage.OutputTokens))
	}
	return formatTimestamp() + " " + strings.Join(parts, " | ") + "\n"
}

// renderAssistant renders an assistant message.
func renderAssistant(msg StreamMessage, toolNames map[string]string) string {
	var am AssistantMessage
	if err := json.Unmarshal(msg.Message, &am); err != nil {
		return formatTimestamp() + " [assistant: parse error]\n"
	}

	var sb strings.Builder
	for _, block := range am.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				sb.WriteString(formatTimestamp() + " " + block.Text + "\n")
			}
		case "thinking":
			preview := truncate(block.Thinking, 500)
			sb.WriteString(formatTimestamp() + " 💭 " + preview + "\n")
		case "tool_use":
			toolNames[block.ID] = block.Name
			sb.WriteString(renderToolUse(block))
		default:
			sb.WriteString(formatTimestamp() + " [assistant content: " + block.Type + "]\n")
		}
	}
	return sb.String()
}

// renderUser renders a user message (tool results).
func renderUser(msg StreamMessage, toolNames map[string]string) string {
	var um UserMessage
	if err := json.Unmarshal(msg.Message, &um); err != nil {
		return formatTimestamp() + " [user: parse error]\n"
	}

	var sb strings.Builder
	for _, block := range um.Content {
		if block.Type != "tool_result" {
			continue
		}
		toolName := toolNames[block.ToolUseID]
		if toolName == "" {
			toolName = "(unknown tool)"
		}
		isError := block.IsError != nil && *block.IsError
		sb.WriteString(renderToolResult(block, toolName, isError))
	}
	return sb.String()
}

// renderToolUse renders a tool_use content block following the Python v2 oracle format.
func renderToolUse(block ContentBlock) string {
	var inp map[string]json.RawMessage
	_ = json.Unmarshal(block.Input, &inp)
	if inp == nil {
		inp = make(map[string]json.RawMessage)
	}

	line := formatToolUse(block.Name, inp)
	return formatTimestamp() + " " + line + "\n"
}

// formatToolUse formats a tool use following the Python v2 oracle exactly.
func formatToolUse(name string, inp map[string]json.RawMessage) string {
	switch name {
	case "Bash":
		return "$ " + shorten(jsonString(inp, "command"), maxLine)
	case "Read":
		return formatReadToolUse(inp)
	case "Write":
		sz := len(jsonString(inp, "content"))
		return fmt.Sprintf("[write] %s (%d chars)", jsonString(inp, "file_path"), sz)
	case "Edit":
		return formatEditToolUse(inp)
	case "Grep":
		return formatGrepToolUse(inp)
	case "Glob":
		return "[glob] " + jsonString(inp, "pattern")
	case "Task", "Agent":
		return formatAgentToolUse(inp)
	case "WebFetch":
		return "[webfetch] " + jsonString(inp, "url")
	case "WebSearch":
		return "[websearch] " + jsonString(inp, "query")
	case "ToolSearch":
		return "[toolsearch] " + shorten(jsonString(inp, "query"), 100)
	case "AskUserQuestion":
		return formatAskToolUse(inp)
	case "Skill":
		return formatSkillToolUse(inp)
	case "TodoWrite":
		return formatTodoWrite(inp)
	case "NotebookEdit":
		return "[notebook] " + jsonString(inp, "notebook_path")
	default:
		return formatUnknownOrMCP(name)
	}
}

// formatReadToolUse formats the Read tool_use line.
func formatReadToolUse(inp map[string]json.RawMessage) string {
	fp := jsonString(inp, "file_path")
	offset := jsonAny(inp, "offset")
	limit := jsonAny(inp, "limit")
	if offset != "" || limit != "" {
		return fmt.Sprintf("[read] %s (offset=%s, limit=%s)", fp, offset, limit)
	}
	return "[read] " + fp
}

// formatEditToolUse formats the Edit tool_use line.
func formatEditToolUse(inp map[string]json.RawMessage) string {
	fp := jsonString(inp, "file_path")
	old := shorten(jsonString(inp, "old_string"), 60)
	newStr := shorten(jsonString(inp, "new_string"), 60)
	return fmt.Sprintf("[edit] %s\n    - %s\n    + %s", fp, old, newStr)
}

// formatGrepToolUse formats the Grep tool_use line.
func formatGrepToolUse(inp map[string]json.RawMessage) string {
	pat := jsonString(inp, "pattern")
	path := jsonString(inp, "path")
	glob := jsonString(inp, "glob")
	var extras []string
	if path != "" {
		extras = append(extras, "path="+path)
	}
	if glob != "" {
		extras = append(extras, "glob="+glob)
	}
	result := "[grep] " + pat
	if len(extras) > 0 {
		result += "  (" + strings.Join(extras, " ") + ")"
	}
	return result
}

// formatAgentToolUse formats the Task/Agent tool_use line.
func formatAgentToolUse(inp map[string]json.RawMessage) string {
	desc := jsonString(inp, "description")
	sub := jsonString(inp, "subagent_type")
	if sub == "" {
		sub = "general"
	}
	prompt := shorten(jsonString(inp, "prompt"), maxPromptChars)
	return fmt.Sprintf("[agent:%s] %s\n    prompt: %s", sub, desc, prompt)
}

// formatAskToolUse formats the AskUserQuestion tool_use line.
func formatAskToolUse(inp map[string]json.RawMessage) string {
	q := jsonString(inp, "question")
	if q == "" {
		q = jsonFirstQuestion(inp)
	}
	return "[ask] " + shorten(q, 120)
}

// formatSkillToolUse formats the Skill tool_use line.
func formatSkillToolUse(inp map[string]json.RawMessage) string {
	skill := jsonString(inp, "skill")
	args := jsonString(inp, "args")
	result := "[skill] /" + skill
	if args != "" {
		result += " " + args
	}
	return result
}

// formatUnknownOrMCP formats mcp__ tools or unknown tool names.
func formatUnknownOrMCP(name string) string {
	if strings.HasPrefix(name, "mcp__") {
		parts := strings.SplitN(name, "__", 3)
		server := "?"
		method := "?"
		if len(parts) > 1 {
			server = parts[1]
		}
		if len(parts) > 2 {
			method = parts[2]
		}
		return fmt.Sprintf("[mcp:%s] %s", server, method)
	}
	return "[" + name + "]"
}

// formatTodoWrite formats a TodoWrite tool use.
func formatTodoWrite(inp map[string]json.RawMessage) string {
	var todos []struct {
		Status  string `json:"status"`
		Content string `json:"content"`
	}
	if raw, ok := inp["todos"]; ok {
		_ = json.Unmarshal(raw, &todos)
	}
	done := 0
	total := len(todos)
	var active *struct {
		Status  string `json:"status"`
		Content string `json:"content"`
	}
	for i := range todos {
		if todos[i].Status == "completed" {
			done++
		}
		if todos[i].Status == "in_progress" && active == nil {
			active = &todos[i]
		}
	}
	if active != nil {
		return fmt.Sprintf("[todo] %d/%d active: %s", done, total, shorten(active.Content, 80))
	}
	return fmt.Sprintf("[todo] %d/%d done", done, total)
}

// renderToolResult renders a tool_result content block following the Python v2 oracle.
func renderToolResult(block ContentBlock, toolName string, isError bool) string {
	text := extractResultPreview(block.Content)

	if isError {
		return formatTimestamp() + " ⚠ [" + toolName + "] error: " + shorten(text, 400) + "\n"
	}

	switch toolName {
	case "Bash":
		return renderBashResult(text)
	case "Read":
		lineCount := strings.Count(text, "\n")
		return fmt.Sprintf("%s   → %d lines read\n", formatTimestamp(), lineCount)
	case "Write", "Edit":
		return renderWriteEditResult(text)
	case "Grep":
		lineCount := strings.Count(text, "\n")
		return fmt.Sprintf("%s   → %d match lines\n", formatTimestamp(), lineCount)
	case "Glob":
		lineCount := strings.Count(text, "\n")
		return fmt.Sprintf("%s   → %d files\n", formatTimestamp(), lineCount)
	case "Task", "Agent":
		lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)
		first := ""
		if len(lines) > 0 {
			first = lines[0]
		}
		return formatTimestamp() + "   ← agent reply: " + shorten(first, maxLine) + "\n"
	default:
		return ""
	}
}

// renderBashResult renders the result of a Bash tool_use.
func renderBashResult(text string) string {
	lower := strings.ToLower(text)
	looksFailed := strings.Contains(lower, "error") ||
		strings.Contains(lower, "exit code") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "fatal")
	if looksFailed {
		lines := strings.Split(text, "\n")
		start := len(lines) - maxTailLines
		if start < 0 {
			start = 0
		}
		tail := strings.Join(lines[start:], "\n")
		return formatTimestamp() + " ⚠ [bash output]\n" + tail + "\n"
	}
	return ""
}

// renderWriteEditResult renders the result of a Write or Edit tool_use.
func renderWriteEditResult(text string) string {
	lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)
	if len(lines) > 0 && lines[0] != "" {
		return formatTimestamp() + "   → " + shorten(lines[0], 120) + "\n"
	}
	return ""
}

// extractResultPreview extracts a string preview from a tool result content field.
// content can be a JSON string or a []ContentBlock.
func extractResultPreview(content json.RawMessage) string {
	if len(content) == 0 {
		return "(result)"
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return "(result)"
}

// shorten truncates s to maxLen characters, replacing newlines with \n literal,
// and appending … if truncated. Matches Python v2 oracle shorten() behaviour.
func shorten(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// truncate returns s truncated to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

// jsonAny extracts any JSON scalar value as a string (strings, numbers, booleans).
// Returns empty string for missing keys or null values.
func jsonAny(inp map[string]json.RawMessage, key string) string {
	raw, ok := inp[key]
	if !ok || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// jsonString extracts a string value from a map of raw JSON values.
func jsonString(inp map[string]json.RawMessage, key string) string {
	raw, ok := inp[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// jsonFirstQuestion extracts the first question from the questions array in AskUserQuestion input.
func jsonFirstQuestion(inp map[string]json.RawMessage) string {
	raw, ok := inp["questions"]
	if !ok {
		return ""
	}
	var questions []struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(raw, &questions); err != nil || len(questions) == 0 {
		return ""
	}
	return questions[0].Question
}
