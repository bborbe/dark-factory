// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package formatter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/formatter.go --fake-name StreamFormatter . Formatter

// Formatter processes a Claude Code stream-json stdout stream.
type Formatter interface {
	// ProcessStream reads newline-delimited stream-json from r until EOF or ctx cancellation.
	// For each line it writes the raw bytes (including the trailing newline) to rawWriter,
	// and a formatted human-readable line (with timestamp prefix, trailing newline) to formattedWriter.
	// Lines that are not valid JSON are written verbatim to both outputs.
	// ProcessStream returns nil on clean EOF. It returns a non-nil error only when rawWriter
	// or formattedWriter return a write error.
	ProcessStream(
		ctx context.Context,
		r io.Reader,
		rawWriter io.Writer,
		formattedWriter io.Writer,
	) error
}

// NewFormatter creates a new Formatter.
func NewFormatter() Formatter {
	return &streamFormatter{}
}

type streamFormatter struct{}

// ProcessStream reads newline-delimited stream-json and writes raw and formatted output.
func (f *streamFormatter) ProcessStream(
	ctx context.Context,
	r io.Reader,
	rawWriter io.Writer,
	formattedWriter io.Writer,
) error {
	toolNames := make(map[string]string)

	scanner := bufio.NewScanner(r)
	const maxScannerBuf = 10 * 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScannerBuf)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := f.processLine(ctx, scanner.Bytes(), rawWriter, formattedWriter, toolNames); err != nil {
			return err
		}
	}

	return f.handleScannerError(ctx, scanner.Err(), rawWriter, formattedWriter)
}

// processLine handles a single scanned line: writes it raw, then parses and formats it.
func (f *streamFormatter) processLine(
	ctx context.Context,
	line []byte,
	rawWriter io.Writer,
	formattedWriter io.Writer,
	toolNames map[string]string,
) error {
	if _, err := rawWriter.Write(line); err != nil {
		return errors.Wrap(ctx, err, "write raw log")
	}
	if _, err := rawWriter.Write([]byte{'\n'}); err != nil {
		return errors.Wrap(ctx, err, "write raw log newline")
	}

	var msg StreamMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		formatted := formatTimestamp() + " " + string(line) + "\n"
		if _, err2 := formattedWriter.Write([]byte(formatted)); err2 != nil {
			return errors.Wrap(ctx, err2, "write formatted fallback")
		}
		return nil
	}

	rendered := f.renderMessage(msg, toolNames)
	if rendered == "" {
		return nil
	}
	if _, err := formattedWriter.Write([]byte(rendered)); err != nil {
		return errors.Wrap(ctx, err, "write formatted output")
	}
	return nil
}

// handleScannerError writes a truncation marker to both outputs when the scanner reports an error.
func (f *streamFormatter) handleScannerError(
	ctx context.Context,
	scanErr error,
	rawWriter io.Writer,
	formattedWriter io.Writer,
) error {
	if scanErr == nil {
		return nil
	}
	marker := fmt.Sprintf("[formatter: scanner error, line truncated or dropped: %v]\n", scanErr)
	if _, werr := rawWriter.Write([]byte(marker)); werr != nil {
		return errors.Wrap(ctx, werr, "write truncation marker to raw log")
	}
	if _, werr := formattedWriter.Write([]byte(formatTimestamp() + " " + marker)); werr != nil {
		return errors.Wrap(ctx, werr, "write truncation marker to formatted log")
	}
	slog.Warn("formatter scanner error", "error", scanErr)
	return nil
}

// formatTimestamp returns the current local time formatted as [HH:MM:SS].
func formatTimestamp() string {
	return time.Now().Format("[15:04:05]")
}

func (f *streamFormatter) renderMessage(msg StreamMessage, toolNames map[string]string) string {
	switch msg.Type {
	case "system":
		return renderSystemInit(msg)
	case "assistant":
		return renderAssistant(msg, toolNames)
	case "user":
		return renderUser(msg, toolNames)
	case "result":
		return renderResult(msg)
	default:
		return formatTimestamp() + " [unknown type: " + msg.Type + "]\n"
	}
}
