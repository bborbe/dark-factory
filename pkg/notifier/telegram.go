// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bborbe/errors"
)

type telegramNotifier struct {
	botToken string
	chatID   string
	baseURL  string
}

// NewTelegramNotifier returns a Notifier that sends messages via Telegram.
func NewTelegramNotifier(botToken, chatID string) Notifier {
	return &telegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		baseURL:  "https://api.telegram.org",
	}
}

// NewTelegramNotifierWithBaseURL returns a Notifier that sends messages via Telegram,
// using the given base URL instead of the default api.telegram.org. Useful for testing.
func NewTelegramNotifierWithBaseURL(botToken, chatID, baseURL string) Notifier {
	return &telegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		baseURL:  baseURL,
	}
}

func (t *telegramNotifier) Notify(ctx context.Context, event Event) error {
	message := formatMessage(event)
	payload := map[string]string{
		"chat_id": t.chatID,
		"text":    message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(ctx, err, "marshal telegram payload")
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", t.baseURL, t.botToken)
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(ctx, err, "create telegram request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(ctx, err, "send telegram request")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf(ctx, "telegram request failed with status %d", resp.StatusCode)
	}
	return nil
}

// formatMessage formats an Event as plain text.
func formatMessage(event Event) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🔔 [%s] %s", event.ProjectName, event.EventType)
	if event.PromptName != "" {
		fmt.Fprintf(&sb, "\nPrompt: %s", event.PromptName)
	}
	if event.PRURL != "" {
		fmt.Fprintf(&sb, "\nPR: %s", event.PRURL)
	}
	return sb.String()
}
