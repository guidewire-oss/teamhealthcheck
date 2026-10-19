package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOrgRepo embeds the (nil) organization.Repository interface so it satisfies the full
// interface without stubbing every method; only FindDimensions is ever exercised by these tests.
type fakeOrgRepo struct {
	organization.Repository
	dimensions []*organization.HealthDimension
	err        error
}

func (f *fakeOrgRepo) FindDimensions(ctx context.Context) ([]*organization.HealthDimension, error) {
	return f.dimensions, f.err
}

func activeDimensions(n int) []*organization.HealthDimension {
	dims := make([]*organization.HealthDimension, n)
	for i := range dims {
		dims[i] = &organization.HealthDimension{ID: string(rune('a' + i)), IsActive: true}
	}
	return dims
}

// fakeHealthRepo embeds the (nil) healthcheck.Repository interface; only SaveDraft is exercised.
type fakeHealthRepo struct {
	healthcheck.Repository
	savedDraft *healthcheck.HealthCheckDraft
	returnErr  error
}

func (f *fakeHealthRepo) SaveDraft(ctx context.Context, draft *healthcheck.HealthCheckDraft) error {
	f.savedDraft = draft
	return f.returnErr
}

func baseCmd() SaveDraftCommand {
	return SaveDraftCommand{
		TeamID:           "team1",
		UserID:           "user1",
		SurveyType:       healthcheck.SurveyTypeIndividual,
		AssessmentPeriod: "2024 - 1st Half",
		CurrentDimension: 0,
		Responses: []HealthCheckResponseCommand{
			{DimensionID: "mission", Score: 2, Trend: "stable"},
		},
	}
}

func TestSaveDraftHandler_Handle_Success(t *testing.T) {
	healthRepo := &fakeHealthRepo{}
	orgRepo := &fakeOrgRepo{dimensions: activeDimensions(5)}
	h := NewSaveDraftHandler(healthRepo, orgRepo)

	saved, err := h.Handle(context.Background(), baseCmd())
	require.NoError(t, err)
	assert.Equal(t, "mission", saved.Responses[0].DimensionID)
	require.NotNil(t, healthRepo.savedDraft)
	assert.Equal(t, "team1", healthRepo.savedDraft.TeamID)
}

func TestSaveDraftHandler_Handle_PropagatesRepositoryError(t *testing.T) {
	repoErr := errors.New("save failed")
	healthRepo := &fakeHealthRepo{returnErr: repoErr}
	orgRepo := &fakeOrgRepo{dimensions: activeDimensions(5)}
	h := NewSaveDraftHandler(healthRepo, orgRepo)

	_, err := h.Handle(context.Background(), baseCmd())
	require.Error(t, err)
	assert.True(t, errors.Is(err, repoErr))
}

func TestSaveDraftHandler_Validate_CurrentDimension(t *testing.T) {
	orgRepo := &fakeOrgRepo{dimensions: activeDimensions(3)} // valid range: 0, 1, 2

	tests := []struct {
		name        string
		dimension   int
		expectValid bool
	}{
		{"negative is rejected", -1, false},
		{"zero is valid", 0, true},
		{"last valid index is valid", 2, true},
		{"equal to dimension count is rejected", 3, false},
		{"far beyond dimension count is rejected", 9999, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthRepo := &fakeHealthRepo{}
			h := NewSaveDraftHandler(healthRepo, orgRepo)
			cmd := baseCmd()
			cmd.CurrentDimension = tt.dimension

			_, err := h.Handle(context.Background(), cmd)
			if tt.expectValid {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				var validationErr *ValidationError
				assert.True(t, errors.As(err, &validationErr), "expected a ValidationError, got %v (%T)", err, err)
			}
		})
	}
}

func TestSaveDraftHandler_Validate_CurrentDimension_NoDimensionsConfigured(t *testing.T) {
	// When no active dimensions are configured (e.g. not yet seeded), the upper-bound check is
	// disabled rather than rejecting every save; negative values are still rejected.
	orgRepo := &fakeOrgRepo{dimensions: nil}

	healthRepo := &fakeHealthRepo{}
	h := NewSaveDraftHandler(healthRepo, orgRepo)
	cmd := baseCmd()
	cmd.CurrentDimension = 500
	_, err := h.Handle(context.Background(), cmd)
	assert.NoError(t, err)

	cmd.CurrentDimension = -1
	_, err = h.Handle(context.Background(), cmd)
	assert.Error(t, err)
}

func TestSaveDraftHandler_Validate_Trend(t *testing.T) {
	orgRepo := &fakeOrgRepo{dimensions: activeDimensions(5)}

	tests := []struct {
		name        string
		trend       string
		expectValid bool
	}{
		{"empty trend is valid (partial draft)", "", true},
		{"improving is valid", "improving", true},
		{"stable is valid", "stable", true},
		{"declining is valid", "declining", true},
		{"unsupported value is rejected", "sideways", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthRepo := &fakeHealthRepo{}
			h := NewSaveDraftHandler(healthRepo, orgRepo)
			cmd := baseCmd()
			cmd.Responses = []HealthCheckResponseCommand{{DimensionID: "mission", Score: 2, Trend: tt.trend}}

			_, err := h.Handle(context.Background(), cmd)
			if tt.expectValid {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				var validationErr *ValidationError
				assert.True(t, errors.As(err, &validationErr))
			}
		})
	}
}

func TestSaveDraftHandler_Validate_RequiredFields(t *testing.T) {
	orgRepo := &fakeOrgRepo{dimensions: activeDimensions(5)}
	healthRepo := &fakeHealthRepo{}
	h := NewSaveDraftHandler(healthRepo, orgRepo)

	cmd := baseCmd()
	cmd.TeamID = ""
	_, err := h.Handle(context.Background(), cmd)
	require.Error(t, err)
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
}
