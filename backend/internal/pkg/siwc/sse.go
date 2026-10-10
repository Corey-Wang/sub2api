package siwc

import (
	"bytes"
	"encoding/json"
	"strings"
)

// AggregateResponsesSSE turns a Responses SSE stream into the final response
// object. response.failed is returned as an error that still carries the code.
// PassthroughSSE returns the raw stream unchanged for clients that asked to stream.
func PassthroughSSE(raw []byte) []byte {
	return append([]byte(nil), raw...)
}

// AggregatedSSE is the non-streaming projection of a Responses SSE body.
type AggregatedSSE struct {
	Body       []byte
	Failed     bool
	Code       string
	HTTPStatus int
}

// AggregateResponsesSSE extracts the response.completed response object.
func AggregateResponsesSSE(raw []byte) (*AggregatedSSE, error) {
	frames := parseSSE(raw)
	var completed []byte
	var failed []byte
	for _, frame := range frames {
		eventType := frame.event
		if eventType == "" {
			eventType = strings.TrimSpace(jsonString(frame.data, "type"))
		}
		switch eventType {
		case "response.completed", "response.done":
			completed = frame.data
		case "response.failed", "error":
			failed = frame.data
		}
	}
	if failed != nil && completed == nil {
		code := firstJSONString(failed, "response.error.code", "error.code", "code")
		action := ClassifyUpstreamError(0, code)
		status := action.HTTPStatus
		if status == 0 {
			status = 502
		}
		return &AggregatedSSE{Body: append([]byte(nil), failed...), Failed: true, Code: code, HTTPStatus: status}, &OAuthError{Code: code, Description: "upstream response.failed", HTTPStatus: status}
	}
	if completed == nil {
		return nil, &OAuthError{Code: "invalid_sse", Description: "SSE stream had no response.completed event"}
	}
	response := json.RawMessage(completed)
	if rawResponse := jsonRaw(completed, "response"); len(rawResponse) > 0 && rawResponse[0] == '{' {
		response = rawResponse
	}
	return &AggregatedSSE{Body: append([]byte(nil), response...), HTTPStatus: 200}, nil
}

type sseFrame struct {
	event string
	data  []byte
}

func parseSSE(raw []byte) []sseFrame {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	blocks := strings.Split(text, "\n\n")
	frames := make([]sseFrame, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, ":") {
			continue
		}
		var event string
		var data bytes.Buffer
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event:") {
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if data.Len() == 0 {
			continue
		}
		frames = append(frames, sseFrame{event: event, data: data.Bytes()})
	}
	return frames
}

func jsonString(body []byte, path string) string {
	return firstJSONString(body, path)
}

func firstJSONString(body []byte, paths ...string) string {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return ""
	}
	for _, path := range paths {
		if value, ok := walkJSON(doc, path).(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func jsonRaw(body []byte, path string) json.RawMessage {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil
	}
	value := walkJSON(doc, path)
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func walkJSON(doc any, path string) any {
	cur := doc
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[part]
	}
	return cur
}
