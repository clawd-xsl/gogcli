package gmailwatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"unicode/utf8"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type HookSender struct {
	URL                string
	Token              string
	Client             HTTPDoer
	MaxPayloadBytes    int
	MaxPayloadMessages int
}

const hookBodyPreviewBytes = 3000

type HookPayloadTooLargeError struct {
	Size  int
	Limit int
}

func (e *HookPayloadTooLargeError) Error() string {
	return fmt.Sprintf("hook payload is %d bytes, limit is %d", e.Size, e.Limit)
}

type HookStatusError struct {
	StatusCode int
}

func (e *HookStatusError) Error() string {
	return fmt.Sprintf("hook status %d", e.StatusCode)
}

func (s *HookSender) Send(ctx context.Context, payload *Payload) DeliveryResult {
	data, err := marshalHookPayload(payload, s.MaxPayloadBytes, s.MaxPayloadMessages)
	if err != nil {
		return DeliveryResult{Err: fmt.Errorf("encode hook payload: %w", err)}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(data))
	if err != nil {
		return DeliveryResult{Err: fmt.Errorf("create hook request: %w", err)}
	}

	request.Header.Set("Content-Type", "application/json")

	if s.Token != "" {
		request.Header.Set("Authorization", "Bearer "+s.Token)
	}

	response, err := s.Client.Do(request)
	if err != nil {
		return DeliveryResult{
			Status: DeliveryStatusError,
			Note:   err.Error(),
			Err:    err,
			Record: true,
		}
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		note := fmt.Sprintf("status %d", response.StatusCode)

		return DeliveryResult{
			Status: DeliveryStatusHTTPError,
			Note:   note,
			Err:    &HookStatusError{StatusCode: response.StatusCode},
			Record: true,
		}
	}

	return DeliveryResult{
		Status: DeliveryStatusOK,
		Record: true,
	}
}

func marshalHookPayload(payload *Payload, maxBytes, maxMessages int) ([]byte, error) {
	bounded := cloneHookPayload(payload)
	if maxMessages > 0 && len(bounded.Messages) > maxMessages {
		bounded.Messages = bounded.Messages[len(bounded.Messages)-maxMessages:]
	}

	data, err := json.Marshal(bounded)
	if err != nil {
		return nil, fmt.Errorf("marshal bounded hook payload: %w", err)
	}

	if maxBytes <= 0 || len(data) <= maxBytes {
		return data, nil
	}

	for i := range bounded.Messages {
		body, truncated := truncateHookBody(bounded.Messages[i].Body, hookBodyPreviewBytes)
		if truncated {
			bounded.Messages[i].Body = body + "\n[truncated by gog]"
			bounded.Messages[i].BodyTruncated = true
		}
	}

	data, err = json.Marshal(bounded)
	if err != nil {
		return nil, fmt.Errorf("marshal body-truncated hook payload: %w", err)
	}

	if len(data) <= maxBytes {
		return data, nil
	}

	for i := range bounded.Messages {
		if bounded.Messages[i].Body == "" {
			continue
		}
		bounded.Messages[i].Body = "[body dropped: payload too large]"
		bounded.Messages[i].BodyTruncated = true
	}

	data, err = json.Marshal(bounded)
	if err != nil {
		return nil, fmt.Errorf("marshal body-free hook payload: %w", err)
	}

	for len(data) > maxBytes && len(bounded.Messages) > 1 {
		bounded.Messages = bounded.Messages[1:]

		data, err = json.Marshal(bounded)
		if err != nil {
			return nil, fmt.Errorf("marshal message-trimmed hook payload: %w", err)
		}
	}

	if len(data) > maxBytes {
		return nil, &HookPayloadTooLargeError{Size: len(data), Limit: maxBytes}
	}

	return data, nil
}

func cloneHookPayload(payload *Payload) *Payload {
	if payload == nil {
		return &Payload{}
	}

	cloned := *payload

	cloned.Messages = append([]Message(nil), payload.Messages...)
	for i := range cloned.Messages {
		cloned.Messages[i].Labels = append([]string(nil), payload.Messages[i].Labels...)
	}

	cloned.DeletedMessageIDs = append([]string(nil), payload.DeletedMessageIDs...)

	return &cloned
}

func truncateHookBody(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}

	data := []byte(value)[:maxBytes]
	for !utf8.Valid(data) {
		data = data[:len(data)-1]
	}

	return string(data), true
}
