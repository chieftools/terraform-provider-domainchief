// Package client contains a minimal client for the Domain Chief REST API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the base URL of the Domain Chief API.
const DefaultBaseURL = "https://domain.chief.app/api/v1"

const (
	defaultMaxAttempts  = 6
	defaultRetryBackoff = 2 * time.Second
)

// Client talks to the Domain Chief API.
type Client struct {
	baseURL    string
	token      string
	team       string
	userAgent  string
	httpClient *http.Client

	maxAttempts  int
	retryBackoff time.Duration

	domainLocks sync.Map
}

// Config configures a Client.
type Config struct {
	// BaseURL overrides DefaultBaseURL.
	BaseURL string
	// Token is a Domain Chief API token.
	Token string
	// Team selects the team for tokens that are not scoped to a single team, it is sent as the X-Chief-Team header.
	Team string
	// UserAgent is sent with every request.
	UserAgent string
}

// New creates a client.
func New(cfg Config) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        cfg.Token,
		team:         cfg.Team,
		userAgent:    cfg.UserAgent,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		maxAttempts:  defaultMaxAttempts,
		retryBackoff: defaultRetryBackoff,
	}
}

// APIError is returned when the API responds with an unexpected status code.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Errors     map[string][]string
}

func (e *APIError) Error() string {
	message := e.Message

	if len(e.Errors) > 0 {
		fields := make([]string, 0, len(e.Errors))
		for field := range e.Errors {
			fields = append(fields, field)
		}
		sort.Strings(fields)

		for _, field := range fields {
			message += fmt.Sprintf(" [%s: %s]", field, strings.Join(e.Errors[field], " "))
		}
	}

	if e.Code != "" {
		return fmt.Sprintf("domain chief API error (%d, %s): %s", e.StatusCode, e.Code, message)
	}

	return fmt.Sprintf("domain chief API error (%d): %s", e.StatusCode, message)
}

// IsNotFound reports whether err is an API error with a 404 status.
func IsNotFound(err error) bool {
	var apiErr *APIError

	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// retryable reports whether the request may succeed when sent again.
//
// A conflict is only retried when another write for the same domain is still running,
// not when it is permanent (e.g. the domain does not use hosted DNS).
func (e *APIError) retryable() bool {
	switch e.StatusCode {
	case http.StatusConflict:
		return e.Code == "dns.zone_busy" || e.Code == "dns.zone_changed" || e.Code == "domain.concurrent_request"
	case http.StatusTooManyRequests:
		return true
	default:
		return e.StatusCode >= http.StatusInternalServerError
	}
}

// LockDomain serializes writes to a single domain, the API only accepts one hosted DNS write per domain at a time.
func (c *Client) LockDomain(domain string) func() {
	value, _ := c.domainLocks.LoadOrStore(strings.ToLower(domain), &sync.Mutex{})
	mutex := value.(*sync.Mutex) //nolint:forcetypeassert // only *sync.Mutex values are stored

	mutex.Lock()

	return mutex.Unlock
}

type dataEnvelope[T any] struct {
	Data T `json:"data"`
}

type pageEnvelope[T any] struct {
	Data []T `json:"data"`
	Meta struct {
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	} `json:"meta"`
}

// get decodes the `data` of a single resource response into out.
func get[T any](ctx context.Context, c *Client, path string, query url.Values) (*T, error) {
	var envelope dataEnvelope[T]

	if err := c.do(ctx, http.MethodGet, path, query, nil, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// send performs a write and decodes the `data` of the response.
func send[T any](ctx context.Context, c *Client, method, path string, body any) (*T, error) {
	var envelope dataEnvelope[T]

	if err := c.do(ctx, method, path, nil, body, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// list retrieves every page of a collection.
func list[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("per_page", "100")

	var items []T

	for page := 1; ; page++ {
		query.Set("page", fmt.Sprint(page))

		var envelope pageEnvelope[T]
		if err := c.do(ctx, http.MethodGet, path, query, nil, &envelope); err != nil {
			return nil, err
		}

		items = append(items, envelope.Data...)

		if envelope.Meta.LastPage == 0 || envelope.Meta.CurrentPage >= envelope.Meta.LastPage {
			return items, nil
		}
	}
}

// do sends the request, retrying when another write for the same domain is still running.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte

	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}

	var err error

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		err = c.send(ctx, method, path, query, payload, out)

		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.retryable() || attempt == c.maxAttempts {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * c.retryBackoff):
		}
	}

	return err
}

func (c *Client) send(ctx context.Context, method, path string, query url.Values, payload []byte, out any) error {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)

	if c.team != "" {
		req.Header.Set("X-Chief-Team", c.team)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("domain chief API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseError(resp)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)

		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode domain chief API response: %w", err)
	}

	return nil
}

func parseError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	apiErr := &APIError{StatusCode: resp.StatusCode}

	var decoded struct {
		Message string              `json:"message"`
		Code    string              `json:"code"`
		Errors  map[string][]string `json:"errors"`
	}

	if json.Unmarshal(raw, &decoded) == nil && decoded.Message != "" {
		apiErr.Code = decoded.Code
		apiErr.Message = decoded.Message
		apiErr.Errors = decoded.Errors
	} else {
		apiErr.Message = strings.TrimSpace(string(raw))
	}

	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(resp.StatusCode)
	}

	return apiErr
}

func escape(segment string) string {
	return url.PathEscape(segment)
}
