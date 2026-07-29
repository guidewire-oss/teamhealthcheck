// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E: Admin Settings Persistence", func() {
	var adminToken string

	BeforeEach(func() {
		var err error
		adminToken, err = loginAndGetToken("admin", "admin")
		Expect(err).NotTo(HaveOccurred(), "Failed to login as admin")
		Expect(adminToken).NotTo(BeEmpty())

		// Reset settings to defaults to avoid order-dependent tests
		_, err = db.Exec(`
			UPDATE app_settings SET
				email_notifications = false,
				slack_notifications = false,
				weekly_digest = false,
				retention_months = 12,
				updated_at = NOW()
			WHERE id = 1
		`)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Notification Settings CRUD", func() {
		Context("when admin reads notification settings", func() {
			It("should return current notification settings from database", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/settings/notifications", adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var settings struct {
					EmailEnabled       bool     `json:"emailEnabled"`
					SlackEnabled       bool     `json:"slackEnabled"`
					NotifyOnSubmission bool     `json:"notifyOnSubmission"`
					NotifyManagers     bool     `json:"notifyManagers"`
					ReminderDaysBefore int      `json:"reminderDaysBefore"`
					ReminderRecipients []string `json:"reminderRecipients"`
				}
				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &settings)
				Expect(err).NotTo(HaveOccurred())

				// Default values from migration
				Expect(settings.ReminderDaysBefore).To(Equal(7))
			})
		})

		Context("when admin updates notification settings", func() {
			It("should persist changes and return them on subsequent read", func() {
				By("Updating notification settings")
				updateBody := `{"emailEnabled":true,"slackEnabled":true,"notifyOnSubmission":true}`
				resp, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/settings/notifications", adminToken, strings.NewReader(updateBody))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				By("Reading settings back to verify persistence")
				resp2, err := makeAuthenticatedRequest("GET", "/api/v1/admin/settings/notifications", adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp2.Body.Close()
				Expect(resp2.StatusCode).To(Equal(http.StatusOK))

				var settings struct {
					EmailEnabled       bool `json:"emailEnabled"`
					SlackEnabled       bool `json:"slackEnabled"`
					NotifyOnSubmission bool `json:"notifyOnSubmission"`
				}
				body, err := io.ReadAll(resp2.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &settings)
				Expect(err).NotTo(HaveOccurred())

				Expect(settings.EmailEnabled).To(BeTrue(), "Email enabled should be persisted as true")
				Expect(settings.SlackEnabled).To(BeTrue(), "Slack enabled should be persisted as true")
				Expect(settings.NotifyOnSubmission).To(BeTrue(), "Notify on submission should be persisted as true")

				By("Toggling settings off")
				updateBody2 := `{"emailEnabled":false,"slackEnabled":false,"notifyOnSubmission":false}`
				resp3, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/settings/notifications", adminToken, strings.NewReader(updateBody2))
				Expect(err).NotTo(HaveOccurred())
				defer resp3.Body.Close()
				Expect(resp3.StatusCode).To(Equal(http.StatusOK))

				By("Verifying toggled-off settings persist")
				resp4, err := makeAuthenticatedRequest("GET", "/api/v1/admin/settings/notifications", adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp4.Body.Close()

				var settings2 struct {
					EmailEnabled       bool `json:"emailEnabled"`
					SlackEnabled       bool `json:"slackEnabled"`
					NotifyOnSubmission bool `json:"notifyOnSubmission"`
				}
				body2, err := io.ReadAll(resp4.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body2, &settings2)
				Expect(err).NotTo(HaveOccurred())

				Expect(settings2.EmailEnabled).To(BeFalse())
				Expect(settings2.SlackEnabled).To(BeFalse())
				Expect(settings2.NotifyOnSubmission).To(BeFalse())
			})
		})
	})

	Describe("Retention Policy CRUD", func() {
		Context("when admin reads retention policy", func() {
			It("should return current retention settings from database", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/settings/retention", adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var policy struct {
					KeepSessionsMonths int  `json:"keepSessionsMonths"`
					ArchiveEnabled     bool `json:"archiveEnabled"`
					AnonymizeAfterDays int  `json:"anonymizeAfterDays"`
				}
				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &policy)
				Expect(err).NotTo(HaveOccurred())

				Expect(policy.KeepSessionsMonths).To(BeNumerically(">", 0))
			})
		})

		Context("when admin updates retention policy", func() {
			It("should persist retention months change", func() {
				By("Updating retention to 24 months")
				updateBody := `{"keepSessionsMonths":24}`
				resp, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/settings/retention", adminToken, strings.NewReader(updateBody))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				By("Reading retention policy back")
				resp2, err := makeAuthenticatedRequest("GET", "/api/v1/admin/settings/retention", adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp2.Body.Close()
				Expect(resp2.StatusCode).To(Equal(http.StatusOK))

				var policy struct {
					KeepSessionsMonths int `json:"keepSessionsMonths"`
					AnonymizeAfterDays int `json:"anonymizeAfterDays"`
				}
				body, err := io.ReadAll(resp2.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &policy)
				Expect(err).NotTo(HaveOccurred())

				Expect(policy.KeepSessionsMonths).To(Equal(24))
				Expect(policy.AnonymizeAfterDays).To(Equal(24 * 30))
			})

			It("should reject invalid retention months", func() {
				updateBody := `{"keepSessionsMonths":0}`
				resp, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/settings/retention", adminToken, strings.NewReader(updateBody))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})

			It("should reject retention months over 120", func() {
				updateBody := `{"keepSessionsMonths":121}`
				resp, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/settings/retention", adminToken, strings.NewReader(updateBody))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Describe("Settings isolation between notification and retention", func() {
		It("should not overwrite notification settings when updating retention", func() {
			By("Setting notification settings to known values")
			notifBody := `{"emailEnabled":true,"slackEnabled":false,"notifyOnSubmission":true}`
			resp, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/settings/notifications", adminToken, strings.NewReader(notifBody))
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			By("Updating retention policy")
			retBody := `{"keepSessionsMonths":6}`
			resp2, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/settings/retention", adminToken, strings.NewReader(retBody))
			Expect(err).NotTo(HaveOccurred())
			resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusOK))

			By("Verifying notification settings are unchanged")
			resp3, err := makeAuthenticatedRequest("GET", "/api/v1/admin/settings/notifications", adminToken, nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp3.Body.Close()

			var settings struct {
				EmailEnabled       bool `json:"emailEnabled"`
				SlackEnabled       bool `json:"slackEnabled"`
				NotifyOnSubmission bool `json:"notifyOnSubmission"`
			}
			body, err := io.ReadAll(resp3.Body)
			Expect(err).NotTo(HaveOccurred())
			json.Unmarshal(body, &settings)

			Expect(settings.EmailEnabled).To(BeTrue(), "Email setting should be preserved after retention update")
			Expect(settings.SlackEnabled).To(BeFalse(), "Slack setting should be preserved after retention update")
			Expect(settings.NotifyOnSubmission).To(BeTrue(), "Notify setting should be preserved after retention update")
		})
	})
})
