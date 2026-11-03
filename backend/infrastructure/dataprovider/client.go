package dataprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

const (
	// apiKeyHeader carries the configured machine credential on every provider
	// request. See docs/organization-snapshot-contract.md: the transport
	// contract fixes both the header name and the snapshot endpoint, so a
	// conforming provider is reachable with only a base URL and a token.
	apiKeyHeader = "x-api-key"

	// acceptJSON is sent on every provider request; the contract defines a JSON
	// response for all provider endpoints.
	acceptJSON = "application/json"

	// defaultTimeout bounds a single provider request. Snapshots run to a few
	// MB for a large organization, so this is sized for a full snapshot fetch
	// rather than a small API call.
	defaultTimeout = 30 * time.Second
)

// ErrNilConfig is returned by NewClient when no configuration is supplied.
var ErrNilConfig = errors.New("dataprovider: config must not be nil")

// ErrEmptyAPIToken is returned by NewClient when the config's APIToken is empty.
var ErrEmptyAPIToken = errors.New("dataprovider: APIToken must not be empty")

// ErrRequestFailed is returned by Do when the underlying transport fails. The
// raw transport error is neither returned nor logged: net/http wraps failures
// in *url.Error, whose message carries the full request URL (including any
// userinfo or query string).
var ErrRequestFailed = errors.New("dataprovider: request failed")

// ErrInvalidBaseURL is returned for a base URL that cannot be parsed or is not
// an absolute http(s) URL with a hostname. The message deliberately omits the
// offending value: it may embed credentials.
var ErrInvalidBaseURL = errors.New("dataprovider: base URL must be an absolute http(s) URL")

// ErrBaseURLHasCredentials is returned for a base URL with userinfo
// (https://user:pass@host). The only supported credential is the x-api-key
// token, so embedded credentials are rejected rather than silently sent.
var ErrBaseURLHasCredentials = errors.New("dataprovider: base URL must not contain credentials")

// errCrossOriginRedirect is returned by the client's redirect policy. It
// carries no URLs for the same reason ErrRequestFailed does.
var errCrossOriginRedirect = errors.New("dataprovider: refusing cross-origin redirect")

// Error categories logged for a failed request. Only these fixed values are
// ever logged, never the underlying error text.
const (
	failureTimeout       = "timeout"
	failureCanceled      = "canceled"
	failureRedirect      = "cross_origin_redirect"
	failureTransport     = "transport"
	requestFailedLogText = "dataprovider: request failed"
)

// Client makes HTTP requests to the data provider API, authenticating every
// outbound request with the x-api-key header.
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

	if strings.TrimSpace(config.APIToken) == "" {
		return nil, ErrEmptyAPIToken
	}

	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
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

// parseBaseURL parses and validates a data provider base URL: it must be an
// absolute http(s) URL with a hostname. NewClient uses this to build its
// client, and ValidateBaseURL exposes the same check to callers (the
// settings handler) that need to validate a base URL without an API token.
func parseBaseURL(raw string) (*url.URL, error) {
	// url.Parse's own error echoes the input, so it is dropped here.
	baseURL, err := url.Parse(raw)
	if err != nil {
		return nil, ErrInvalidBaseURL
	}
	if !baseURL.IsAbs() || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Hostname() == "" {
		return nil, ErrInvalidBaseURL
	}
	if baseURL.User != nil {
		return nil, ErrBaseURLHasCredentials
	}
	return baseURL, nil
}

// ValidateBaseURL reports whether raw would be accepted by NewClient as a
// base URL. It performs the same check without requiring an API token, so
// the settings handler can report DATA_PROVIDER_BASE_URL's validity on its
// own.
func ValidateBaseURL(raw string) error {
	_, err := parseBaseURL(raw)
	return err
}

// rejectCrossOriginRedirect stops the client from following a redirect to a
// different host than the original request, so the x-api-key header
// (which net/http forwards on same-origin redirects) can never leak to
// another host.
func rejectCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}

	origin := via[0].URL
	if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
		return errCrossOriginRedirect
	}

	return nil
}

// Do sends an HTTP request to a data provider endpoint. path is resolved
// against the configured DATA_PROVIDER_BASE_URL using net/url, and the
// x-api-key header is set from DATA_PROVIDER_API_TOKEN on every request.
// ctx bounds the request's lifetime; the client additionally enforces its
// own fixed timeout regardless of ctx.
//
// The caller owns the returned response and must close its body.
func (c *Client) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	requestURL, err := c.resolve(path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(apiKeyHeader, c.config.APIToken)
	req.Header.Set("Accept", acceptJSON)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Never log err itself (see ErrRequestFailed): only a fixed category
		// and the HTTP method, both allowlisted values.
		logger.Get().
			WithField("operation", "dataprovider.do").
			WithField("method", safeMethod(method)).
			WithField("failure", classifyFailure(err)).
			Error(requestFailedLogText)
		return nil, ErrRequestFailed
	}

	return resp, nil
}

// classifyFailure maps a transport error to one of the fixed failure
// categories without exposing its text.
func classifyFailure(err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, errCrossOriginRedirect):
		return failureRedirect
	case errors.Is(err, context.Canceled):
		return failureCanceled
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return failureTimeout
	default:
		return failureTransport
	}
}

// safeMethod returns method only if it is a standard HTTP method, so a
// caller-supplied string can never smuggle arbitrary text into the log.
func safeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "other"
	}
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
