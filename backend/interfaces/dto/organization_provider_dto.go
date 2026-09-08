package dto

// OrganizationProviderSettingsDTO reports whether the external organization-
// data provider is configured and ready to sync.
//
// There is no token field, ever: the provider credential lives only in
// environment configuration (DATA_PROVIDER_BASE_URL / DATA_PROVIDER_API_TOKEN),
// is read once at startup by the transport client, and is never stored,
// echoed, or otherwise made readable through this API.
type OrganizationProviderSettingsDTO struct {
	Provider string `json:"provider"`

	// BaseURLConfigured is true when DATA_PROVIDER_BASE_URL is set and valid.
	BaseURLConfigured bool `json:"baseUrlConfigured"`
	// TokenConfigured is true when DATA_PROVIDER_API_TOKEN is set.
	TokenConfigured bool `json:"tokenConfigured"`
	// ReadyToSync is true when every configuration prerequisite is met.
	ReadyToSync bool `json:"readyToSync"`
}
