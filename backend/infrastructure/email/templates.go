// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package email

import (
	"fmt"
	"html"
	"strings"
)

// DimensionResult holds a single dimension's data for email rendering.
type DimensionResult struct {
	Name    string
	Score   int    // 1=Red, 2=Yellow, 3=Green
	Trend   string // improving, stable, declining
	Comment string
}

// IndividualSurveyEmailData holds data for the individual survey email.
type IndividualSurveyEmailData struct {
	UserName         string
	TeamName         string
	AssessmentPeriod string
	SurveyType       string
	Dimensions       []DimensionResult
}

// TeamSummaryEmailData holds data for the team summary email.
type TeamSummaryEmailData struct {
	TeamName         string
	AssessmentPeriod string
	SubmittedBy      string
	Dimensions       []DimensionResult
}

// ScoreToLabel converts a numeric score to a label.
func ScoreToLabel(score int) string {
	switch score {
	case 1:
		return "Red"
	case 2:
		return "Yellow"
	case 3:
		return "Green"
	default:
		return "Unknown"
	}
}

// ScoreToColor converts a numeric score to a hex color.
func ScoreToColor(score int) string {
	switch score {
	case 1:
		return "#EF4444" // red
	case 2:
		return "#EAB308" // yellow
	case 3:
		return "#22C55E" // green
	default:
		return "#6B7280" // gray
	}
}

// TrendToIcon converts a trend to a Unicode arrow.
func TrendToIcon(trend string) string {
	switch strings.ToLower(trend) {
	case "improving":
		return "&#8593;" // ↑
	case "declining":
		return "&#8595;" // ↓
	case "stable":
		return "&#8594;" // →
	default:
		return ""
	}
}

// RenderIndividualSurveyEmail renders the HTML email for an individual survey copy.
func RenderIndividualSurveyEmail(data IndividualSurveyEmailData) string {
	var rows strings.Builder
	for _, d := range data.Dimensions {
		escapedName := html.EscapeString(d.Name)
		escapedComment := html.EscapeString(d.Comment)
		commentCell := ""
		if d.Comment != "" {
			commentCell = fmt.Sprintf(`<td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;font-size:13px;color:#4B5563;">%s</td>`, escapedComment)
		} else {
			commentCell = `<td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;font-size:13px;color:#9CA3AF;">-</td>`
		}

		rows.WriteString(fmt.Sprintf(`<tr>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;font-weight:500;">%s</td>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;text-align:center;">
    <span style="display:inline-block;padding:2px 10px;border-radius:12px;background:%s;color:#fff;font-size:12px;font-weight:600;">%s</span>
  </td>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;text-align:center;font-size:16px;">%s</td>
  %s
</tr>`, escapedName, ScoreToColor(d.Score), ScoreToLabel(d.Score), TrendToIcon(d.Trend), commentCell))
	}

	surveyLabel := "Health Check"
	if data.SurveyType == "post_workshop" {
		surveyLabel = "Post-Workshop Survey"
	}

	escapedUserName := html.EscapeString(data.UserName)
	escapedTeamName := html.EscapeString(data.TeamName)
	escapedPeriod := html.EscapeString(data.AssessmentPeriod)

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#F3F4F6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#F3F4F6;padding:24px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1);">
  <tr><td style="background:#1E40AF;padding:24px 32px;">
    <h1 style="margin:0;color:#fff;font-size:20px;">Teams360</h1>
    <p style="margin:4px 0 0;color:#BFDBFE;font-size:14px;">%s Submission Copy</p>
  </td></tr>
  <tr><td style="padding:24px 32px;">
    <p style="margin:0 0 8px;color:#374151;">Hi <strong>%s</strong>,</p>
    <p style="margin:0 0 16px;color:#6B7280;font-size:14px;">
      Here is a copy of your %s responses for <strong>%s</strong> — %s.
    </p>
    <table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #E5E7EB;border-radius:6px;overflow:hidden;">
      <thead>
        <tr style="background:#F9FAFB;">
          <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6B7280;text-transform:uppercase;">Dimension</th>
          <th style="padding:10px 12px;text-align:center;font-size:12px;color:#6B7280;text-transform:uppercase;">Score</th>
          <th style="padding:10px 12px;text-align:center;font-size:12px;color:#6B7280;text-transform:uppercase;">Trend</th>
          <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6B7280;text-transform:uppercase;">Comment</th>
        </tr>
      </thead>
      <tbody>
        %s
      </tbody>
    </table>
    <p style="margin:24px 0 0;color:#9CA3AF;font-size:12px;">This is an automated copy of your submission. No action is required.</p>
  </td></tr>
  <tr><td style="background:#F9FAFB;padding:16px 32px;text-align:center;">
    <p style="margin:0;color:#9CA3AF;font-size:11px;">Teams360 — Team Health Check Platform</p>
  </td></tr>
</table>
</td></tr>
</table>
</body>
</html>`, surveyLabel, escapedUserName, strings.ToLower(surveyLabel), escapedTeamName, escapedPeriod, rows.String())
}

// RenderTeamSummaryEmail renders the HTML email for a team DL summary.
func RenderTeamSummaryEmail(data TeamSummaryEmailData) string {
	var rows strings.Builder
	for _, d := range data.Dimensions {
		escapedName := html.EscapeString(d.Name)
		escapedComment := html.EscapeString(d.Comment)
		commentCell := ""
		if d.Comment != "" {
			commentCell = fmt.Sprintf(`<td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;font-size:13px;color:#4B5563;">%s</td>`, escapedComment)
		} else {
			commentCell = `<td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;font-size:13px;color:#9CA3AF;">-</td>`
		}

		rows.WriteString(fmt.Sprintf(`<tr>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;font-weight:500;">%s</td>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;text-align:center;">
    <span style="display:inline-block;padding:2px 10px;border-radius:12px;background:%s;color:#fff;font-size:12px;font-weight:600;">%s</span>
  </td>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;text-align:center;font-size:16px;">%s</td>
  %s
</tr>`, escapedName, ScoreToColor(d.Score), ScoreToLabel(d.Score), TrendToIcon(d.Trend), commentCell))
	}

	escapedTeamName := html.EscapeString(data.TeamName)
	escapedPeriod := html.EscapeString(data.AssessmentPeriod)
	escapedSubmittedBy := html.EscapeString(data.SubmittedBy)

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#F3F4F6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#F3F4F6;padding:24px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1);">
  <tr><td style="background:#7C3AED;padding:24px 32px;">
    <h1 style="margin:0;color:#fff;font-size:20px;">Teams360</h1>
    <p style="margin:4px 0 0;color:#DDD6FE;font-size:14px;">Post-Workshop Survey Summary</p>
  </td></tr>
  <tr><td style="padding:24px 32px;">
    <p style="margin:0 0 16px;color:#374151;">
      A post-workshop survey has been submitted for <strong>%s</strong> — %s.
    </p>
    <p style="margin:0 0 16px;color:#6B7280;font-size:14px;">Submitted by: %s</p>
    <table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #E5E7EB;border-radius:6px;overflow:hidden;">
      <thead>
        <tr style="background:#F9FAFB;">
          <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6B7280;text-transform:uppercase;">Dimension</th>
          <th style="padding:10px 12px;text-align:center;font-size:12px;color:#6B7280;text-transform:uppercase;">Score</th>
          <th style="padding:10px 12px;text-align:center;font-size:12px;color:#6B7280;text-transform:uppercase;">Trend</th>
          <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6B7280;text-transform:uppercase;">Comment</th>
        </tr>
      </thead>
      <tbody>
        %s
      </tbody>
    </table>
    <p style="margin:24px 0 0;color:#9CA3AF;font-size:12px;">This is an automated summary. Log in to Teams360 for full analytics.</p>
  </td></tr>
  <tr><td style="background:#F9FAFB;padding:16px 32px;text-align:center;">
    <p style="margin:0;color:#9CA3AF;font-size:11px;">Teams360 — Team Health Check Platform</p>
  </td></tr>
</table>
</td></tr>
</table>
</body>
</html>`, escapedTeamName, escapedPeriod, escapedSubmittedBy, rows.String())
}
