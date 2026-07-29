// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
)

var _ = Describe("Health Check API", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		err        error
		jwtService *services.JWTService
		userToken  string
	)

	BeforeEach(func() {
		// Set Gin to test mode
		gin.SetMode(gin.TestMode)

		// Connect to test database
		databaseURL := os.Getenv("TEST_DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
		}

		db, err = sql.Open("postgres", databaseURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		// Clean and run migrations
		_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
		Expect(err).NotTo(HaveOccurred())

		driver, err := migratePostgres.WithInstance(db, &migratePostgres.Config{})
		Expect(err).NotTo(HaveOccurred())

		migrationEngine, err := migrate.NewWithDatabaseInstance(
			"file://../../../infrastructure/persistence/postgres/migrations",
			"postgres",
			driver,
		)
		Expect(err).NotTo(HaveOccurred())

		err = migrationEngine.Up()
		Expect(err).NotTo(HaveOccurred())

		// Initialize JWT service and generate test token
		jwtService = services.NewJWTService()
		tokenPair, tokenErr := jwtService.GenerateTokenPair(context.Background(), "user123", "user123", "user123@test.com", "level-5", []string{"team1"})
		Expect(tokenErr).NotTo(HaveOccurred())
		userToken = tokenPair.AccessToken

		// Initialize router with API routes
		router = gin.New()
		healthCheckRepo := postgres.NewHealthCheckRepository(db)
		orgRepo := postgres.NewOrganizationRepository(db)
		v1.SetupHealthCheckRoutes(router, healthCheckRepo, orgRepo, jwtService, nil)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v1/health-checks", func() {
		Context("when submitting a valid health check", func() {
			It("should create a new session and return 201 Created", func() {
				// Given: A valid health check submission
				submission := map[string]interface{}{
					"teamId":           "team1",
					"userId":           "user123",
					"date":             time.Now().Format(time.RFC3339),
					"assessmentPeriod": "2024 - 2nd Half",
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "improving",
							"comment":     "Clear mission and goals",
						},
						{
							"dimensionId": "value",
							"score":       2,
							"trend":       "stable",
							"comment":     "Delivering value consistently",
						},
						{
							"dimensionId": "speed",
							"score":       1,
							"trend":       "declining",
							"comment":     "Too many blockers",
						},
					},
					"completed": true,
				}

				body, err := json.Marshal(submission)
				Expect(err).NotTo(HaveOccurred())

				// When: Submitting via POST
				req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 201 Created
				Expect(w.Code).To(Equal(http.StatusCreated))

				// And: Response should contain session ID
				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response).To(HaveKey("id"))
				Expect(response["teamId"]).To(Equal("team1"))
				Expect(response["userId"]).To(Equal("user123"))
				Expect(response["completed"]).To(BeTrue())

				// And: Session should be persisted in database
				sessionId := response["id"].(string)
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM health_check_sessions WHERE id = $1", sessionId).Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(1))

				// And: Responses should be persisted
				var responseCount int
				err = db.QueryRow("SELECT COUNT(*) FROM health_check_responses WHERE session_id = $1", sessionId).Scan(&responseCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(responseCount).To(Equal(3))
			})

			It("should auto-generate session ID if not provided", func() {
				// Given: Submission without ID
				submission := map[string]interface{}{
					"teamId": "team1",
					"userId": "user123",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{
							"dimensionId": "mission",
							"score":       3,
							"trend":       "improving",
						},
					},
					"completed": true,
				}

				body, _ := json.Marshal(submission)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()

				// When: Posting
				router.ServeHTTP(w, req)

				// Then: Should generate ID
				Expect(w.Code).To(Equal(http.StatusCreated))
				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)
				Expect(response["id"]).NotTo(BeEmpty())
			})
		})

		Context("when submitting invalid data", func() {
			It("should return 400 Bad Request for missing teamId", func() {
				// Given: Submission without teamId
				submission := map[string]interface{}{
					"userId": "user123",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{"dimensionId": "mission", "score": 3, "trend": "improving"},
					},
				}

				body, _ := json.Marshal(submission)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()

				// When: Posting
				router.ServeHTTP(w, req)

				// Then: Should return 400
				Expect(w.Code).To(Equal(http.StatusBadRequest))
				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)
				Expect(response).To(HaveKey("error"))
			})

			It("should return 400 Bad Request for invalid score", func() {
				// Given: Submission with score out of range
				submission := map[string]interface{}{
					"teamId": "team1",
					"userId": "user123",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{"dimensionId": "mission", "score": 5, "trend": "improving"}, // Invalid score
					},
				}

				body, _ := json.Marshal(submission)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()

				// When: Posting
				router.ServeHTTP(w, req)

				// Then: Should return 400
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})

			It("should return 400 Bad Request for invalid trend", func() {
				// Given: Submission with invalid trend
				submission := map[string]interface{}{
					"teamId": "team1",
					"userId": "user123",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{"dimensionId": "mission", "score": 3, "trend": "invalid-trend"},
					},
				}

				body, _ := json.Marshal(submission)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()

				// When: Posting
				router.ServeHTTP(w, req)

				// Then: Should return 400
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})

			It("should return 400 Bad Request for empty responses", func() {
				// Given: Submission with no responses
				submission := map[string]interface{}{
					"teamId":    "team1",
					"userId":    "user123",
					"date":      time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{},
				}

				body, _ := json.Marshal(submission)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()

				// When: Posting
				router.ServeHTTP(w, req)

				// Then: Should return 400
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Describe("GET /api/v1/health-dimensions", func() {
		Context("when fetching health dimensions", func() {
			It("should return all 11 active dimensions", func() {
				// When: Getting dimensions
				req := httptest.NewRequest(http.MethodGet, "/api/v1/health-dimensions", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Should have 11 dimensions
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response).To(HaveKey("dimensions"))

				dimensions := response["dimensions"].([]interface{})
				Expect(dimensions).To(HaveLen(11))

				// And: Each dimension should have required fields
				firstDimension := dimensions[0].(map[string]interface{})
				Expect(firstDimension).To(HaveKey("id"))
				Expect(firstDimension).To(HaveKey("name"))
				Expect(firstDimension).To(HaveKey("description"))
				Expect(firstDimension).To(HaveKey("goodDescription"))
				Expect(firstDimension).To(HaveKey("badDescription"))
			})

			It("should include mission dimension", func() {
				// When: Getting dimensions
				req := httptest.NewRequest(http.MethodGet, "/api/v1/health-dimensions", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should include mission
				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)
				dimensions := response["dimensions"].([]interface{})

				found := false
				for _, dim := range dimensions {
					dimension := dim.(map[string]interface{})
					if dimension["id"] == "mission" {
						found = true
						Expect(dimension["name"]).To(Equal("Mission"))
						break
					}
				}
				Expect(found).To(BeTrue(), "Mission dimension should be present")
			})
		})
	})

	Describe("GET /api/v1/health-checks/:id", func() {
		Context("when fetching an existing session", func() {
			It("should return the session with all responses", func() {
				// Given: An existing session
				submission := map[string]interface{}{
					"teamId": "team1",
					"userId": "user123",
					"date":   time.Now().Format(time.RFC3339),
					"responses": []map[string]interface{}{
						{"dimensionId": "mission", "score": 3, "trend": "improving", "comment": "Great"},
					},
					"completed": true,
				}

				body, _ := json.Marshal(submission)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				var createResponse map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &createResponse)
				sessionId := createResponse["id"].(string)

				// When: Getting by ID
				req = httptest.NewRequest(http.MethodGet, "/api/v1/health-checks/"+sessionId, nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w = httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Should have session details
				var getResponse map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &getResponse)
				Expect(getResponse["id"]).To(Equal(sessionId))
				Expect(getResponse["teamId"]).To(Equal("team1"))

				// And: Should have responses
				responses := getResponse["responses"].([]interface{})
				Expect(responses).To(HaveLen(1))
			})
		})

		Context("when session doesn't exist", func() {
			It("should return 404 Not Found", func() {
				// When: Getting non-existent session
				req := httptest.NewRequest(http.MethodGet, "/api/v1/health-checks/non-existent-id", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 404
				Expect(w.Code).To(Equal(http.StatusNotFound))
			})
		})
	})

	Describe("GET /api/v1/health-checks/team/:id", func() {
		Context("when team has multiple sessions", func() {
			It("should return all sessions for the team", func() {
				// Given: Multiple sessions for team1
				for i := 0; i < 3; i++ {
					submission := map[string]interface{}{
						"teamId": "team1",
						"userId": "user" + string(rune(i+1)),
						"date":   time.Now().Format(time.RFC3339),
						"responses": []map[string]interface{}{
							{"dimensionId": "mission", "score": 3, "trend": "improving"},
						},
						"completed": true,
					}

					body, _ := json.Marshal(submission)
					req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer "+userToken)
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
				}

				// When: Getting team sessions
				req := httptest.NewRequest(http.MethodGet, "/api/v1/health-checks/team/team1", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Should have 3 sessions
				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)
				sessions := response["sessions"].([]interface{})
				Expect(sessions).To(HaveLen(3))
			})
		})

		Context("when filtering by assessment period", func() {
			It("should return only sessions from that period", func() {
				// Given: Sessions from different periods
				submission1 := map[string]interface{}{
					"teamId":           "team1",
					"userId":           "user1",
					"date":             "2024-01-15T10:00:00Z",
					"assessmentPeriod": "2023 - 2nd Half",
					"responses": []map[string]interface{}{
						{"dimensionId": "mission", "score": 3, "trend": "improving"},
					},
					"completed": true,
				}

				submission2 := map[string]interface{}{
					"teamId":           "team1",
					"userId":           "user2",
					"date":             "2024-07-15T10:00:00Z",
					"assessmentPeriod": "2024 - 1st Half",
					"responses": []map[string]interface{}{
						{"dimensionId": "value", "score": 2, "trend": "stable"},
					},
					"completed": true,
				}

				// Post both submissions
				for _, sub := range []map[string]interface{}{submission1, submission2} {
					body, _ := json.Marshal(sub)
					req := httptest.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewBuffer(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer "+userToken)
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
				}

				// When: Filtering by period
				req := httptest.NewRequest(http.MethodGet, "/api/v1/health-checks/team/team1?assessmentPeriod=2024+-+1st+Half", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return only matching sessions
				Expect(w.Code).To(Equal(http.StatusOK))
				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)
				sessions := response["sessions"].([]interface{})
				Expect(sessions).To(HaveLen(1))

				session := sessions[0].(map[string]interface{})
				Expect(session["assessmentPeriod"]).To(Equal("2024 - 1st Half"))
			})
		})
	})
})
