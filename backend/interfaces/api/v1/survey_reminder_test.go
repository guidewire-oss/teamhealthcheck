package v1

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agopalakrishnan/teams360/backend/application/queries"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
)

// fakeReminderSender records what would have been mailed.
type fakeReminderSender struct {
	sent   []sentReminder
	failTo map[string]bool
}

type sentReminder struct{ to, subject, body string }

func (f *fakeReminderSender) SendHTML(_ context.Context, to, subject, body string) error {
	if f.failTo[to] {
		return errors.New("smtp: connection refused")
	}
	f.sent = append(f.sent, sentReminder{to, subject, body})
	return nil
}

func (f *fakeReminderSender) SendPasswordResetEmail(context.Context, string, string) error {
	return nil
}

var _ = Describe("survey reminders", func() {
	// sd-a is a root with two children (dir-b, sm-c); dir-b has mgr-d under it.
	// sd-e is a second root. Pods:
	//   alpha   sd-a   complete
	//   eta     sd-a   opted out
	//   beta    dir-b  in progress
	//   gamma   mgr-d  complete          (so mgr-d's whole branch is done)
	//   delta   sm-c   not started
	//   epsilon sd-e   complete
	//   nolead  sd-e   not started, no team lead
	//   orphan  none   in progress       (lands in "Other")
	persons := []queries.PersonNode{
		{UserID: "sd-a", Name: "Senior Director A", Email: "sda@x.com", LevelID: "senior-director", LevelName: "Senior Director"},
		{UserID: "dir-b", Name: "Director B", Email: "dirb@x.com", LevelID: "director", LevelName: "Director", ParentID: "sd-a"},
		{UserID: "sm-c", Name: "Senior Manager C", Email: "smc@x.com", LevelID: "senior-manager", LevelName: "Senior Manager", ParentID: "sd-a"},
		{UserID: "mgr-d", Name: "Manager D", Email: "mgrd@x.com", LevelID: "manager", LevelName: "Manager", ParentID: "dir-b"},
		{UserID: "sd-e", Name: "Senior Director E", Email: "sde@x.com", LevelID: "senior-director", LevelName: "Senior Director"},
	}

	teams := func(extra ...queries.SurveyCompletionTeam) []queries.SurveyCompletionTeam {
		return append([]queries.SurveyCompletionTeam{
			{TeamID: "alpha", TeamName: "Team Alpha", OwnerUserID: "sd-a", TeamLeadID: "tl1", TeamLeadName: "Lead Alpha", TeamLeadEmail: "tl1@x.com", Total: 3, Completed: 3, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			{TeamID: "eta", TeamName: "Team Eta", OwnerUserID: "sd-a", TeamLeadID: "tl-eta", TeamLeadName: "Lead Eta", TeamLeadEmail: "tleta@x.com", Total: 2, HealthCheckEnabled: false},
			{TeamID: "beta", TeamName: "Team Beta", OwnerUserID: "dir-b", TeamLeadID: "tl2", TeamLeadName: "Lead Beta", TeamLeadEmail: "tl2@x.com", Total: 4, Completed: 1, HealthCheckEnabled: true},
			{TeamID: "gamma", TeamName: "Team Gamma", OwnerUserID: "mgr-d", TeamLeadID: "tl3", TeamLeadName: "Lead Gamma", TeamLeadEmail: "tl3@x.com", Total: 2, Completed: 2, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			{TeamID: "delta", TeamName: "Team Delta", OwnerUserID: "sm-c", TeamLeadID: "tl4", TeamLeadName: "Lead Delta", TeamLeadEmail: "tl4@x.com", Total: 2, HealthCheckEnabled: true},
			{TeamID: "epsilon", TeamName: "Team Epsilon", OwnerUserID: "sd-e", TeamLeadID: "tl6", TeamLeadName: "Lead Epsilon", TeamLeadEmail: "tl6@x.com", Total: 2, Completed: 2, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			{TeamID: "nolead", TeamName: "Team NoLead", OwnerUserID: "sd-e", Total: 3, HealthCheckEnabled: true},
			{TeamID: "orphan", TeamName: "Team Orphan", TeamLeadID: "tl5", TeamLeadName: "Lead Orphan", TeamLeadEmail: "tl5@x.com", Total: 2, Completed: 1, HealthCheckEnabled: true},
		}, extra...)
	}

	plan := func(req dto.SurveyReminderRequest, extra ...queries.SurveyCompletionTeam) (*reminderPlan, error) {
		overview := &queries.SurveyCompletionOverview{Persons: persons, Teams: teams(extra...)}
		leadEmail := map[string]string{}
		for _, t := range overview.Teams {
			leadEmail[t.TeamID] = t.TeamLeadEmail
		}
		return planSurveyReminders(buildSurveyCompletionResponse(overview).Groups, leadEmail, req)
	}

	emails := func(p *reminderPlan) []string {
		out := []string{}
		for _, r := range p.Recipients {
			out = append(out, r.Email)
		}
		return out
	}

	podNames := func(r *reminderRecipient) []string {
		out := []string{}
		for _, p := range r.Pods {
			out = append(out, p.TeamName)
		}
		return out
	}

	byEmail := func(p *reminderPlan, addr string) *reminderRecipient {
		for _, r := range p.Recipients {
			if r.Email == addr {
				return r
			}
		}
		return nil
	}

	Describe("scope team", func() {
		It("reminds the pod's owner and its team lead, and nobody above the owner", func() {
			p, err := plan(dto.SurveyReminderRequest{Scope: "team", TeamID: "beta"})

			Expect(err).NotTo(HaveOccurred())
			Expect(emails(p)).To(ConsistOf("dirb@x.com", "tl2@x.com"))
			Expect(byEmail(p, "dirb@x.com").Role).To(Equal("leader"))
			Expect(byEmail(p, "tl2@x.com").Role).To(Equal("team lead"))
		})

		It("reminds only the team lead for a pod in Other", func() {
			p, err := plan(dto.SurveyReminderRequest{Scope: "team", TeamID: "orphan"})

			Expect(err).NotTo(HaveOccurred())
			Expect(emails(p)).To(ConsistOf("tl5@x.com"))
		})

		It("refuses a pod that is already complete or opted out", func() {
			for _, id := range []string{"alpha", "eta"} {
				_, err := plan(dto.SurveyReminderRequest{Scope: "team", TeamID: id})
				Expect(err).To(MatchError(errNothingToRemind), id)
			}
		})

		It("reports an unknown pod as not found", func() {
			_, err := plan(dto.SurveyReminderRequest{Scope: "team", TeamID: "nope"})
			Expect(err).To(MatchError(errReminderTargetNotFound))
		})

		It("reports a pod with no team lead instead of dropping it silently", func() {
			p, err := plan(dto.SurveyReminderRequest{Scope: "team", TeamID: "nolead"})

			Expect(err).NotTo(HaveOccurred())
			Expect(emails(p)).To(ConsistOf("sde@x.com"))
			Expect(p.PodsWithoutTeamLead).To(ConsistOf("Team NoLead"))
		})
	})

	Describe("scope person", func() {
		It("reminds the leader, each descendant leader with a lagging pod, and those pods' team leads", func() {
			p, err := plan(dto.SurveyReminderRequest{Scope: "person", PersonID: "sd-a"})

			Expect(err).NotTo(HaveOccurred())
			// mgr-d's branch is all complete, so mgr-d and tl3 are left out.
			Expect(emails(p)).To(ConsistOf("sda@x.com", "dirb@x.com", "smc@x.com", "tl2@x.com", "tl4@x.com"))
			Expect(podNames(byEmail(p, "sda@x.com"))).To(ConsistOf("Team Beta", "Team Delta"))
			Expect(podNames(byEmail(p, "dirb@x.com"))).To(ConsistOf("Team Beta"))
			Expect(podNames(byEmail(p, "smc@x.com"))).To(ConsistOf("Team Delta"))
			Expect(podNames(byEmail(p, "tl2@x.com"))).To(ConsistOf("Team Beta"))
		})

		It("refuses a leader whose whole branch is complete", func() {
			_, err := plan(dto.SurveyReminderRequest{Scope: "person", PersonID: "mgr-d"})
			Expect(err).To(MatchError(errNothingToRemind))
		})

		It("reports an unknown leader as not found", func() {
			_, err := plan(dto.SurveyReminderRequest{Scope: "person", PersonID: "nope"})
			Expect(err).To(MatchError(errReminderTargetNotFound))
		})
	})

	Describe("scope org", func() {
		It("reminds every leader with a lagging pod and every lagging pod's team lead, including Other", func() {
			p, err := plan(dto.SurveyReminderRequest{Scope: "org"})

			Expect(err).NotTo(HaveOccurred())
			Expect(emails(p)).To(ConsistOf(
				"sda@x.com", "dirb@x.com", "smc@x.com", "sde@x.com",
				"tl2@x.com", "tl4@x.com", "tl5@x.com",
			))
			Expect(p.PodsWithoutTeamLead).To(ConsistOf("Team NoLead"))
		})

		It("never mails a leader or team lead whose pods are all complete", func() {
			p, err := plan(dto.SurveyReminderRequest{Scope: "org"})

			Expect(err).NotTo(HaveOccurred())
			Expect(emails(p)).NotTo(ContainElements("mgrd@x.com", "tl1@x.com", "tl3@x.com", "tl6@x.com", "tleta@x.com"))
		})
	})

	It("sends one email to someone who is both a leader and a team lead, listing all their pods", func() {
		// dir-b also leads Team Zed.
		zed := queries.SurveyCompletionTeam{TeamID: "zed", TeamName: "Team Zed", OwnerUserID: "sd-a", TeamLeadID: "dir-b", TeamLeadName: "Director B", TeamLeadEmail: "dirb@x.com", Total: 2, Completed: 1, HealthCheckEnabled: true}

		p, err := plan(dto.SurveyReminderRequest{Scope: "org"}, zed)

		Expect(err).NotTo(HaveOccurred())
		count := 0
		for _, r := range p.Recipients {
			if r.Email == "dirb@x.com" {
				count++
			}
		}
		Expect(count).To(Equal(1), "one person, one email")
		Expect(byEmail(p, "dirb@x.com").Role).To(Equal("leader"))
		Expect(podNames(byEmail(p, "dirb@x.com"))).To(ConsistOf("Team Beta", "Team Zed"))
	})

	It("rejects an unknown scope and a missing target id", func() {
		for _, req := range []dto.SurveyReminderRequest{
			{Scope: ""},
			{Scope: "everyone"},
			{Scope: "team"},
			{Scope: "person"},
		} {
			_, err := plan(req)
			Expect(err).To(MatchError(errInvalidReminderRequest), req.Scope)
		}
	})

	Describe("sendReminderPlan", func() {
		mkPlan := func(rs ...*reminderRecipient) *reminderPlan { return &reminderPlan{Recipients: rs} }
		pod := dto.SurveyCompletionTeamDTO{TeamID: "t1", TeamName: "Team <One>", Completed: 1, Total: 4, Status: "in_progress"}

		It("mails each recipient once with the period in the subject and the pods in the body", func() {
			sender := &fakeReminderSender{}

			resp := sendReminderPlan(context.Background(), sender, "2026 - 1st Half", "org", mkPlan(
				&reminderRecipient{UserID: "u1", Name: "Ann", Email: "ann@x.com", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{pod}},
				&reminderRecipient{UserID: "u2", Name: "Bob", Email: "bob@x.com", Role: "team lead", Pods: []dto.SurveyCompletionTeamDTO{pod}},
			))

			Expect(resp.Sent).To(Equal(2))
			Expect(resp.SentTo).To(Equal([]string{"Ann", "Bob"}))
			Expect(sender.sent).To(HaveLen(2))
			Expect(sender.sent[0].to).To(Equal("ann@x.com"))
			Expect(sender.sent[0].subject).To(ContainSubstring("2026 - 1st Half"))
			Expect(sender.sent[0].body).To(ContainSubstring("Team &lt;One&gt;"), "pod names must be HTML-escaped")
			Expect(sender.sent[0].body).NotTo(ContainSubstring("Team <One>"))
			Expect(sender.sent[0].body).To(ContainSubstring("1 / 4"))
		})

		It("carries the pod's survey progress, status and team lead into the email", func() {
			sender := &fakeReminderSender{}
			lead := "Yash <Jambotkar>"
			notDone := false
			full := dto.SurveyCompletionTeamDTO{
				TeamID: "t9", TeamName: "HopeFarm", Completed: 1, Total: 5, Status: "in_progress",
				PostWorkshopCompleted: &notDone, TeamLeadName: &lead,
			}

			sendReminderPlan(context.Background(), sender, "2026 Q4", "team", mkPlan(
				&reminderRecipient{UserID: "u1", Name: "Ann", Email: "ann@x.com", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{full}},
			))

			body := sender.sent[0].body
			Expect(body).To(ContainSubstring("HopeFarm"))
			Expect(body).To(ContainSubstring("Individual Survey"))
			Expect(body).To(ContainSubstring("1 / 5"))
			Expect(body).To(ContainSubstring("Post-Workshop Survey"))
			Expect(body).To(ContainSubstring("0 / 1"))
			Expect(body).To(ContainSubstring("In progress"))
			Expect(body).To(ContainSubstring("Yash &lt;Jambotkar&gt;"), "the team lead, escaped")
		})

		It("skips a recipient with no email and carries on", func() {
			sender := &fakeReminderSender{}

			resp := sendReminderPlan(context.Background(), sender, "2026 - 1st Half", "org", mkPlan(
				&reminderRecipient{UserID: "u1", Name: "No Email", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{pod}},
				&reminderRecipient{UserID: "u2", Name: "Bob", Email: "bob@x.com", Role: "team lead", Pods: []dto.SurveyCompletionTeamDTO{pod}},
			))

			Expect(resp.SkippedNoEmail).To(Equal([]string{"No Email"}))
			Expect(resp.SentTo).To(Equal([]string{"Bob"}))
		})

		It("records a rejected recipient as failed and still mails the rest", func() {
			sender := &fakeReminderSender{failTo: map[string]bool{"ann@x.com": true}}

			resp := sendReminderPlan(context.Background(), sender, "2026 - 1st Half", "org", mkPlan(
				&reminderRecipient{UserID: "u1", Name: "Ann", Email: "ann@x.com", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{pod}},
				&reminderRecipient{UserID: "u2", Name: "Bob", Email: "bob@x.com", Role: "team lead", Pods: []dto.SurveyCompletionTeamDTO{pod}},
			))

			Expect(resp.Failed).To(Equal([]string{"Ann"}))
			Expect(resp.SentTo).To(Equal([]string{"Bob"}))
			Expect(resp.Sent).To(Equal(1))
		})
	})
})

// logOnlySender is a sender that, like email.LogSender, delivers nothing.
type logOnlySender struct{ fakeReminderSender }

func (*logOnlySender) Delivers() bool { return false }

type fakeMembers struct {
	byTeam map[string][]string
	err    error
}

func (f fakeMembers) FindMembers(_ context.Context, teamID string) ([]*team.Member, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []*team.Member
	for _, id := range f.byTeam[teamID] {
		out = append(out, &team.Member{UserID: id})
	}
	return out, nil
}

// These cover the guard that keeps a test run away from real inboxes.
var _ = Describe("survey reminder test-pod restriction and log mode", func() {
	podBeta := dto.SurveyCompletionTeamDTO{TeamID: "beta", TeamName: "Hopefarm", Status: "in_progress", Completed: 1, Total: 4}
	podDelta := dto.SurveyCompletionTeamDTO{TeamID: "delta", TeamName: "Team Delta", Status: "not_started", Total: 2}

	teams := []queries.SurveyCompletionTeam{
		{TeamID: "beta", TeamName: "Hopefarm", TeamLeadID: "tl-beta"},
		{TeamID: "delta", TeamName: "Team Delta", TeamLeadID: "tl-delta"},
	}

	// Everyone a full run would mail: the leaders above Hopefarm, its team
	// lead, and the other pod's people.
	fullPlan := func() *reminderPlan {
		return &reminderPlan{
			Recipients: []*reminderRecipient{
				{UserID: "sd-a", Name: "Senior Director", Email: "sd@x.com", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{podBeta, podDelta}},
				{UserID: "dir-b", Name: "Director", Email: "dir@x.com", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{podBeta}},
				{UserID: "tl-beta", Name: "Beta Lead", Email: "tlbeta@x.com", Role: "team lead", Pods: []dto.SurveyCompletionTeamDTO{podBeta}},
				{UserID: "tl-delta", Name: "Delta Lead", Email: "tldelta@x.com", Role: "team lead", Pods: []dto.SurveyCompletionTeamDTO{podDelta}},
			},
			PodsWithoutTeamLead: []string{"Some Other Pod"},
		}
	}

	addrs := func(p *reminderPlan) []string {
		out := []string{}
		for _, r := range p.Recipients {
			out = append(out, r.Email)
		}
		return out
	}

	Describe("restrictPlanToTestPod", func() {
		It("keeps only the test pod's team lead and drops every leader above it and every other pod", func() {
			p, err := restrictPlanToTestPod(context.Background(), fullPlan(), "Hopefarm", teams, fakeMembers{})

			Expect(err).NotTo(HaveOccurred())
			Expect(addrs(p)).To(Equal([]string{"tlbeta@x.com"}))
		})

		It("also keeps a recipient who is a member of the test pod, trimming their pods to it", func() {
			members := fakeMembers{byTeam: map[string][]string{"beta": {"dir-b", "sd-a"}}}

			p, err := restrictPlanToTestPod(context.Background(), fullPlan(), "Hopefarm", teams, members)

			Expect(err).NotTo(HaveOccurred())
			Expect(addrs(p)).To(ConsistOf("sd@x.com", "dir@x.com", "tlbeta@x.com"))
			for _, r := range p.Recipients {
				Expect(r.Pods).To(HaveLen(1), r.Email)
				Expect(r.Pods[0].TeamID).To(Equal("beta"), "pods outside the test pod must be removed from the email")
			}
		})

		It("drops a test pod member whose lagging pods do not include the test pod", func() {
			// tl-delta is a member of Hopefarm but is only being reminded about Team Delta.
			members := fakeMembers{byTeam: map[string][]string{"beta": {"tl-delta"}}}

			p, err := restrictPlanToTestPod(context.Background(), fullPlan(), "Hopefarm", teams, members)

			Expect(err).NotTo(HaveOccurred())
			Expect(addrs(p)).NotTo(ContainElement("tldelta@x.com"))
		})

		It("matches the pod name case-insensitively and ignores surrounding spaces", func() {
			p, err := restrictPlanToTestPod(context.Background(), fullPlan(), "  hopeFARM ", teams, fakeMembers{})

			Expect(err).NotTo(HaveOccurred())
			Expect(addrs(p)).To(Equal([]string{"tlbeta@x.com"}))
		})

		It("keeps only the test pod in the list of pods with no team lead", func() {
			// A genuinely leadless Hopefarm: no team lead, and nobody mailed for it.
			leadless := []queries.SurveyCompletionTeam{{TeamID: "beta", TeamName: "Hopefarm"}}
			plan := &reminderPlan{
				Recipients:          []*reminderRecipient{{UserID: "dir-b", Name: "Director", Email: "dir@x.com", Pods: []dto.SurveyCompletionTeamDTO{podBeta}}},
				PodsWithoutTeamLead: []string{"Hopefarm", "Some Other Pod"},
			}
			p, err := restrictPlanToTestPod(context.Background(), plan, "Hopefarm", leadless, fakeMembers{})

			Expect(err).NotTo(HaveOccurred())
			Expect(p.PodsWithoutTeamLead).To(Equal([]string{"Hopefarm"}))
		})

		It("fails closed when more than one pod has that name", func() {
			dup := append([]queries.SurveyCompletionTeam{{TeamID: "beta2", TeamName: "Hopefarm", TeamLeadID: "tl-x"}}, teams...)
			p, err := restrictPlanToTestPod(context.Background(), fullPlan(), "Hopefarm", dup, fakeMembers{})

			Expect(err).To(MatchError(errTestPodAmbiguous))
			Expect(p).To(BeNil())
		})

		It("fails closed when no pod has that name", func() {
			p, err := restrictPlanToTestPod(context.Background(), fullPlan(), "Nonexistent", teams, fakeMembers{})

			Expect(err).To(MatchError(errTestPodNotFound))
			Expect(p).To(BeNil(), "no plan may come back that a caller could send")
		})

		It("fails closed when the membership lookup fails", func() {
			p, err := restrictPlanToTestPod(context.Background(), fullPlan(), "Hopefarm", teams, fakeMembers{err: errors.New("db down")})

			Expect(err).To(HaveOccurred())
			Expect(p).To(BeNil())
		})

		It("never mails a recipient without a user id", func() {
			plan := &reminderPlan{Recipients: []*reminderRecipient{
				{UserID: "", Name: "Anon", Email: "anon@x.com", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{podBeta}},
			}}

			p, err := restrictPlanToTestPod(context.Background(), plan, "Hopefarm", teams, fakeMembers{})

			Expect(err).NotTo(HaveOccurred())
			Expect(p.Recipients).To(BeEmpty())
		})
	})

	Describe("sendReminderPlan in log mode", func() {
		It("sends nothing, records who it would have reached, and says it is in log mode", func() {
			sender := &logOnlySender{}

			resp := sendReminderPlan(context.Background(), sender, "2026 - 1st Half", "team", fullPlan())

			Expect(sender.sent).To(BeEmpty(), "log mode must never call the mail transport")
			Expect(resp.Mode).To(Equal("log"))
			Expect(resp.Sent).To(BeZero())
			Expect(resp.SentTo).To(BeEmpty())
			Expect(resp.Logged).To(Equal(4))
			Expect(resp.LoggedFor).To(ConsistOf("Senior Director", "Director", "Beta Lead", "Delta Lead"))
		})

		It("still reports a recipient with no email as skipped", func() {
			sender := &logOnlySender{}
			plan := &reminderPlan{Recipients: []*reminderRecipient{
				{UserID: "u1", Name: "No Email", Role: "leader", Pods: []dto.SurveyCompletionTeamDTO{podBeta}},
			}}

			resp := sendReminderPlan(context.Background(), sender, "2026 - 1st Half", "org", plan)

			Expect(resp.SkippedNoEmail).To(Equal([]string{"No Email"}))
			Expect(resp.Logged).To(BeZero())
			Expect(sender.sent).To(BeEmpty())
		})

		It("reports normal sending as mode send", func() {
			sender := &fakeReminderSender{}

			resp := sendReminderPlan(context.Background(), sender, "2026 - 1st Half", "org", fullPlan())

			Expect(resp.Mode).To(Equal("send"))
			Expect(resp.Sent).To(Equal(4))
			Expect(resp.Logged).To(BeZero())
		})
	})
})

var _ = Describe("toReminderPod", func() {
	yes, no := true, false

	surveys := func(p dto.SurveyCompletionTeamDTO) map[string]string {
		out := map[string]string{}
		for _, sv := range toReminderPod(p).Surveys {
			if sv.NotApplicable {
				out[sv.Label] = "—"
			} else {
				out[sv.Label] = fmt.Sprintf("%d / %d", sv.Completed, sv.Total)
			}
		}
		return out
	}

	It("reports the individual survey as members completed over members eligible", func() {
		got := surveys(dto.SurveyCompletionTeamDTO{Completed: 3, Total: 8, Status: "in_progress", PostWorkshopCompleted: &no})

		Expect(got).To(HaveKeyWithValue("Individual Survey", "3 / 8"))
	})

	It("reports the post-workshop survey as one per team: 0 / 1 until done, 1 / 1 after", func() {
		Expect(surveys(dto.SurveyCompletionTeamDTO{Completed: 2, Total: 2, Status: "in_progress", PostWorkshopCompleted: &no})).
			To(HaveKeyWithValue("Post-Workshop Survey", "0 / 1"))
		Expect(surveys(dto.SurveyCompletionTeamDTO{Completed: 2, Total: 2, Status: "complete", PostWorkshopCompleted: &yes})).
			To(HaveKeyWithValue("Post-Workshop Survey", "1 / 1"))
	})

	It("marks every survey not applicable when the pod has opted out", func() {
		got := surveys(dto.SurveyCompletionTeamDTO{Completed: 4, Total: 4, Status: "opted_out", PostWorkshopCompleted: nil})

		Expect(got).To(HaveKeyWithValue("Individual Survey", "—"))
		Expect(got).To(HaveKeyWithValue("Post-Workshop Survey", "—"))
	})

	It("marks the individual survey not applicable when nobody is eligible for it", func() {
		got := surveys(dto.SurveyCompletionTeamDTO{Completed: 0, Total: 0, Status: "not_started", PostWorkshopCompleted: &no})

		Expect(got).To(HaveKeyWithValue("Individual Survey", "—"))
		Expect(got).To(HaveKeyWithValue("Post-Workshop Survey", "0 / 1"))
	})

	It("marks the post-workshop survey not applicable when no state is reported for it", func() {
		got := surveys(dto.SurveyCompletionTeamDTO{Completed: 1, Total: 2, Status: "in_progress", PostWorkshopCompleted: nil})

		Expect(got).To(HaveKeyWithValue("Post-Workshop Survey", "—"))
	})

	It("keeps the pod's name, status and team lead", func() {
		lead := "Yash Jambotkar"
		pod := toReminderPod(dto.SurveyCompletionTeamDTO{TeamName: "HopeFarm", Status: "in_progress", TeamLeadName: &lead})

		Expect(pod.Name).To(Equal("HopeFarm"))
		Expect(pod.Status).To(Equal("in_progress"))
		Expect(pod.TeamLeadName).To(Equal("Yash Jambotkar"))
	})
})
