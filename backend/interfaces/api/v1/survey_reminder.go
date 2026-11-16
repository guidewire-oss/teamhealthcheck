package v1

import (
	"context"
	"errors"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/application/queries"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/email"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

var (
	// errReminderTargetNotFound means the requested team or leader is not in
	// the survey-completion data for the period.
	errReminderTargetNotFound = errors.New("reminder target not found")
	// errNothingToRemind means the target exists but has no pod that is still
	// in progress or not started.
	errNothingToRemind = errors.New("nothing to remind: no pods are in progress or not started")
	// errInvalidReminderRequest means the scope or its target id is missing.
	errInvalidReminderRequest = errors.New("invalid reminder request")
	// errTestPodNotFound means EMAIL_TEST_POD names a pod that does not exist.
	// The run fails closed: nothing is sent.
	errTestPodNotFound = errors.New("test pod not found")
	// errTestPodAmbiguous means more than one pod has the EMAIL_TEST_POD name.
	// Team names are not unique, so the run fails closed: nothing is sent.
	errTestPodAmbiguous = errors.New("test pod name matches more than one pod")
)

const (
	reminderRoleLeader   = "leader"
	reminderRoleTeamLead = "team lead"
)

// reminderRecipient is one person who gets one email, with every lagging pod
// that email lists.
type reminderRecipient struct {
	UserID string
	Name   string
	Email  string
	Role   string
	Pods   []dto.SurveyCompletionTeamDTO
}

// reminderPlan is who a reminder run will mail, decided from the
// survey-completion tree.
type reminderPlan struct {
	Recipients []*reminderRecipient
	// PodsWithoutTeamLead are lagging pods that have no team lead to mail.
	PodsWithoutTeamLead []string
}

// isRemindable reports whether a pod still needs the survey. It is the same
// rule as isRemindable in frontend/lib/survey-completion-tree.ts, which is
// what makes the dashboard's preview match what is sent.
func isRemindable(t dto.SurveyCompletionTeamDTO) bool {
	return t.Status == "in_progress" || t.Status == "not_started"
}

// recipientSet collects recipients keyed by user, so a person who is both a
// leader and a team lead, or who leads several pods, gets one email.
type recipientSet struct {
	byKey         map[string]*reminderRecipient
	order         []string
	teamLeadEmail map[string]string // teamID -> team lead email
	noTeamLead    []string
	seenNoLead    map[string]bool
}

func newRecipientSet(teamLeadEmail map[string]string) *recipientSet {
	return &recipientSet{
		byKey:         map[string]*reminderRecipient{},
		teamLeadEmail: teamLeadEmail,
		seenNoLead:    map[string]bool{},
	}
}

func (s *recipientSet) add(userID, name, emailAddr, role string, pods ...dto.SurveyCompletionTeamDTO) {
	key := userID
	if key == "" {
		key = emailAddr
	}
	r, ok := s.byKey[key]
	if !ok {
		r = &reminderRecipient{UserID: userID, Name: name, Email: emailAddr, Role: role}
		s.byKey[key] = r
		s.order = append(s.order, key)
	}
	if role == reminderRoleLeader {
		r.Role = reminderRoleLeader
	}
	for _, p := range pods {
		if !hasPod(r.Pods, p.TeamID) {
			r.Pods = append(r.Pods, p)
		}
	}
}

func hasPod(pods []dto.SurveyCompletionTeamDTO, teamID string) bool {
	for _, p := range pods {
		if p.TeamID == teamID {
			return true
		}
	}
	return false
}

// addTeamLead adds a pod's team lead, or records the pod as having none.
func (s *recipientSet) addTeamLead(t dto.SurveyCompletionTeamDTO) {
	if t.TeamLeadID == nil || *t.TeamLeadID == "" {
		if !s.seenNoLead[t.TeamID] {
			s.seenNoLead[t.TeamID] = true
			s.noTeamLead = append(s.noTeamLead, t.TeamName)
		}
		return
	}
	name := ""
	if t.TeamLeadName != nil {
		name = *t.TeamLeadName
	}
	s.add(*t.TeamLeadID, name, s.teamLeadEmail[t.TeamID], reminderRoleTeamLead, t)
}

// addLeaderTree walks one leader's subtree the way buildSubtree does in
// frontend/lib/reminder-tree.ts: the leader's own lagging pods plus every
// descendant leader that has at least one lagging pod below them. An
// all-complete branch is dropped at any depth. It returns every lagging pod
// in the subtree.
func (s *recipientSet) addLeaderTree(g dto.SurveyCompletionGroupDTO) []dto.SurveyCompletionTeamDTO {
	var pods []dto.SurveyCompletionTeamDTO

	for _, t := range g.DirectTeams {
		if !isRemindable(t) {
			continue
		}
		pods = append(pods, t)
		s.addTeamLead(t)
	}
	for _, child := range g.Children {
		pods = append(pods, s.addLeaderTree(child)...)
	}

	if len(pods) > 0 && g.Person != nil {
		s.add(g.Person.ID, g.Person.Name, g.Person.Email, reminderRoleLeader, pods...)
	}
	return pods
}

func (s *recipientSet) plan() *reminderPlan {
	out := &reminderPlan{PodsWithoutTeamLead: s.noTeamLead}
	for _, key := range s.order {
		out.Recipients = append(out.Recipients, s.byKey[key])
	}
	return out
}

// findPersonGroup returns the leader group with the given user id, at any depth.
func findPersonGroup(groups []dto.SurveyCompletionGroupDTO, personID string) *dto.SurveyCompletionGroupDTO {
	for i := range groups {
		g := &groups[i]
		if g.Type != "person" || g.Person == nil {
			continue
		}
		if g.Person.ID == personID {
			return g
		}
		if found := findPersonGroup(g.Children, personID); found != nil {
			return found
		}
	}
	return nil
}

// findTeam returns the pod with the given id and the leader that owns it
// (nil for a pod in the "Other" group).
func findTeam(groups []dto.SurveyCompletionGroupDTO, teamID string) (*dto.SurveyCompletionTeamDTO, *dto.SurveyCompletionGroupDTO) {
	for i := range groups {
		g := &groups[i]
		if g.Type == "other" {
			for j := range g.Teams {
				if g.Teams[j].TeamID == teamID {
					return &g.Teams[j], nil
				}
			}
			continue
		}
		for j := range g.DirectTeams {
			if g.DirectTeams[j].TeamID == teamID {
				return &g.DirectTeams[j], g
			}
		}
		if t, owner := findTeam(g.Children, teamID); t != nil {
			return t, owner
		}
	}
	return nil, nil
}

// planSurveyReminders decides who a reminder run mails. The recipients come
// from the survey-completion tree and teamLeadEmail (teamID -> team lead
// email), never from the request.
func planSurveyReminders(groups []dto.SurveyCompletionGroupDTO, teamLeadEmail map[string]string, req dto.SurveyReminderRequest) (*reminderPlan, error) {
	set := newRecipientSet(teamLeadEmail)

	switch req.Scope {
	case dto.SurveyReminderScopeTeam:
		if req.TeamID == "" {
			return nil, errInvalidReminderRequest
		}
		t, owner := findTeam(groups, req.TeamID)
		if t == nil {
			return nil, errReminderTargetNotFound
		}
		if !isRemindable(*t) {
			return nil, errNothingToRemind
		}
		if owner != nil && owner.Person != nil {
			set.add(owner.Person.ID, owner.Person.Name, owner.Person.Email, reminderRoleLeader, *t)
		}
		set.addTeamLead(*t)

	case dto.SurveyReminderScopePerson:
		if req.PersonID == "" {
			return nil, errInvalidReminderRequest
		}
		g := findPersonGroup(groups, req.PersonID)
		if g == nil {
			return nil, errReminderTargetNotFound
		}
		if len(set.addLeaderTree(*g)) == 0 {
			return nil, errNothingToRemind
		}

	case dto.SurveyReminderScopeOrg:
		podCount := 0
		for _, g := range groups {
			switch g.Type {
			case "person":
				podCount += len(set.addLeaderTree(g))
			case "other":
				for _, t := range g.Teams {
					if isRemindable(t) {
						podCount++
						set.addTeamLead(t)
					}
				}
			}
		}
		if podCount == 0 {
			return nil, errNothingToRemind
		}

	default:
		return nil, errInvalidReminderRequest
	}

	return set.plan(), nil
}

// memberLister is the part of team.Repository restrictPlanToTestPod needs.
type memberLister interface {
	FindMembers(ctx context.Context, teamID string) ([]*team.Member, error)
}

// restrictPlanToTestPod narrows a plan to one pod, for testing reminders
// without reaching real inboxes. A recipient stays only if they are that pod's
// team lead or one of its members, and what they are mailed about shrinks to
// that pod alone. Everyone else is dropped, including the leaders above the pod.
//
// It fails closed: if no pod has the name it returns errTestPodNotFound, and
// if several do it returns errTestPodAmbiguous. The caller must send nothing.
func restrictPlanToTestPod(ctx context.Context, plan *reminderPlan, podName string, teams []queries.SurveyCompletionTeam, members memberLister) (*reminderPlan, error) {
	podIDs := map[string]bool{}
	allowed := map[string]bool{}
	var matched []queries.SurveyCompletionTeam
	for _, t := range teams {
		if strings.EqualFold(strings.TrimSpace(t.TeamName), strings.TrimSpace(podName)) {
			podIDs[t.TeamID] = true
			matched = append(matched, t)
		}
	}
	if len(podIDs) == 0 {
		return nil, errTestPodNotFound
	}
	if len(podIDs) > 1 {
		return nil, errTestPodAmbiguous
	}
	for _, t := range matched {
		if t.TeamLeadID != "" {
			allowed[t.TeamLeadID] = true
		}
		ms, err := members.FindMembers(ctx, t.TeamID)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			allowed[m.UserID] = true
		}
	}

	out := &reminderPlan{}
	for _, r := range plan.Recipients {
		if r.UserID == "" || !allowed[r.UserID] {
			continue
		}
		var pods []dto.SurveyCompletionTeamDTO
		for _, p := range r.Pods {
			if podIDs[p.TeamID] {
				pods = append(pods, p)
			}
		}
		if len(pods) == 0 {
			continue
		}
		kept := *r
		kept.Pods = pods
		out.Recipients = append(out.Recipients, &kept)
	}
	for _, name := range plan.PodsWithoutTeamLead {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(podName)) {
			out.PodsWithoutTeamLead = append(out.PodsWithoutTeamLead, name)
		}
	}
	return out, nil
}

// surveyLabel is the name a survey type goes by in the email.
func surveyLabel(surveyType string) string {
	switch surveyType {
	case healthcheck.SurveyTypePostWorkshop:
		return "Post-Workshop Survey"
	case healthcheck.SurveyTypeIndividual:
		return "Individual Survey"
	default:
		return surveyType
	}
}

// toReminderPod turns the dashboard's data for a pod into the email's per-survey progress. Every
// number comes from that data: the Individual Survey is the members who completed out of those
// eligible, and the Post-Workshop Survey is one per team per period, so it reads 1 / 1 once
// done and 0 / 1 until then. A survey that does not apply (the pod opted out, nobody is
// eligible, or no post-workshop state is reported) is marked not applicable and shows as "—".
func toReminderPod(p dto.SurveyCompletionTeamDTO) email.ReminderPod {
	optedOut := p.Status == "opted_out"

	individual := email.SurveyProgress{
		Label:         surveyLabel(healthcheck.SurveyTypeIndividual),
		Completed:     p.Completed,
		Total:         p.Total,
		NotApplicable: optedOut || p.Total <= 0,
	}

	postWorkshop := email.SurveyProgress{
		Label:         surveyLabel(healthcheck.SurveyTypePostWorkshop),
		Total:         1,
		NotApplicable: optedOut || p.PostWorkshopCompleted == nil,
	}
	if p.PostWorkshopCompleted != nil && *p.PostWorkshopCompleted {
		postWorkshop.Completed = 1
	}

	pod := email.ReminderPod{
		Name:    p.TeamName,
		Status:  p.Status,
		Surveys: []email.SurveyProgress{individual, postWorkshop},
	}
	if p.TeamLeadName != nil {
		pod.TeamLeadName = *p.TeamLeadName
	}
	return pod
}

// sendReminderPlan mails every recipient in the plan and reports the outcome.
// A recipient with no email address is skipped and one the transport rejects
// is recorded as failed; neither stops the rest of the run.
//
// When the sender does not deliver (EMAIL_MODE=log), nothing is sent: each
// reminder is written to the log with who it was for, which pods, the survey,
// the reminder type and the subject, and the response says so.
func sendReminderPlan(ctx context.Context, sender email.Sender, assessmentPeriod, reminderType string, plan *reminderPlan) dto.SurveyReminderResponse {
	log := logger.Get()
	delivers := email.Delivers(sender)

	resp := dto.SurveyReminderResponse{
		AssessmentPeriod:    assessmentPeriod,
		Mode:                "send",
		SentTo:              []string{},
		LoggedFor:           []string{},
		SkippedNoEmail:      []string{},
		Failed:              []string{},
		PodsWithoutTeamLead: append([]string{}, plan.PodsWithoutTeamLead...),
	}

	if !delivers {
		resp.Mode = email.ModeLog
	}

	for _, r := range plan.Recipients {
		if r.Email == "" {
			resp.SkippedNoEmail = append(resp.SkippedNoEmail, r.Name)
			continue
		}

		if !delivers {
			podNames := make([]string, len(r.Pods))
			reminderPods := make([]email.ReminderPod, len(r.Pods))
			for i, p := range r.Pods {
				podNames[i] = p.TeamName
				reminderPods[i] = toReminderPod(p)
			}
			log.WithFields(map[string]interface{}{
				"recipient_name":  r.Name,
				"recipient_email": r.Email,
				"recipient_role":  r.Role,
				"pods":            strings.Join(podNames, ", "),
				"survey":          assessmentPeriod,
				"reminder_type":   reminderType,
				"subject":         email.SurveyReminderSubject(assessmentPeriod),
				"body": email.RenderSurveyReminderText(email.SurveyReminderEmailData{
					RecipientName:    r.Name,
					Role:             r.Role,
					AssessmentPeriod: assessmentPeriod,
					Pods:             reminderPods,
				}),
			}).Info("survey reminder (log mode): NOT sent")
			resp.LoggedFor = append(resp.LoggedFor, r.Name)
			continue
		}

		pods := make([]email.ReminderPod, len(r.Pods))
		for i, p := range r.Pods {
			pods[i] = toReminderPod(p)
		}
		body := email.RenderSurveyReminderEmail(email.SurveyReminderEmailData{
			RecipientName:    r.Name,
			Role:             r.Role,
			AssessmentPeriod: assessmentPeriod,
			Pods:             pods,
		})

		err := sender.SendHTML(ctx, r.Email, email.SurveyReminderSubject(assessmentPeriod), body)
		var partial *email.PartialDeliveryError
		if errors.As(err, &partial) {
			// Some redirect targets got it; a retry would duplicate those copies.
			log.WithField("user_id", r.UserID).WithError(err).Warn("survey reminder: partially delivered, counting as sent")
			err = nil
		}
		if err != nil {
			log.WithField("user_id", r.UserID).WithError(err).Warn("survey reminder: failed to send")
			resp.Failed = append(resp.Failed, r.Name)
			continue
		}
		resp.SentTo = append(resp.SentTo, r.Name)
	}

	resp.Sent = len(resp.SentTo)
	resp.Logged = len(resp.LoggedFor)
	return resp
}
