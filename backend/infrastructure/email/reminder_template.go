package email

import (
	"fmt"
	"html"
	"strings"
)

// Survey statuses a pod can be in, as the survey completion dashboard reports them.
const (
	podStatusNotStarted = "not_started"
	podStatusInProgress = "in_progress"
	podStatusComplete   = "complete"
	podStatusOptedOut   = "opted_out"
)

// notApplicable is shown in place of a count when a survey does not apply to a pod.
const notApplicable = "—"

// SurveyProgress is how far a pod is through one survey it has to complete.
type SurveyProgress struct {
	Label     string // for example "Individual Survey"
	Completed int
	Total     int
	// NotApplicable marks a survey that does not apply to the pod, shown as "—" instead of a
	// count.
	NotApplicable bool
}

// value is the cell text: "completed / total", or "—" when the survey does not apply.
func (s SurveyProgress) value() string {
	if s.NotApplicable {
		return notApplicable
	}
	return fmt.Sprintf("%d / %d", s.Completed, s.Total)
}

// ReminderPod is one pod (team) in a survey reminder email.
type ReminderPod struct {
	Name         string
	Status       string // not_started | in_progress | complete | opted_out
	TeamLeadName string // empty when the pod has no team lead
	// Surveys is the pod's progress on each survey it has to complete. The email shows one
	// column per distinct label found across its pods, so a survey added later appears without
	// any change to the template.
	Surveys []SurveyProgress
}

// SurveyReminderEmailData holds data for the survey-completion reminder email.
type SurveyReminderEmailData struct {
	RecipientName    string
	Role             string // "leader" or "team lead"
	AssessmentPeriod string
	Pods             []ReminderPod
}

// reminderSubjectBase is the fixed start of every survey reminder subject.
const reminderSubjectBase = "Your Pod Health Check Survey Reminder"

// SurveyReminderSubject is the subject line of a survey reminder email, for example
// "Your Pod Health Check Survey Reminder: 2026 Q4". It is built only from the assessment
// period, so it never carries a recipient's address or a test label, and every reminder
// email uses it.
func SurveyReminderSubject(assessmentPeriod string) string {
	if assessmentPeriod == "" {
		return reminderSubjectBase
	}
	return reminderSubjectBase + ": " + assessmentPeriod
}

func reminderIntro(role string) string {
	if role == "team lead" {
		return "Your pod still needs to complete the survey:"
	}
	return "These pods still need to complete the survey:"
}

func reminderStatusLabel(status string) string {
	switch status {
	case podStatusNotStarted:
		return "Not started"
	case podStatusComplete:
		return "Completed"
	case podStatusOptedOut:
		return "Opted out"
	default:
		return "In progress"
	}
}

// reminderStatusColor is the badge colour for a status, matching the dashboard's red / amber /
// green reading of "not started / in progress / completed".
func reminderStatusColor(status string) string {
	switch status {
	case podStatusNotStarted:
		return "#DC2626"
	case podStatusComplete:
		return "#059669"
	case podStatusOptedOut:
		return "#6B7280"
	default:
		return "#D97706"
	}
}

// surveyColumns returns each distinct survey label across the pods, in the order first seen.
func surveyColumns(pods []ReminderPod) []string {
	seen := map[string]bool{}
	var labels []string
	for _, pod := range pods {
		for _, s := range pod.Surveys {
			if !seen[s.Label] {
				seen[s.Label] = true
				labels = append(labels, s.Label)
			}
		}
	}
	return labels
}

// surveyValue is the cell text for one pod and one survey: its count, or "—" when the pod has no
// such survey or it does not apply.
func (p ReminderPod) surveyValue(label string) string {
	for _, s := range p.Surveys {
		if s.Label == label {
			return s.value()
		}
	}
	return notApplicable
}

// RenderSurveyReminderText renders the same reminder as plain text. It is used to show what an
// email would say when delivery is switched off, where the HTML would be unreadable in a
// terminal.
func RenderSurveyReminderText(data SurveyReminderEmailData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\n%s %s\n", data.RecipientName, reminderIntro(data.Role), data.AssessmentPeriod)

	columns := surveyColumns(data.Pods)
	for _, p := range data.Pods {
		fmt.Fprintf(&b, "\n%s - %s\n", p.Name, reminderStatusLabel(p.Status))
		for _, label := range columns {
			fmt.Fprintf(&b, "  %s: %s\n", label, p.surveyValue(label))
		}
		if p.TeamLeadName != "" {
			fmt.Fprintf(&b, "  Team lead: %s\n", p.TeamLeadName)
		}
	}

	b.WriteString("\nThis is an automated reminder. Log in to Teams360 to complete or follow up on the survey.")
	return b.String()
}

// reminderStyles makes the table readable on a phone: below 480px the header row is hidden and
// each pod's cells stack, each prefixed by its own label. Clients that ignore <style> still get
// the ordinary table, which is why every layout rule is also inline.
const reminderStyles = `<style>
  @media only screen and (max-width:480px) {
    .container { width:100% !important; border-radius:0 !important; }
    .pad { padding:20px 16px !important; }
    .rem-head { display:none !important; }
    .rem-cell { display:block !important; width:100% !important; box-sizing:border-box !important; text-align:left !important; border-bottom:0 !important; padding:3px 12px !important; }
    .rem-cell-last { border-bottom:1px solid #E5E7EB !important; padding-bottom:12px !important; }
    .rem-label { display:inline !important; }
  }
</style>`

const cellBase = "padding:10px 12px;border-bottom:1px solid #E5E7EB;font-size:14px;color:#111827;vertical-align:top;"

// renderReminderTable renders the pods as a table: one row per pod, one column per survey, and
// the pod's overall status.
func renderReminderTable(pods []ReminderPod) string {
	columns := surveyColumns(pods)

	// Pod and status take a fixed share; the surveys split the rest.
	surveyWidth := 0
	if len(columns) > 0 {
		surveyWidth = (100 - 30 - 18) / len(columns)
	}

	var head strings.Builder
	head.WriteString(`<th align="left" style="padding:10px 12px;font-size:12px;color:#6B7280;text-transform:uppercase;width:30%;">Pod</th>`)
	for _, label := range columns {
		fmt.Fprintf(&head, `<th align="center" style="padding:10px 12px;font-size:12px;color:#6B7280;text-transform:uppercase;width:%d%%;">%s</th>`,
			surveyWidth, html.EscapeString(label))
	}
	head.WriteString(`<th align="center" style="padding:10px 12px;font-size:12px;color:#6B7280;text-transform:uppercase;width:18%;">Status</th>`)

	var rows strings.Builder
	for _, p := range pods {
		lead := ""
		if p.TeamLeadName != "" {
			lead = fmt.Sprintf(`<br><span style="font-size:12px;font-weight:400;color:#6B7280;">Team lead: %s</span>`, html.EscapeString(p.TeamLeadName))
		}
		rows.WriteString(`<tr>`)
		fmt.Fprintf(&rows, `<td class="rem-cell" style="%sfont-weight:600;" align="left">%s%s</td>`, cellBase, html.EscapeString(p.Name), lead)
		for _, label := range columns {
			fmt.Fprintf(&rows, `<td class="rem-cell" style="%stext-align:center;" align="center"><span class="rem-label" style="display:none;font-size:11px;color:#6B7280;text-transform:uppercase;">%s&nbsp;</span>%s</td>`,
				cellBase, html.EscapeString(label), html.EscapeString(p.surveyValue(label)))
		}
		fmt.Fprintf(&rows, `<td class="rem-cell rem-cell-last" style="%stext-align:center;" align="center"><span style="display:inline-block;padding:2px 10px;border-radius:12px;background:%s;color:#fff;font-size:12px;font-weight:600;white-space:nowrap;">%s</span></td>`,
			cellBase, reminderStatusColor(p.Status), reminderStatusLabel(p.Status))
		rows.WriteString(`</tr>`)
	}

	return fmt.Sprintf(`<table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #E5E7EB;border-radius:6px;overflow:hidden;">
      <thead>
        <tr class="rem-head" style="background:#F9FAFB;">%s</tr>
      </thead>
      <tbody>
        %s
      </tbody>
    </table>`, head.String(), rows.String())
}

// RenderSurveyReminderEmail renders the HTML email nudging a leader or team lead about pods that
// have not finished the survey: a table with each pod's progress on every survey and its overall
// status.
func RenderSurveyReminderEmail(data SurveyReminderEmailData) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
%s
</head>
<body style="margin:0;padding:0;background:#F3F4F6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#F3F4F6;padding:24px 0;">
<tr><td align="center">
<table class="container" width="100%%" cellpadding="0" cellspacing="0" style="max-width:640px;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1);">
  <tr><td style="background:#7C3AED;padding:24px 32px;">
    <h1 style="margin:0;color:#fff;font-size:20px;">Teams360</h1>
    <p style="margin:4px 0 0;color:#DDD6FE;font-size:14px;">Survey reminder</p>
  </td></tr>
  <tr><td class="pad" style="padding:24px 32px;">
    <p style="margin:0 0 16px;color:#374151;">Hi %s,</p>
    <p style="margin:0 0 16px;color:#374151;">%s <strong>%s</strong></p>
    %s
    <p style="margin:24px 0 0;color:#9CA3AF;font-size:12px;">This is an automated reminder. Log in to Teams360 to complete or follow up on the survey.</p>
  </td></tr>
  <tr><td style="background:#F9FAFB;padding:16px 32px;text-align:center;">
    <p style="margin:0;color:#9CA3AF;font-size:11px;">Teams360 — Team Health Check Platform</p>
  </td></tr>
</table>
</td></tr>
</table>
</body>
</html>`, reminderStyles, html.EscapeString(data.RecipientName), reminderIntro(data.Role),
		html.EscapeString(data.AssessmentPeriod), renderReminderTable(data.Pods))
}
