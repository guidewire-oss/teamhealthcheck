// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/email"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

// NotificationService orchestrates email notifications for survey submissions.
type NotificationService struct {
	sender   email.Sender // nil when email is not configured
	teamRepo team.Repository
	userRepo user.Repository
	orgRepo  organization.Repository
}

// NewNotificationService creates a new notification service.
// sender may be nil (email disabled).
func NewNotificationService(
	sender email.Sender,
	teamRepo team.Repository,
	userRepo user.Repository,
	orgRepo organization.Repository,
) *NotificationService {
	return &NotificationService{
		sender:   sender,
		teamRepo: teamRepo,
		userRepo: userRepo,
		orgRepo:  orgRepo,
	}
}

// SendIndividualSurveyEmail sends a copy of the survey responses to the user's email.
// Errors are logged but never returned (fire-and-forget).
func (n *NotificationService) SendIndividualSurveyEmail(ctx context.Context, session *healthcheck.HealthCheckSession) {
	log := logger.Get()

	if n.sender == nil {
		log.Debug("email not configured, skipping individual survey email")
		return
	}

	// Look up user
	usr, err := n.userRepo.FindByID(ctx, session.UserID)
	if err != nil {
		log.WithField("user_id", session.UserID).Warn("notification: failed to find user for individual email")
		return
	}

	if usr.Email == "" {
		log.WithField("user_id", session.UserID).Debug("notification: user has no email, skipping")
		return
	}

	// Look up team name
	tm, err := n.teamRepo.FindByID(ctx, session.TeamID)
	if err != nil {
		log.WithField("team_id", session.TeamID).Warn("notification: failed to find team for individual email")
		return
	}

	// Build dimension results with names
	dimensions := n.buildDimensionResults(ctx, session.Responses)

	data := email.IndividualSurveyEmailData{
		UserName:         usr.Name,
		TeamName:         tm.Name,
		AssessmentPeriod: session.AssessmentPeriod,
		SurveyType:       session.SurveyType,
		Dimensions:       dimensions,
	}

	htmlBody := email.RenderIndividualSurveyEmail(data)
	subject := "Teams360 — Your Health Check Submission (" + tm.Name + ")"

	if err := n.sender.SendHTML(ctx, usr.Email, subject, htmlBody); err != nil {
		log.WithError(err).WithField("to", usr.Email).Warn("notification: failed to send individual survey email")
	} else {
		log.WithField("to", usr.Email).Info("notification: individual survey email sent")
	}
}

// SendPostWorkshopEmails handles email routing for post-workshop surveys.
// If the team has a distribution list configured, the summary goes to the DL only.
// Otherwise, falls back to sending an individual copy to the submitter's email.
func (n *NotificationService) SendPostWorkshopEmails(ctx context.Context, session *healthcheck.HealthCheckSession) {
	log := logger.Get()

	if n.sender == nil {
		log.Debug("email not configured, skipping post-workshop emails")
		return
	}

	// Check if team has a DL configured
	tm, err := n.teamRepo.FindByID(ctx, session.TeamID)
	if err != nil {
		log.WithField("team_id", session.TeamID).Warn("notification: failed to find team for post-workshop emails")
		return
	}

	if tm.DistributionListEmail != nil && *tm.DistributionListEmail != "" {
		n.sendTeamSummaryEmail(ctx, session, tm)
	} else {
		n.SendIndividualSurveyEmail(ctx, session)
	}
}

// sendTeamSummaryEmail sends a post-workshop summary to the team's distribution list.
// Errors are logged but never returned (fire-and-forget).
func (n *NotificationService) sendTeamSummaryEmail(ctx context.Context, session *healthcheck.HealthCheckSession, tm *team.Team) {
	log := logger.Get()

	// Look up submitter name
	submittedBy := session.UserID
	if usr, err := n.userRepo.FindByID(ctx, session.UserID); err == nil {
		submittedBy = usr.Name
	}

	// Build dimension results with names
	dimensions := n.buildDimensionResults(ctx, session.Responses)

	data := email.TeamSummaryEmailData{
		TeamName:         tm.Name,
		AssessmentPeriod: session.AssessmentPeriod,
		SubmittedBy:      submittedBy,
		Dimensions:       dimensions,
	}

	htmlBody := email.RenderTeamSummaryEmail(data)
	subject := "Teams360 — Post-Workshop Summary (" + tm.Name + ", " + session.AssessmentPeriod + ")"

	if err := n.sender.SendHTML(ctx, *tm.DistributionListEmail, subject, htmlBody); err != nil {
		log.WithError(err).WithField("to", *tm.DistributionListEmail).Warn("notification: failed to send team summary email")
	} else {
		log.WithField("to", *tm.DistributionListEmail).Info("notification: team summary email sent")
	}
}

// buildDimensionResults maps session responses to DimensionResult with resolved names.
func (n *NotificationService) buildDimensionResults(ctx context.Context, responses []healthcheck.HealthCheckResponse) []email.DimensionResult {
	// Load dimension names
	dimMap := make(map[string]string)
	if dims, err := n.orgRepo.FindDimensions(ctx); err == nil {
		for _, d := range dims {
			dimMap[d.ID] = d.Name
		}
	}

	results := make([]email.DimensionResult, len(responses))
	for i, r := range responses {
		name := r.DimensionID
		if n, ok := dimMap[r.DimensionID]; ok {
			name = n
		}
		results[i] = email.DimensionResult{
			Name:    name,
			Score:   r.Score,
			Trend:   r.Trend,
			Comment: r.Comment,
		}
	}
	return results
}
