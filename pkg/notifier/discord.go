// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bborbe/errors"
)

// NewDiscordNotifier returns a Notifier that sends messages via Discord webhook.
func NewDiscordNotifier(webhookURL string) Notifier {
	return &discordNotifier{webhookURL: webhookURL}
}

type discordNotifier struct {
	webhookURL string
}

func (d *discordNotifier) Notify(ctx context.Context, event Event) error {
	message := formatMessage(event)
	payload := map[string]string{
		"content": message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(ctx, err, "marshal discord payload")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodPost,
		d.webhookURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return errors.Wrap(ctx, err, "create discord request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(ctx, err, "send discord request")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf(ctx, "discord request failed with status %d", resp.StatusCode)
	}
	return nil
}
