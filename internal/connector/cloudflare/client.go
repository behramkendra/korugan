package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://api.cloudflare.com/client/v4"
	// Cloudflare's documented REST budget is 1200 req / 5 min / user;
	// we self-limit well below it.
	rateWindow = 5 * time.Minute
	rateMax    = 900
)

// client wraps Cloudflare REST + GraphQL with auth, rate limiting and
// retry/backoff. HTTP transport is injectable for fixture tests.
type client struct {
	baseURL string
	token   string
	http    *http.Client

	mu     sync.Mutex
	stamps []time.Time // sliding-window rate limiter
}

func newClient(token, baseURL string, hc *http.Client) *client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &client{baseURL: baseURL, token: token, http: hc}
}

func (c *client) wait(ctx context.Context) error {
	for {
		c.mu.Lock()
		now := time.Now()
		keep := c.stamps[:0]
		for _, s := range c.stamps {
			if now.Sub(s) < rateWindow {
				keep = append(keep, s)
			}
		}
		c.stamps = keep
		if len(c.stamps) < rateMax {
			c.stamps = append(c.stamps, now)
			c.mu.Unlock()
			return nil
		}
		oldest := c.stamps[0]
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rateWindow - now.Sub(oldest) + 50*time.Millisecond):
		}
	}
}

// envelope is Cloudflare's standard REST response wrapper.
type envelope struct {
	Success    bool            `json:"success"`
	Errors     []apiError      `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo *resultInfo     `json:"result_info"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type resultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

// get performs a REST GET with retry/backoff, decoding into the envelope.
func (c *client) get(ctx context.Context, path string) (*envelope, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *client) do(ctx context.Context, method, path string, body []byte) (*envelope, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("cloudflare %s %s: status %d (attempt %d)", method, path, resp.StatusCode, attempt+1)
			continue
		}
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("cloudflare %s %s: decode: %w", method, path, err)
		}
		if !env.Success {
			// 4xx with structured errors: not retryable, surface codes only
			// (messages can echo request content; keep them, tokens never appear).
			return nil, fmt.Errorf("cloudflare %s %s: api errors %v", method, path, env.Errors)
		}
		return &env, nil
	}
	return nil, lastErr
}

// gql performs a GraphQL Analytics query.
func (c *client) gql(ctx context.Context, query string, vars map[string]any) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return nil, err
	}
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloudflare graphql: status %d", resp.StatusCode)
	}
	var out struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("cloudflare graphql decode: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("cloudflare graphql: %s", out.Errors[0].Message)
	}
	return out.Data, nil
}
