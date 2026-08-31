package dataprovider

import (
	"io"
	"net/http"
	"strings"
)

const apiTokenHeader = "x-api-token"

// Client makes HTTP requests to the data provider API, authenticating every
// outbound request with the x-api-token header.
type Client struct {
	config     *Config
	httpClient *http.Client
}

// NewClient creates a data provider client from the given configuration.
func NewClient(config *Config) *Client {
	return &Client{
		config:     config,
		httpClient: &http.Client{},
	}
}

// Do sends an HTTP request to a data provider endpoint. path is joined onto
// the configured DATA_PROVIDER_BASE_URL, and the x-api-token header is set
// from DATA_PROVIDER_API_TOKEN on every request.
func (c *Client) Do(method, path string, body io.Reader) (*http.Response, error) {
	url := strings.TrimRight(c.config.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(apiTokenHeader, c.config.APIToken)

	return c.httpClient.Do(req)
}
