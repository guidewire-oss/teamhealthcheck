package email

import (
	"strings"
	"testing"
)

const (
	individual   = "Individual Survey"
	postWorkshop = "Post-Workshop Survey"
)

func podWith(name, status string, surveys ...SurveyProgress) ReminderPod {
	return ReminderPod{Name: name, Status: status, Surveys: surveys}
}

func progress(label string, completed, total int) SurveyProgress {
	return SurveyProgress{Label: label, Completed: completed, Total: total}
}

func notApplicableSurvey(label string) SurveyProgress {
	return SurveyProgress{Label: label, NotApplicable: true}
}

func hopefarm() ReminderPod {
	pod := podWith("HopeFarm", "in_progress", progress(individual, 1, 5), progress(postWorkshop, 0, 1))
	pod.TeamLeadName = "Yash Jambotkar"
	return pod
}

func render(pods ...ReminderPod) (htmlBody, text string) {
	data := SurveyReminderEmailData{RecipientName: "Yash", Role: "team lead", AssessmentPeriod: "2026 Q4", Pods: pods}
	return RenderSurveyReminderEmail(data), RenderSurveyReminderText(data)
}

func mustContain(t *testing.T, label, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("%s is missing %q", label, want)
		}
	}
}

func TestReminderTableShowsEachSurveyAsCompletedOverTotal(t *testing.T) {
	htmlBody, text := render(hopefarm())

	mustContain(t, "html", htmlBody, "HopeFarm", individual, "1 / 5", postWorkshop, "0 / 1", "In progress", "Team lead: Yash Jambotkar")
	mustContain(t, "text", text, "HopeFarm - In progress", individual+": 1 / 5", postWorkshop+": 0 / 1", "Team lead: Yash Jambotkar")
}

func TestReminderTableHasOneColumnPerSurveyPlusPodAndStatus(t *testing.T) {
	htmlBody, _ := render(hopefarm())

	if got := strings.Count(htmlBody, "<th "); got != 4 {
		t.Errorf("header cells = %d, want 4 (Pod, two surveys, Status)", got)
	}
	mustContain(t, "html", htmlBody, ">Pod</th>", ">"+individual+"</th>", ">"+postWorkshop+"</th>", ">Status</th>")
}

func TestReminderUsesADashWhereASurveyDoesNotApply(t *testing.T) {
	pod := podWith("Falcons", "not_started", progress(individual, 0, 3), notApplicableSurvey(postWorkshop))
	htmlBody, text := render(pod)

	mustContain(t, "html", htmlBody, ">—</td>")
	mustContain(t, "text", text, postWorkshop+": —")
	if strings.Contains(htmlBody, "0 / 0") || strings.Contains(text, "0 / 0") {
		t.Error("a survey that does not apply must show a dash, not a zero count")
	}
}

// The columns come from the data, so a survey added later appears with no change to the template.
func TestReminderGrowsAColumnForAnyAdditionalSurvey(t *testing.T) {
	const extra = "Security Training"
	a := podWith("Alpha", "in_progress", progress(individual, 2, 4), progress(extra, 3, 4))
	b := podWith("Beta", "not_started", progress(individual, 0, 2))

	htmlBody, text := render(a, b)

	mustContain(t, "html", htmlBody, ">"+extra+"</th>", "3 / 4")
	if got := strings.Count(htmlBody, "<th "); got != 4 {
		t.Errorf("header cells = %d, want 4 (Pod, Individual, extra, Status)", got)
	}
	mustContain(t, "text", text, extra+": 3 / 4", extra+": —") // Beta has no such survey
}

func TestReminderLabelsTheOverallStatus(t *testing.T) {
	cases := []struct{ status, label, color string }{
		{"not_started", "Not started", "#DC2626"},
		{"in_progress", "In progress", "#D97706"},
		{"complete", "Completed", "#059669"},
		{"opted_out", "Opted out", "#6B7280"},
	}
	for _, c := range cases {
		htmlBody, text := render(podWith("P", c.status, progress(individual, 0, 2)))
		mustContain(t, c.status+" html", htmlBody, c.label, c.color)
		mustContain(t, c.status+" text", text, "P - "+c.label)
	}
}

func TestReminderSplitsTheSpaceBetweenSurveyColumns(t *testing.T) {
	htmlBody, _ := render(hopefarm())

	// Pod 30% + Status 18% leaves 52% for two surveys: 26% each.
	mustContain(t, "html", htmlBody, "width:30%;", "width:26%;", "width:18%;")
}

func TestReminderIsResponsiveAndKeepsItsBranding(t *testing.T) {
	htmlBody, _ := render(hopefarm())

	mustContain(t, "html", htmlBody,
		`<meta name="viewport"`, "@media only screen and (max-width:480px)", `class="container"`, "max-width:640px",
		"Teams360", "Survey reminder", "#7C3AED",
		"Your pod still needs to complete the survey:", "2026 Q4",
		"This is an automated reminder. Log in to Teams360 to complete or follow up on the survey.")
	if strings.Contains(htmlBody, `width="600"`) {
		t.Error("the container must not be a fixed 600px wide")
	}
}

func TestReminderRepeatsEachSurveyLabelInlineForNarrowScreens(t *testing.T) {
	htmlBody, _ := render(hopefarm())

	// The header row is hidden on a phone, so every value carries its own label.
	for _, label := range []string{individual, postWorkshop} {
		mustContain(t, "html", htmlBody, `<span class="rem-label" style="display:none;`+"font-size:11px;color:#6B7280;text-transform:uppercase;\">"+label+"&nbsp;</span>")
	}
}

func TestReminderTextHasNoMarkup(t *testing.T) {
	_, text := render(hopefarm())

	if strings.ContainsAny(text, "<>") {
		t.Errorf("the plain-text reminder must not contain HTML:\n%s", text)
	}
}

func TestReminderEscapesNamesAndLabels(t *testing.T) {
	pod := podWith(`<script>alert(1)</script>`, "in_progress", progress(`<b>Survey</b>`, 1, 2))
	pod.TeamLeadName = `"><img src=x onerror=alert(1)>`
	htmlBody, _ := render(pod)

	for _, bad := range []string{"<script>", "<img src=x", "<b>Survey</b>"} {
		if strings.Contains(htmlBody, bad) {
			t.Errorf("unescaped %q in the email", bad)
		}
	}
}

func TestReminderOmitsTheTeamLeadLineWhenThereIsNone(t *testing.T) {
	pod := hopefarm()
	pod.TeamLeadName = ""
	htmlBody, text := render(pod)

	for label, body := range map[string]string{"html": htmlBody, "text": text} {
		if strings.Contains(body, "Team lead") {
			t.Errorf("%s: no team lead line expected when the pod has none", label)
		}
	}
}

func TestReminderListsEveryPodAsARow(t *testing.T) {
	htmlBody, text := render(hopefarm(), podWith("Falcons", "not_started", progress(individual, 0, 3), progress(postWorkshop, 0, 1)))

	mustContain(t, "html", htmlBody, "HopeFarm", "Falcons", "Not started", "0 / 3")
	mustContain(t, "text", text, "HopeFarm - In progress", "Falcons - Not started")
	if got := strings.Count(htmlBody, `<td class="rem-cell`); got != 8 {
		t.Errorf("body cells = %d, want 8 (2 pods x 4 columns)", got)
	}
}

func TestReminderWithNoPodsStillRenders(t *testing.T) {
	htmlBody, text := render()

	mustContain(t, "html", htmlBody, "Hi Yash,", "<table")
	mustContain(t, "text", text, "Hi Yash,")
}

func TestSurveyReminderSubject(t *testing.T) {
	cases := map[string]string{
		"2026 Q4": "Your Pod Health Check Survey Reminder: 2026 Q4",
		"":        "Your Pod Health Check Survey Reminder",
	}
	for period, want := range cases {
		if got := SurveyReminderSubject(period); got != want {
			t.Errorf("SurveyReminderSubject(%q) = %q, want %q", period, got, want)
		}
	}
	if got := SurveyReminderSubject("2026 Q4"); strings.ContainsAny(got, "@[]") {
		t.Errorf("subject must not carry an address or label: %q", got)
	}
}
