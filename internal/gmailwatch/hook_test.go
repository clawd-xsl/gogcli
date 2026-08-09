package gmailwatch

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type hookDoer func(*http.Request) (*http.Response, error)

func (f hookDoer) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHookSenderSuccess(t *testing.T) {
	t.Parallel()

	sender := &HookSender{
		URL:   "https://example.com/hook",
		Token: "secret",
		Client: hookDoer(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("Authorization") != "Bearer secret" {
				t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
			}

			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	result := sender.Send(context.Background(), &Payload{HistoryID: "200"})
	if result.Err != nil || result.Status != DeliveryStatusOK || !result.Record {
		t.Fatalf("result = %#v", result)
	}
}

func TestHookSenderSignsPayload(t *testing.T) {
	t.Parallel()

	const secret = "signing-secret"
	sender := &HookSender{
		URL:        "https://example.com/hook",
		HMACSecret: secret,
		Client: hookDoer(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			signature := hmac.New(sha256.New, []byte(secret))
			_, _ = signature.Write(body)
			want := "sha256=" + fmt.Sprintf("%x", signature.Sum(nil))

			if got := request.Header.Get("X-Hub-Signature-256"); got != want {
				t.Fatalf("signature = %q, want %q", got, want)
			}

			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	result := sender.Send(context.Background(), &Payload{HistoryID: "200"})
	if result.Err != nil || result.Status != DeliveryStatusOK || !result.Record {
		t.Fatalf("result = %#v", result)
	}
}

func TestHookSenderClassifiesFailures(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("dial failed") //nolint:err113 // Test-only transport failure.
	sender := &HookSender{
		URL: "https://example.com/hook",
		Client: hookDoer(func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		}),
	}

	result := sender.Send(context.Background(), &Payload{})
	if !errors.Is(result.Err, transportErr) || result.Status != DeliveryStatusError || !result.Record {
		t.Fatalf("transport result = %#v", result)
	}

	sender.Client = hookDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	result = sender.Send(context.Background(), &Payload{})

	var statusErr *HookStatusError
	if !errors.As(result.Err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway ||
		result.Status != DeliveryStatusHTTPError || result.Note != "status 502" || !result.Record {
		t.Fatalf("HTTP result = %#v", result)
	}
}

func TestHookSenderBoundsPayloadAndKeepsNewestMessages(t *testing.T) {
	t.Parallel()

	payload := &Payload{
		Messages: []Message{
			{ID: "m1", Body: strings.Repeat("old", 2000)},
			{ID: "m2", Body: strings.Repeat("two", 2000)},
			{ID: "m3", Body: strings.Repeat("three", 2000)},
			{ID: "m4", Body: strings.Repeat("new", 2000)},
		},
	}
	var encodedSize int
	var delivered Payload
	sender := &HookSender{
		URL:                "https://example.com/hook",
		MaxPayloadBytes:    1024,
		MaxPayloadMessages: 3,
		Client: hookDoer(func(request *http.Request) (*http.Response, error) {
			data, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			encodedSize = len(data)
			if err := json.Unmarshal(data, &delivered); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
	}

	result := sender.Send(context.Background(), payload)
	if result.Err != nil {
		t.Fatalf("Send: %v", result.Err)
	}

	if encodedSize > sender.MaxPayloadBytes {
		t.Fatalf("encoded size = %d", encodedSize)
	}

	if len(delivered.Messages) != 3 || delivered.Messages[0].ID != "m2" || delivered.Messages[2].ID != "m4" {
		t.Fatalf("messages = %#v", delivered.Messages)
	}

	if !delivered.Messages[0].BodyTruncated {
		t.Fatalf("message was not truncated: %#v", delivered.Messages[0])
	}

	if len(payload.Messages) != 4 || payload.Messages[0].BodyTruncated {
		t.Fatalf("input payload mutated: %#v", payload)
	}
}

func TestHookSenderRejectsPayloadThatCannotFit(t *testing.T) {
	t.Parallel()

	called := false
	sender := &HookSender{
		URL:             "https://example.com/hook",
		MaxPayloadBytes: 64,
		Client: hookDoer(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected hook call") //nolint:err113 // Test-only failure.
		}),
	}
	result := sender.Send(context.Background(), &Payload{
		Messages: []Message{{ID: "m1", Subject: strings.Repeat("x", 256)}},
	})

	var sizeErr *HookPayloadTooLargeError
	if !errors.As(result.Err, &sizeErr) || sizeErr.Limit != 64 {
		t.Fatalf("result = %#v", result)
	}

	if called {
		t.Fatal("oversized payload was sent")
	}
}
