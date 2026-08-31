package dataprovider

import "os"

// Config holds data provider configuration loaded from environment variables.
type Config struct {
	BaseURL  string
	APIToken string
}

// LoadConfig reads data provider settings from environment variables.
// Returns nil if DATA_PROVIDER_BASE_URL is not set (disables the data provider).
func LoadConfig() *Config {
	baseURL := os.Getenv("DATA_PROVIDER_BASE_URL")
	if baseURL == "" {
		return nil
	}

	return &Config{
		BaseURL:  baseURL,
		APIToken: os.Getenv("DATA_PROVIDER_API_TOKEN"),
	}
}
