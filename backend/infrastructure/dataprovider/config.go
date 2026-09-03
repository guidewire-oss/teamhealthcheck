package dataprovider

import (
	"errors"
	"os"
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
	baseURL := os.Getenv("DATA_PROVIDER_BASE_URL")
	if baseURL == "" {
		return nil, nil
	}

	apiToken := os.Getenv("DATA_PROVIDER_API_TOKEN")
	if apiToken == "" {
		return nil, ErrMissingAPIToken
	}

	return &Config{
		BaseURL:  baseURL,
		APIToken: apiToken,
	}, nil
}
