package leadershipapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"
)

// Watch delivers service notifications until disconnect or cancellation.
// Connecting alone does not imply that leadership has changed.
func (c *apiClient) Watch(ctx context.Context, notify func(string)) error {
	response, err := c.request(ctx, "text/event-stream", "v1", "events")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "text/event-stream" {
		return fmt.Errorf("leadership API did not return an SSE stream")
	}
	return readEvents(response.Body, notify)
}

// readEvents parses bounded SSE frames, ignoring comments and unrelated event types.
// Only a terminating blank line dispatches a frame; partial frames on disconnect are discarded.
func readEvents(reader io.Reader, notify func(string)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var event string
	var data []string
	size := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(data) > 0 && (event == "" || event == "message" || event == "leadership-changed") {
				var change struct {
					SHA256 string `json:"sha256"`
				}
				if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &change); err != nil {
					return fmt.Errorf("decode leadership event: %w", err)
				}
				if err := validateChecksum(change.SHA256); err != nil {
					return err
				}
				notify(change.SHA256)
			}
			event, data, size = "", nil, 0
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			size += len(value) + 1
			if size > 1024*1024 {
				return fmt.Errorf("leadership event exceeds size limit")
			}
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}
