package dataprovider

import "testing"

func TestLoadConfig_ReturnsNilWhenBaseURLUnset(t *testing.T) {
	t.Setenv("DATA_PROVIDER_BASE_URL", "")
	t.Setenv("DATA_PROVIDER_API_TOKEN", "")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if config != nil {
		t.Fatalf("expected nil config, got %+v", config)
	}
}

func TestLoadConfig_ReturnsConfigWhenBothSet(t *testing.T) {
	t.Setenv("DATA_PROVIDER_BASE_URL", "https://provider.example.com")
	t.Setenv("DATA_PROVIDER_API_TOKEN", "secret-token")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.BaseURL != "https://provider.example.com" {
		t.Errorf("BaseURL = %q, want %q", config.BaseURL, "https://provider.example.com")
	}
	if config.APIToken != "secret-token" {
		t.Errorf("APIToken = %q, want %q", config.APIToken, "secret-token")
	}
}

func TestLoadConfig_ErrorsWhenTokenMissing(t *testing.T) {
	t.Setenv("DATA_PROVIDER_BASE_URL", "https://provider.example.com")
	t.Setenv("DATA_PROVIDER_API_TOKEN", "")

	config, err := LoadConfig()
	if err != ErrMissingAPIToken {
		t.Fatalf("err = %v, want %v", err, ErrMissingAPIToken)
	}
	if config != nil {
		t.Fatalf("expected nil config, got %+v", config)
	}
}
