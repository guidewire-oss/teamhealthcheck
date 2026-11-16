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

// OrganizationSyncRequestDTO is the optional body of the manual sync trigger.
// An absent body means a normal sync, which is what the Sync Now button sends.
type OrganizationSyncRequestDTO struct {
	// OverrideMassDeletion asks the backend to apply a sync that the
	// mass-deletion guard would otherwise hold. It is honoured only for an
	// authenticated administrator and only for the request carrying it -- it is
	// never stored, and it never changes the configured threshold.
	OverrideMassDeletion bool `json:"overrideMassDeletion"`

	// ConfirmedMassDeletion echoes the counts the admin reviewed on the held
	// sync's response, and is required when OverrideMassDeletion is true. Every
	// sync re-fetches the provider snapshot fresh, so without this the backend
	// would have no way to tell whether the admin's confirmation still matches
	// what is actually about to be deleted. A missing or stale value (no longer
	// matching a freshly recomputed report) is refused, not silently ignored.
	ConfirmedMassDeletion *ConfirmedMassDeletionDTO `json:"confirmedMassDeletion,omitempty"`
}

// ConfirmedMassDeletionDTO carries the four counts an admin reviewed before
// checking overrideMassDeletion -- an echo of the corresponding fields on
// MassDeletionReportDTO, not a client-computed value.
type ConfirmedMassDeletionDTO struct {
	UsersExisting int `json:"usersExisting"`
	UsersDeleting int `json:"usersDeleting"`
	TeamsExisting int `json:"teamsExisting"`
	TeamsDeleting int `json:"teamsDeleting"`
}

// DeletionMetricDTO is one entity type's share of a proposed deletion, shown to
// an admin reviewing a held sync. It mirrors orgprovider.DeletionMetric field
// for field; the dto package deliberately imports no domain package, so the
// handler maps between them.
type DeletionMetricDTO struct {
	// Kind is "deleted" for rows the sync removes directly, or "cascaded" for
	// rows the database removes as a consequence.
	Kind string `json:"kind"`
	// Existing is the denominator: eligible (non-protected) rows in THC now.
	Existing int `json:"existing"`
	// Incoming is how many records of this type the provider's snapshot
	// contained, for diagnosing a hold caused by an under-reported payload.
	Incoming int `json:"incoming"`
	// Deleting is the numerator: rows this sync proposes to remove.
	Deleting int `json:"deleting"`
	// Percent is Deleting/Existing*100 as the guard itself computes it.
	Percent float64 `json:"percent"`
	// Threshold is the configured maximum percentage.
	Threshold float64 `json:"threshold"`
	// ExceedsThreshold is this metric's own comparison result.
	ExceedsThreshold bool `json:"exceedsThreshold"`
	// ContributesToHold is false for metrics reported for information only.
	ContributesToHold bool `json:"contributesToHold"`
}

// MassDeletionReportDTO carries the counts behind a held sync. It contains only
// aggregate numbers: no record identities, no provider payload, no credentials.
type MassDeletionReportDTO struct {
	Threshold   float64            `json:"threshold"`
	Users       DeletionMetricDTO  `json:"users"`
	Teams       DeletionMetricDTO  `json:"teams"`
	Memberships *DeletionMetricDTO `json:"memberships,omitempty"`
}

// CodeMassDeletionHold is the ErrorResponse.Code a held sync carries, so the
// UI can recognise the hold from the typed field rather than by matching on
// message text.
const CodeMassDeletionHold = "mass_deletion_hold"

// MassDeletionHoldResponseDTO is the body returned when the mass-deletion guard
// holds a sync. It is a superset of the standard ErrorResponse -- the embedded
// fields serialize inline, so existing clients that only read error/message/code
// are unaffected.
type MassDeletionHoldResponseDTO struct {
	ErrorResponse
	// Applied is always false: a held sync writes nothing.
	Applied bool `json:"applied"`
	// MassDeletion is the counts an admin reviews before deciding whether to
	// re-request the sync with an explicit override.
	MassDeletion *MassDeletionReportDTO `json:"massDeletion,omitempty"`
}
