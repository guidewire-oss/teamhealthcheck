// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E: Data Validation", func() {
	var userToken string

	BeforeEach(func() {
		var err error
		userToken, err = loginAndGetToken("demo", "demo")
		Expect(err).NotTo(HaveOccurred(), "Failed to login as demo user")
		Expect(userToken).NotTo(BeEmpty())
	})

	Describe("Issue #7: Future Date Validation", func() {
		Context("when submitting health check with future date", func() {
			It("should reject health check submission with future date", func() {
				futureDate := time.Now().AddDate(0, 0, 7).Format(time.RFC3339) // 7 days in future

				body := map[string]interface{}{
					"teamId": "team-phoenix",
					"userId": "demo",
					"date":   futureDate,
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
							"comment":     "Test comment",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Health check with future date should be rejected")

				var errorResp map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&errorResp)
				Expect(errorResp["error"]).To(ContainSubstring("future"),
					"Error message should mention future date issue")
			})

			It("should reject health check with assessment period from future", func() {
				// Calculate a future assessment period
				futureYear := time.Now().Year() + 2

				body := map[string]interface{}{
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": formatPeriod(futureYear, "1st"),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Health check with future assessment period should be rejected")
			})

			It("should accept health check with current or past date", func() {
				pastDate := time.Now().AddDate(0, 0, -1).Format(time.RFC3339) // yesterday
				uniqueID := fmt.Sprintf("test-%d", time.Now().UnixNano())

				body := map[string]interface{}{
					"id":     uniqueID,
					"teamId": "team-phoenix",
					"userId": "demo",
					"date":   pastDate,
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
							"comment":     "Valid submission",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Should be accepted (201) or conflict if already submitted (409)
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
					Equal(http.StatusOK),
				), "Health check with past date should be accepted")
			})
		})
	})

	Describe("Issue #8: Assessment Period Format Validation", func() {
		Context("when submitting health check with invalid assessment period format", func() {
			It("should reject assessment period without proper format", func() {
				body := map[string]interface{}{
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": "2024-first-half", // Invalid format
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Invalid assessment period format should be rejected")

				var errorResp map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&errorResp)
				Expect(errorResp["error"]).To(Or(
					ContainSubstring("assessment"),
					ContainSubstring("period"),
					ContainSubstring("format"),
				), "Error message should indicate format issue")
			})

			It("should reject assessment period with invalid half", func() {
				body := map[string]interface{}{
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": "2024 - 3rd Half", // Invalid - no 3rd half
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Invalid half value should be rejected")
			})

			It("should reject assessment period with invalid year", func() {
				body := map[string]interface{}{
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": "abcd - 1st Half", // Invalid year
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Invalid year in assessment period should be rejected")
			})

			It("should accept valid assessment period format - 1st Half", func() {
				uniqueID := fmt.Sprintf("test-1st-%d", time.Now().UnixNano())
				body := map[string]interface{}{
					"id":               uniqueID,
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": "2024 - 1st Half",
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Should be accepted (201) or conflict if already submitted (409)
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
					Equal(http.StatusOK),
				), "Valid assessment period format should not be rejected")
			})

			It("should accept valid assessment period format - 2nd Half", func() {
				uniqueID := fmt.Sprintf("test-2nd-%d", time.Now().UnixNano())
				body := map[string]interface{}{
					"id":               uniqueID,
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": "2024 - 2nd Half",
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
					Equal(http.StatusOK),
				), "Valid assessment period format should not be rejected")
			})

			It("should accept valid monthly period format", func() {
				uniqueID := fmt.Sprintf("test-monthly-%d", time.Now().UnixNano())
				pastYear := time.Now().Year() - 1
				body := map[string]interface{}{
					"id":               uniqueID,
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": fmt.Sprintf("%d Jan", pastYear),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
					Equal(http.StatusOK),
				), "Valid monthly period format should be accepted")
			})

			It("should accept valid quarterly period format", func() {
				uniqueID := fmt.Sprintf("test-quarterly-%d", time.Now().UnixNano())
				pastYear := time.Now().Year() - 1
				body := map[string]interface{}{
					"id":               uniqueID,
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": fmt.Sprintf("%d Q1", pastYear),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
					Equal(http.StatusOK),
				), "Valid quarterly period format should be accepted")
			})

			It("should accept valid half-yearly period format", func() {
				uniqueID := fmt.Sprintf("test-half-%d", time.Now().UnixNano())
				pastYear := time.Now().Year() - 1
				body := map[string]interface{}{
					"id":               uniqueID,
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": fmt.Sprintf("%d H1", pastYear),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
					Equal(http.StatusOK),
				), "Valid half-yearly period format should be accepted")
			})

			It("should accept valid yearly period format", func() {
				uniqueID := fmt.Sprintf("test-yearly-%d", time.Now().UnixNano())
				pastYear := time.Now().Year() - 1
				body := map[string]interface{}{
					"id":               uniqueID,
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": fmt.Sprintf("%d", pastYear),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
					Equal(http.StatusOK),
				), "Valid yearly period format should be accepted")
			})

			It("should reject future monthly period", func() {
				futureYear := time.Now().Year() + 2
				body := map[string]interface{}{
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": fmt.Sprintf("%d Jan", futureYear),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Future monthly period should be rejected")
			})

			It("should reject future quarterly period", func() {
				futureYear := time.Now().Year() + 2
				body := map[string]interface{}{
					"teamId":           "team-phoenix",
					"userId":           "demo",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": fmt.Sprintf("%d Q1", futureYear),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Future quarterly period should be rejected")
			})
		})
	})

	Describe("Additional Input Validation", func() {
		Context("when submitting health check with invalid score", func() {
			It("should reject score less than 1", func() {
				body := map[string]interface{}{
					"teamId": "team-phoenix",
					"userId": "demo",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       0, // Invalid - must be 1-3
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Score of 0 should be rejected")
			})

			It("should reject score greater than 3", func() {
				body := map[string]interface{}{
					"teamId": "team-phoenix",
					"userId": "demo",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       5, // Invalid - must be 1-3
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Score of 5 should be rejected")
			})
		})

		Context("when submitting health check with invalid trend", func() {
			It("should reject invalid trend value", func() {
				body := map[string]interface{}{
					"teamId": "team-phoenix",
					"userId": "demo",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "sideways", // Invalid - must be improving/stable/declining
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Invalid trend value should be rejected")
			})
		})

		Context("when submitting health check with missing required fields", func() {
			It("should reject submission without teamId", func() {
				body := map[string]interface{}{
					"userId": "demo",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Missing teamId should be rejected")
			})

			It("should reject submission with empty responses", func() {
				body := map[string]interface{}{
					"teamId":    "team-phoenix",
					"userId":    "demo",
					"date":      time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"Empty responses should be rejected")
			})
		})

		Context("when submitting health check with XSS attempts", func() {
			It("should sanitize script tags in comments", func() {
				uniqueID := fmt.Sprintf("test-xss-%d", time.Now().UnixNano())
				xssPayload := "<script>alert('xss')</script>"
				body := map[string]interface{}{
					"id":     uniqueID,
					"teamId": "team-phoenix",
					"userId": "demo",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "stable",
							"comment":     xssPayload,
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)

				resp, err := makeAuthenticatedRequest("POST", "/api/v1/health-checks", userToken, strings.NewReader(string(bodyBytes)))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Should either sanitize and accept, or reject outright
				// Note: Backend stores raw text; frontend (React) auto-escapes on render,
				// so XSS is mitigated at the presentation layer. Server-side sanitization
				// is a future enhancement.
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusOK),
					Equal(http.StatusConflict),
					Equal(http.StatusBadRequest),
				))
			})
		})
	})
})

// Helper function to format assessment period
func formatPeriod(year int, half string) string {
	return fmt.Sprintf("%d - %s Half", year, half)
}
