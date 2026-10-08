package healthcheck

import (
	"regexp"
	"testing"
)

// TestNewSubmissionCooldownError_HalfYearMessage pins the exact "Already Submitted" message
// for both survey types, verifying the half-year rollover the product spec requires:
//   - H1 submitted and H2 not submitted -> next eligible period is H2 of the SAME year.
//   - H1 and H2 both submitted -> next eligible period is H1 of the FOLLOWING year.
//   - only H2 submitted -> next eligible period is H1 of the FOLLOWING year.
//
// It also asserts no quarter terminology (Q1-Q4) ever appears in the rendered message or in
// either exposed period field, for either survey type.
func TestNewSubmissionCooldownError_HalfYearMessage(t *testing.T) {
	quarterPattern := regexp.MustCompile(`\bQ[1-4]\b`)

	cases := []struct {
		name                   string
		surveyType             string
		duplicatedPeriod       string
		submittedPeriods       []string
		wantLastSubmitted      string
		wantNextEligiblePeriod string
		wantMessage            string
	}{
		{
			name:                   "Individual Survey: H1 submitted, H2 not submitted -> next eligible is H2 of the same year",
			surveyType:             SurveyTypeIndividual,
			duplicatedPeriod:       "H1 2026",
			submittedPeriods:       []string{"2026 H1"},
			wantLastSubmitted:      "H1 2026",
			wantNextEligiblePeriod: "H2 2026",
			wantMessage:            "You have already submitted the Individual Survey for H1 2026. Your next eligible survey period is H2 2026.",
		},
		{
			name:                   "Individual Survey: H1 and H2 both submitted -> next eligible is H1 of the following year",
			surveyType:             SurveyTypeIndividual,
			duplicatedPeriod:       "H2 2026",
			submittedPeriods:       []string{"2026 H1", "2026 H2"},
			wantLastSubmitted:      "H2 2026",
			wantNextEligiblePeriod: "H1 2027",
			wantMessage:            "You have already submitted the Individual Survey for H2 2026. Your next eligible survey period is H1 2027.",
		},
		{
			name:                   "Post-Workshop Survey: H1 submitted, H2 not submitted -> next eligible is H2 of the same year",
			surveyType:             SurveyTypePostWorkshop,
			duplicatedPeriod:       "H1 2026",
			submittedPeriods:       []string{"2026 H1"},
			wantLastSubmitted:      "H1 2026",
			wantNextEligiblePeriod: "H2 2026",
			wantMessage:            "You have already submitted the Post-Workshop Survey for H1 2026. Your next eligible survey period is H2 2026.",
		},
		{
			name:                   "Post-Workshop Survey: only H2 submitted -> next eligible is H1 of the following year",
			surveyType:             SurveyTypePostWorkshop,
			duplicatedPeriod:       "H2 2026",
			submittedPeriods:       []string{"2026 H2"},
			wantLastSubmitted:      "H2 2026",
			wantNextEligiblePeriod: "H1 2027",
			wantMessage:            "You have already submitted the Post-Workshop Survey for H2 2026. Your next eligible survey period is H1 2027.",
		},
		{
			// Legacy quarter-labeled historical data must still collapse safely to H1/H2 in
			// the message -- a quarter number must never reach the user.
			name:                   "legacy quarter-labeled duplicate ('2026 Q1') renders as H1 2026, never as Q1",
			surveyType:             SurveyTypeIndividual,
			duplicatedPeriod:       "2026 Q1",
			submittedPeriods:       []string{"2026 Q1"},
			wantLastSubmitted:      "H1 2026",
			wantNextEligiblePeriod: "H2 2026",
			wantMessage:            "You have already submitted the Individual Survey for H1 2026. Your next eligible survey period is H2 2026.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewSubmissionCooldownError(tc.surveyType, tc.duplicatedPeriod, tc.submittedPeriods)

			if err.LastSubmittedPeriod != tc.wantLastSubmitted {
				t.Errorf("LastSubmittedPeriod = %q, want %q", err.LastSubmittedPeriod, tc.wantLastSubmitted)
			}
			if err.NextEligiblePeriod != tc.wantNextEligiblePeriod {
				t.Errorf("NextEligiblePeriod = %q, want %q", err.NextEligiblePeriod, tc.wantNextEligiblePeriod)
			}
			if got := err.Error(); got != tc.wantMessage {
				t.Errorf("Error() = %q, want %q", got, tc.wantMessage)
			}
			if quarterPattern.MatchString(err.Error()) {
				t.Errorf("Error() contains a quarter reference: %q", err.Error())
			}
		})
	}
}

// TestNewPeriodNotOpenError verifies the distinct future/past rejection message and reason,
// used when a candidate period is not a duplicate but simply is not open yet (or no longer).
func TestNewPeriodNotOpenError(t *testing.T) {
	cases := []struct {
		name        string
		surveyType  string
		period      string
		reason      PeriodEligibilityReason
		wantMessage string
	}{
		{
			name:        "future period",
			surveyType:  SurveyTypeIndividual,
			period:      "2027 H1",
			reason:      PeriodReasonFuture,
			wantMessage: "Individual Survey for H1 2027 is not yet available.",
		},
		{
			name:        "past period",
			surveyType:  SurveyTypePostWorkshop,
			period:      "2025 H2",
			reason:      PeriodReasonPast,
			wantMessage: "Post-Workshop Survey for H2 2025 is no longer open for new submissions.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewPeriodNotOpenError(tc.surveyType, tc.period, tc.reason)
			if got := err.Error(); got != tc.wantMessage {
				t.Errorf("Error() = %q, want %q", got, tc.wantMessage)
			}
		})
	}
}
