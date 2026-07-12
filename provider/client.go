package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL  = "https://sportarr.net"
	maxRetries      = 3
	maxResponseBody = 2 << 20 // 2 MB
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	limiter    *rate.Limiter
}

// ErrNotFound reports a 404 from Sportarr without treating other 4xx responses
// as a missing resource.
type ErrNotFound struct {
	URL string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("sportarr: not found: %s", e.URL)
}

func NewClient(rateLimit int) *Client {
	if rateLimit <= 0 {
		rateLimit = 10
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    defaultBaseURL,
		limiter:    rate.NewLimiter(rate.Limit(rateLimit), rateLimit),
	}
}

func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

func (c *Client) localMovieAPIConfigured() bool {
	endpoint, err := url.Parse(c.baseURL)
	if err != nil || endpoint.Hostname() == "" {
		return false
	}

	scheme := strings.ToLower(endpoint.Scheme)
	host := strings.TrimSuffix(strings.ToLower(endpoint.Hostname()), ".")
	effectivePort := endpoint.Port()
	if effectivePort == "" {
		switch scheme {
		case "http":
			effectivePort = "80"
		case "https":
			effectivePort = "443"
		}
	}

	// The Movie agent API is instance-local, never the public Sportarr hub.
	// Reject its hostname regardless of HTTP scheme or explicit/default port.
	if (scheme == "http" || scheme == "https") && host == "sportarr.net" && effectivePort != "" {
		return false
	}
	return true
}

func (c *Client) doGet(ctx context.Context, path string, dest any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}

	reqURL := c.baseURL + path

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("sportarr: create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "silo-plugin-sportarr/1.0")
		req.Header.Set("Cache-Control", "no-cache, no-store")
		req.Header.Set("Pragma", "no-cache")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("sportarr: request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt < maxRetries {
				backoff := retryAfterOrDefault(resp, attempt)
				slog.Warn("sportarr: rate limited, backing off",
					"path", path, "attempt", attempt+1, "backoff", backoff.String())
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			return fmt.Errorf("sportarr: rate limited after %d retries", maxRetries)
		}

		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			if attempt < maxRetries {
				backoff := time.Duration(1<<attempt) * time.Second
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			return fmt.Errorf("sportarr: server error %d after %d retries", resp.StatusCode, maxRetries)
		}

		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return &ErrNotFound{URL: reqURL}
		}

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
			resp.Body.Close()
			return fmt.Errorf("sportarr: HTTP %d: %s", resp.StatusCode, string(body))
		}

		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(dest)
		resp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("sportarr: decode response: %w", decodeErr)
		}
		return nil
	}
	return fmt.Errorf("sportarr: max retries exceeded")
}

func retryAfterOrDefault(resp *http.Response, attempt int) time.Duration {
	if val := resp.Header.Get("Retry-After"); val != "" {
		if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(1<<attempt) * time.Second
}

func (c *Client) Search(ctx context.Context, title string) (*AgentSearchResponse, error) {
	path := "/api/metadata/agents/search?title=" + url.QueryEscape(title)
	var resp AgentSearchResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) SearchMovies(ctx context.Context, title string, year int) (*AgentMovieSearchResponse, error) {
	path := "/api/metadata/agents/movies/search?" + url.Values{
		"title": []string{title},
		"year":  []string{strconv.Itoa(year)},
	}.Encode()
	var resp AgentMovieSearchResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetMovie(ctx context.Context, providerID string) (*AgentMovieResponse, error) {
	path := "/api/metadata/agents/movies/" + url.PathEscape(providerID)
	var resp AgentMovieResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetSeries(ctx context.Context, leagueID string) (*AgentSeriesResponse, error) {
	path := "/api/metadata/agents/series/" + url.PathEscape(leagueID)
	var resp AgentSeriesResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetSeasons(ctx context.Context, leagueID string) (*AgentSeasonsResponse, error) {
	path := "/api/metadata/agents/series/" + url.PathEscape(leagueID) + "/seasons"
	var resp AgentSeasonsResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetSeasonEpisodes(ctx context.Context, leagueID string, seasonNumber int) (*AgentEpisodesResponse, error) {
	path := fmt.Sprintf("/api/metadata/agents/series/%s/season/%d/episodes",
		url.PathEscape(leagueID), seasonNumber)
	var resp AgentEpisodesResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetEntityImages(ctx context.Context, entityType, entityID string) (*EntityImageResponse, error) {
	path := "/api/v1/images/entity/" + entityType + "/" + url.PathEscape(entityID) + "?completed_only=true"
	var resp EntityImageResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetEntityImagesBatch(ctx context.Context, entityType string, entityIDs []string) map[string]*EntityImageResponse {
	type result struct {
		id   string
		resp *EntityImageResponse
	}

	seen := make(map[string]struct{}, len(entityIDs))
	var unique []string
	for _, id := range entityIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}

	ch := make(chan result, len(unique))
	for _, id := range unique {
		go func(eid string) {
			resp, err := c.GetEntityImages(ctx, entityType, eid)
			if err != nil {
				ch <- result{id: eid}
				return
			}
			ch <- result{id: eid, resp: resp}
		}(id)
	}

	out := make(map[string]*EntityImageResponse, len(unique))
	for range unique {
		r := <-ch
		if r.resp != nil {
			out[r.id] = r.resp
		}
	}
	return out
}
