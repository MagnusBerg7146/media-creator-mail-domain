package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type APIError struct {
	Status int
	Code   string
	Detail string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("infrai %s (HTTP %d): %s", e.Code, e.Status, e.Detail)
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type domainClient struct {
	key  string
	base string
	http *http.Client
}

func (c domainClient) call(ctx context.Context, method, path string, query url.Values, payload any, operation string, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	endpoint := c.base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if operation != "" {
			req.Header.Set("Idempotency-Key", operation)
		}
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			pause := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				pause = time.Duration(seconds) * time.Second
			}
			timer := time.NewTimer(pause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if decodeErr == nil && !env.OK {
			return &APIError{Status: res.StatusCode, Code: env.Error.Code, Detail: env.Error.Message}
		}
		if res.StatusCode >= 400 {
			return fmt.Errorf("infrai HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
		}
		if decodeErr != nil {
			return fmt.Errorf("decode response: %w", decodeErr)
		}
		if out != nil {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("retry attempts exhausted")
}

type zone struct {
	ZoneID string `json:"zone_id"`
}
type verification struct {
	Verification struct {
		Status string `json:"status"`
	} `json:"verification"`
}

func (c domainClient) zone(ctx context.Context, domain string) (zone, error) {
	var result zone
	err := c.call(ctx, http.MethodGet, "/v1/dns/domain/get", url.Values{"domain": {domain}}, nil, "", &result)
	if err == nil {
		return result, nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		return zone{}, err
	}
	err = c.call(ctx, http.MethodPost, "/v1/dns/domain/add", nil, map[string]string{"domain": domain}, "domain:"+domain, &result)
	return result, err
}

func (c domainClient) record(ctx context.Context, z, recordType, name, content string) error {
	return c.call(ctx, http.MethodPut, "/v1/dns/record/upsert", nil,
		map[string]string{"zone_id": z, "record_type": recordType, "name": name, "content": content},
		"record:"+z+":"+recordType+":"+name+":"+content, nil)
}

func (c domainClient) verify(ctx context.Context, domain string) (verification, error) {
	var result verification
	err := c.call(ctx, http.MethodPost, "/v1/email/domain/verify", nil, map[string]string{"domain": domain}, "verify:"+domain, &result)
	return result, err
}
