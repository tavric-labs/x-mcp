// Package x is a small client for the two X API calls this server makes on
// behalf of a signed-in person: reading their profile and publishing a post.
// It never stores a token; the caller passes one per call.
package x

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnauthorized means X rejected the person's token: it expired or was
// revoked. The person has to sign in again.
var ErrUnauthorized = errors.New("x rejected the member's token")

// ErrPaymentRequired means X refused the call because the developer app has
// no API credits (pay-per-use).
var ErrPaymentRequired = errors.New("x api credits are required")

// Profile is the authenticated user X returns from /2/users/me.
type Profile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// Client calls the X API. BaseURL is injectable so tests can point at another host.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for the API at baseURL (for example https://api.x.com).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 15 * time.Second,
			// A token must never follow a redirect to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("x api must not redirect") },
		},
	}
}

// Profile returns the person the token belongs to.
func (c *Client) Profile(ctx context.Context, token string) (Profile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/2/users/me?user.fields=name,username", nil)
	if err != nil {
		return Profile{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(request)
	if err != nil {
		return Profile{}, fmt.Errorf("x users/me: %w", err)
	}
	defer response.Body.Close()
	if err := statusError(response); err != nil {
		return Profile{}, err
	}
	var body struct {
		Data Profile `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return Profile{}, fmt.Errorf("decode x users/me: %w", err)
	}
	if body.Data.ID == "" {
		return Profile{}, errors.New("x users/me has no user id")
	}
	return body.Data, nil
}

// CreatePost publishes text as the signed-in person and returns the new post id.
func (c *Client) CreatePost(ctx context.Context, token, text string) (string, error) {
	payload, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/2/tweets", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return "", fmt.Errorf("x create post: %w", err)
	}
	defer response.Body.Close()
	if err := statusError(response); err != nil {
		return "", err
	}
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return "", fmt.Errorf("decode x create post: %w", err)
	}
	if body.Data.ID == "" {
		return "", errors.New("x create post returned no post id")
	}
	return body.Data.ID, nil
}

// statusError turns a non-2xx response into an error that carries only the
// status, never X's response body, which can echo request details.
func statusError(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<16))
	switch response.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusPaymentRequired:
		return ErrPaymentRequired
	}
	return fmt.Errorf("x returned HTTP %d", response.StatusCode)
}
