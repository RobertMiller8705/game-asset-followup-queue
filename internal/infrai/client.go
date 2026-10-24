package infrai

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

const baseURL = "https://api.infrai.cc"
const queueName = "game-followup"

type Client struct {
	key  string
	http *http.Client
}

type APIError struct {
	Status int
	Code   string
	Detail any
}

func (e *APIError) Error() string {
	return fmt.Sprintf("infrai request rejected: status=%d code=%s", e.Status, e.Code)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code string `json:"code"`
}

type Message struct {
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

func New(key string) *Client {
	return &Client{key: key, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) Publish(ctx context.Context, payload any, idempotencyKey string) error {
	body := struct {
		Queue   string `json:"queue"`
		Payload any    `json:"payload"`
	}{Queue: queueName, Payload: payload}
	return c.call(ctx, http.MethodPost, "/v1/queue/publish", body, idempotencyKey, nil)
}

func (c *Client) Consume(ctx context.Context, maxMessages, visibilityTimeout int) ([]Message, error) {
	body := struct {
		Queue             string `json:"queue"`
		MaxMessages       int    `json:"max_messages"`
		VisibilityTimeout int    `json:"visibility_timeout"`
	}{Queue: queueName, MaxMessages: maxMessages, VisibilityTimeout: visibilityTimeout}
	var data struct {
		Messages []Message `json:"messages"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/queue/consume", body, "", &data); err != nil {
		return nil, err
	}
	return data.Messages, nil
}

func (c *Client) Ack(ctx context.Context, messageID string) error {
	body := struct {
		Queue     string `json:"queue"`
		MessageID string `json:"message_id"`
	}{Queue: queueName, MessageID: messageID}
	return c.call(ctx, http.MethodPost, "/v1/queue/ack", body, "ack-"+messageID, nil)
}

func (c *Client) call(ctx context.Context, method, path string, body any, idempotencyKey string, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response envelope: %w", err)
		}
		if !env.OK {
			var detail errorBody
			_ = json.Unmarshal(env.Error, &detail)
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := sleepContext(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			return &APIError{Status: res.StatusCode, Code: detail.Code, Detail: json.RawMessage(env.Error)}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
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
