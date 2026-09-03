package dataprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	apiTokenHeader = "x-api-token"
	defaultTimeout = 10 * time.Second
)

// ErrNilConfig is returned by NewClient when no configuration is supplied.
var ErrNilConfig = errors.New("dataprovider: config must not be nil")

// ErrEmptyAPIToken is returned by NewClient when the config's APIToken is empty.
var ErrEmptyAPIToken = errors.New("dataprovider: APIToken must not be empty")

// Client makes HTTP requests to the data provider API, authenticating every
// outbound request with the x-api-token header.
type Client struct {
	baseURL    *url.URL
	config     *Config
	httpClient *http.Client
}

// NewClient creates a data provider client from the given configuration.
// It returns an error if config is nil or its BaseURL cannot be parsed.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	if config.APIToken == "" {
		return nil, ErrEmptyAPIToken
	}

	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("dataprovider: invalid base URL %q: %w", config.BaseURL, err)
	}

	if !baseURL.IsAbs() || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Hostname() == "" {
		return nil, fmt.Errorf("dataprovider: base URL %q must be an absolute http(s) URL", config.BaseURL)
	}

	return &Client{
		baseURL: baseURL,
		config:  config,
		httpClient: &http.Client{
			Timeout:       defaultTimeout,
			CheckRedirect: rejectCrossOriginRedirect,
		},
	}, nil
}

// rejectCrossOriginRedirect stops the client from following a redirect to a
// different host than the original request, so the x-api-token header
// (which net/http forwards on same-origin redirects) can never leak to
// another host.
func rejectCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}

	origin := via[0].URL
	if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
		return fmt.Errorf("dataprovider: refusing cross-origin redirect from %s to %s", origin, req.URL)
	}

	return nil
}

// Do sends an HTTP request to a data provider endpoint. path is resolved
// against the configured DATA_PROVIDER_BASE_URL using net/url, and the
// x-api-token header is set from DATA_PROVIDER_API_TOKEN on every request.
// ctx bounds the request's lifetime; the client additionally enforces its
// own fixed timeout regardless of ctx.
func (c *Client) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	requestURL, err := c.resolve(path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(apiTokenHeader, c.config.APIToken)

	return c.httpClient.Do(req)
}

// resolve joins path onto the configured base URL using net/url, so the
// result is always a well-formed URL rather than an ad-hoc string join.
func (c *Client) resolve(path string) (*url.URL, error) {
	ref, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("dataprovider: invalid path %q: %w", path, err)
	}

	joined, err := url.JoinPath(c.baseURL.String(), ref.Path)
	if err != nil {
		return nil, fmt.Errorf("dataprovider: failed to join base URL with path %q: %w", path, err)
	}

	result, err := url.Parse(joined)
	if err != nil {
		return nil, fmt.Errorf("dataprovider: failed to parse resolved URL %q: %w", joined, err)
	}

	result.RawQuery = ref.RawQuery
	return result, nil
}
