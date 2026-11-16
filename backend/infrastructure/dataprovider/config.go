package dataprovider

import (
	"errors"
	"os"
)

// EnvBaseURL and EnvAPIToken name the environment variables the data provider
// configuration is read from. Both are configuration keys only -- the value
// read from EnvAPIToken is sent to the provider in the x-api-key header, never
// as a bearer token and never under a header named after the variable.
const (
	EnvBaseURL  = "DATA_PROVIDER_BASE_URL"
	EnvAPIToken = "DATA_PROVIDER_API_TOKEN"
)

// ErrMissingAPIToken is returned when DATA_PROVIDER_BASE_URL is set but
// DATA_PROVIDER_API_TOKEN is not, since requests would go out unauthenticated.
var ErrMissingAPIToken = errors.New("dataprovider: DATA_PROVIDER_API_TOKEN is required when DATA_PROVIDER_BASE_URL is set")

// Config holds data provider configuration loaded from environment variables.
type Config struct {
	BaseURL  string
	APIToken string
}

// LoadConfig reads data provider settings from environment variables.
// It returns (nil, nil) if DATA_PROVIDER_BASE_URL is not set, which disables
// the data provider. It returns an error if DATA_PROVIDER_BASE_URL is set
// without DATA_PROVIDER_API_TOKEN, since that configuration is incomplete.
func LoadConfig() (*Config, error) {
	baseURL := os.Getenv(EnvBaseURL)
	if baseURL == "" {
		return nil, nil
	}

	apiToken := os.Getenv(EnvAPIToken)
	if apiToken == "" {
		return nil, ErrMissingAPIToken
	}

	return &Config{
		BaseURL:  baseURL,
		APIToken: apiToken,
	}, nil
}
