// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"encoding/json"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E: Dynamic Assessment Periods", func() {
	var userToken string

	BeforeEach(func() {
		var err error
		userToken, err = loginAndGetToken("demo", "demo")
		Expect(err).NotTo(HaveOccurred(), "Failed to login as demo user")
		Expect(userToken).NotTo(BeEmpty())
	})

	Describe("GET /api/v1/assessment-periods", func() {
		Context("when assessment periods exist in the database", func() {
			It("should return distinct periods from health check sessions", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/assessment-periods", userToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result struct {
					Periods []string `json:"periods"`
				}
				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &result)
				Expect(err).NotTo(HaveOccurred())

				// Suite setup seeds sessions with "2023 - 2nd Half" and "2024 - 1st Half"
				Expect(result.Periods).NotTo(BeEmpty(), "Should return assessment periods from seeded data")
				Expect(result.Periods).To(ContainElement("2023 - 2nd Half"))
				Expect(result.Periods).To(ContainElement("2024 - 1st Half"))
			})

			It("should return periods in descending order", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/assessment-periods", userToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				var result struct {
					Periods []string `json:"periods"`
				}
				body, _ := io.ReadAll(resp.Body)
				json.Unmarshal(body, &result)

				Expect(len(result.Periods)).To(BeNumerically(">=", 2),
					"Should have at least 2 assessment periods")

				// Verify descending order: each period should be >= the next
				for i := 0; i < len(result.Periods)-1; i++ {
					Expect(result.Periods[i] >= result.Periods[i+1]).To(BeTrue(),
						"Periods should be in descending order")
				}
			})
		})

		Context("when requesting without authentication", func() {
			It("should return 401 Unauthorized", func() {
				resp, err := makeRequest("GET", "/api/v1/assessment-periods", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Assessment periods endpoint should require authentication")
			})
		})
	})
})
