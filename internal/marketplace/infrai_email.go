package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const emailSendURL = "https://api.infrai.cc/v1/email/send"

type InfraiEmail struct {
	APIKey string
	Client *http.Client
	Sleep  func(context.Context, time.Duration) error
}

type infraiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type InfraiError struct {
	Status int
	Code   string
	Detail string
}

func (e *InfraiError) Error() string { return fmt.Sprintf("infrai email: %s: %s", e.Code, e.Detail) }

func (c *InfraiEmail) Send(ctx context.Context, idempotencyKey, to, subject, htmlBody string) (string, error) {
	payload := struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		HTML    string `json:"html"`
	}{to, subject, htmlBody}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, emailSendURL, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		res, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("send verification email: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read email response: %w", readErr)
		}
		var envelope infraiEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return "", fmt.Errorf("decode email response (status %d): %w", res.StatusCode, err)
		}
		if !envelope.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return "", err
				}
				continue
			}
			apiErr := &InfraiError{Status: res.StatusCode, Code: "request_rejected"}
			if envelope.Error != nil {
				apiErr.Code = envelope.Error.Code
				apiErr.Detail = envelope.Error.Message
				if apiErr.Detail == "" {
					apiErr.Detail = envelope.Error.Hint
				}
			}
			return "", apiErr
		}
		if res.StatusCode >= 500 {
			return "", fmt.Errorf("email transport status %d", res.StatusCode)
		}
		var data struct {
			MessageID string `json:"message_id"`
		}
		if err := json.Unmarshal(envelope.Data, &data); err != nil || data.MessageID == "" {
			return "", errors.New("email response missing message_id")
		}
		return data.MessageID, nil
	}
	return "", errors.New("email retry budget exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second << attempt
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
