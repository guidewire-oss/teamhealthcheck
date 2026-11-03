package v1_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

var _ = Describe("Admin API", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		adminToken string
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)

		databaseURL := "postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
		var err error
		db, err = sql.Open("postgres", databaseURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		// Clean schema and run migrations fresh
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

		// Clean up test data
		db.Exec("DELETE FROM team_members WHERE user_id LIKE 'test-%' OR team_id LIKE 'test-%'")
		db.Exec("DELETE FROM team_supervisors WHERE user_id LIKE 'test-%' OR team_id LIKE 'test-%'")
		db.Exec("DELETE FROM health_check_responses")
		db.Exec("DELETE FROM health_check_sessions WHERE user_id LIKE 'test-%'")
		db.Exec("DELETE FROM teams WHERE id LIKE 'test-%'")
		db.Exec("DELETE FROM users WHERE id LIKE 'test-%'")
		db.Exec("DELETE FROM hierarchy_levels WHERE id LIKE 'test-%'")
		db.Exec("DELETE FROM health_dimensions WHERE id LIKE 'test-%' OR id LIKE 'e2e-%' OR id LIKE 'dim-%'")

		// Create JWT service and generate admin token
		jwtService := services.NewJWTService()
		tokenPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = tokenPair.AccessToken

		// Create repositories and router
		orgRepo := postgres.NewOrganizationRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)
		healthCheckRepo := postgres.NewHealthCheckRepository(db)

		router = gin.New()
		v1.SetupAdminRoutes(router, orgRepo, userRepo, teamRepo, healthCheckRepo, jwtService)
	})

	AfterEach(func() {
		if db != nil {
			db.Exec("DELETE FROM team_members WHERE team_id LIKE 'test-%'")
			db.Exec("DELETE FROM team_supervisors WHERE team_id LIKE 'test-%'")
			db.Exec("DELETE FROM teams WHERE id LIKE 'test-%'")
			db.Exec("DELETE FROM hierarchy_levels WHERE id LIKE 'test-%'")
			db.Exec("DELETE FROM users WHERE id LIKE 'test-%'")
			db.Close()
		}
	})

	Describe("GET /api/v1/admin/hierarchy-levels", func() {
		It("should return all hierarchy levels ordered by position", func() {
			req := httptest.NewRequest("GET", "/api/v1/admin/hierarchy-levels", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.HierarchyLevelsResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(response.Levels)).To(BeNumerically(">=", 5))

			// Verify order
			for i := 1; i < len(response.Levels); i++ {
				Expect(response.Levels[i].Position).To(BeNumerically(">=", response.Levels[i-1].Position))
			}
		})
	})

	Describe("POST /api/v1/admin/hierarchy-levels", func() {
		It("should create a new hierarchy level", func() {
			reqBody := dto.CreateHierarchyLevelRequest{
				ID:   "test-level-1",
				Name: "Test Level",
				Permissions: dto.HierarchyPermissionsDTO{
					CanViewAllTeams:  true,
					CanEditTeams:     false,
					CanManageUsers:   false,
					CanTakeSurvey:    true,
					CanViewAnalytics: false,
				},
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))

			var response dto.HierarchyLevelDTO
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.ID).To(Equal("test-level-1"))
			Expect(response.Name).To(Equal("Test Level"))
		})

		It("should create a level successfully with no color field, and ignore any color sent by the client", func() {
			// Note: the level's own id/name intentionally avoid the word
			// "color" so the response-body assertion below can't produce a
			// false positive from the ID itself.
			reqBody := map[string]interface{}{
				"id":   "test-level-no-swatch",
				"name": "Test Level No Swatch",
				// A client attempting to send a "color" field must be
				// silently ignored: the feature no longer exists.
				"color": "#FF0000",
				"permissions": map[string]bool{
					"canViewAllTeams":  true,
					"canEditTeams":     false,
					"canManageUsers":   false,
					"canTakeSurvey":    true,
					"canViewAnalytics": false,
				},
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))
			Expect(w.Body.String()).NotTo(ContainSubstring("color"))

			var raw map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &raw)).To(Succeed())
			Expect(raw).NotTo(HaveKey("color"))
		})

		It("should assign position on the server and ignore any position sent by the client", func() {
			before := httptest.NewRequest("GET", "/api/v1/admin/hierarchy-levels", nil)
			before.Header.Set("Authorization", "Bearer "+adminToken)
			beforeW := httptest.NewRecorder()
			router.ServeHTTP(beforeW, before)
			var beforeList dto.HierarchyLevelsResponse
			Expect(json.Unmarshal(beforeW.Body.Bytes(), &beforeList)).To(Succeed())
			maxPositionBefore := 0
			for _, lvl := range beforeList.Levels {
				if lvl.Position > maxPositionBefore {
					maxPositionBefore = lvl.Position
				}
			}

			// A client attempting to smuggle a "position" field must be
			// ignored: the server always appends after the current max.
			reqBody := map[string]interface{}{
				"id":       "test-level-position",
				"name":     "Test Level Position",
				"position": 999,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))
			var response dto.HierarchyLevelDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Position).To(Equal(maxPositionBefore + 1))
		})

		It("should generate a valid, non-empty, ASCII id for a Unicode display name", func() {
			reqBody := dto.CreateHierarchyLevelRequest{
				Name: "Café Manager",
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))
			var response dto.HierarchyLevelDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.ID).NotTo(BeEmpty())
			Expect(response.ID).To(MatchRegexp(`^[a-z0-9-]+$`))
			Expect(response.Name).To(Equal("Café Manager"))

			db.Exec("DELETE FROM hierarchy_levels WHERE id = $1", response.ID)
		})

		It("should fall back to a generic id for a name with no ASCII letters or digits", func() {
			reqBody := dto.CreateHierarchyLevelRequest{
				Name: "経理部",
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))
			var response dto.HierarchyLevelDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.ID).NotTo(BeEmpty())
			Expect(response.ID).To(MatchRegexp(`^[a-z0-9-]+$`))
			Expect(response.Name).To(Equal("経理部"))

			db.Exec("DELETE FROM hierarchy_levels WHERE id = $1", response.ID)
		})

		It("should never collide or overwrite for names that differ only in punctuation", func() {
			createLevel := func(name string) dto.HierarchyLevelDTO {
				reqBody := dto.CreateHierarchyLevelRequest{Name: name}
				body, _ := json.Marshal(reqBody)
				req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(http.StatusCreated))
				var response dto.HierarchyLevelDTO
				Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
				return response
			}

			first := createLevel("Team Lead!")
			second := createLevel("Team Lead?")

			Expect(first.ID).NotTo(Equal(second.ID))
			Expect(first.Name).To(Equal("Team Lead!"))
			Expect(second.Name).To(Equal("Team Lead?"))

			db.Exec("DELETE FROM hierarchy_levels WHERE id = $1", first.ID)
			db.Exec("DELETE FROM hierarchy_levels WHERE id = $1", second.ID)
		})

		It("should reject a duplicate name with a clear 409 conflict instead of a generic 500", func() {
			first := dto.CreateHierarchyLevelRequest{ID: "test-dup-1", Name: "Duplicate Level"}
			body, _ := json.Marshal(first)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusCreated))

			second := dto.CreateHierarchyLevelRequest{ID: "test-dup-2", Name: "Duplicate Level"}
			body2, _ := json.Marshal(second)
			req2 := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body2))
			req2.Header.Set("Content-Type", "application/json")
			req2.Header.Set("Authorization", "Bearer "+adminToken)
			w2 := httptest.NewRecorder()
			router.ServeHTTP(w2, req2)

			Expect(w2.Code).To(Equal(http.StatusConflict))
			var errResp dto.ErrorResponse
			Expect(json.Unmarshal(w2.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp.Message).To(ContainSubstring("Duplicate Level"))
		})
	})

	Describe("PUT /api/v1/admin/hierarchy-levels/:id", func() {
		It("should preserve the level id when renaming", func() {
			createReq := dto.CreateHierarchyLevelRequest{ID: "test-rename-1", Name: "Old Name"}
			body, _ := json.Marshal(createReq)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusCreated))

			updateReq := dto.UpdateHierarchyLevelRequest{Name: "New Name"}
			updateBody, _ := json.Marshal(updateReq)
			putReq := httptest.NewRequest("PUT", "/api/v1/admin/hierarchy-levels/test-rename-1", bytes.NewBuffer(updateBody))
			putReq.Header.Set("Content-Type", "application/json")
			putReq.Header.Set("Authorization", "Bearer "+adminToken)
			putW := httptest.NewRecorder()
			router.ServeHTTP(putW, putReq)

			Expect(putW.Code).To(Equal(http.StatusOK))
			var response dto.HierarchyLevelDTO
			Expect(json.Unmarshal(putW.Body.Bytes(), &response)).To(Succeed())
			Expect(response.ID).To(Equal("test-rename-1"))
			Expect(response.Name).To(Equal("New Name"))
		})

		It("should update successfully and ignore any color sent by the client", func() {
			createReq := dto.CreateHierarchyLevelRequest{ID: "test-color-update", Name: "Color Update Level"}
			body, _ := json.Marshal(createReq)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusCreated))

			updateReq := map[string]interface{}{
				"name":  "Color Update Level Renamed",
				"color": "#123ABC",
			}
			updateBody, _ := json.Marshal(updateReq)
			putReq := httptest.NewRequest("PUT", "/api/v1/admin/hierarchy-levels/test-color-update", bytes.NewBuffer(updateBody))
			putReq.Header.Set("Content-Type", "application/json")
			putReq.Header.Set("Authorization", "Bearer "+adminToken)
			putW := httptest.NewRecorder()
			router.ServeHTTP(putW, putReq)

			Expect(putW.Code).To(Equal(http.StatusOK))
			var raw map[string]interface{}
			Expect(json.Unmarshal(putW.Body.Bytes(), &raw)).To(Succeed())
			Expect(raw).NotTo(HaveKey("color"))
			Expect(raw["name"]).To(Equal("Color Update Level Renamed"))
		})
	})

	Describe("DELETE /api/v1/admin/hierarchy-levels/:id", func() {
		It("should compact remaining positions to contiguous 1..N after a delete", func() {
			createLevel := func(id string) {
				reqBody := dto.CreateHierarchyLevelRequest{ID: id, Name: "Compact " + id}
				body, _ := json.Marshal(reqBody)
				req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(http.StatusCreated))
			}
			createLevel("test-compact-1")
			createLevel("test-compact-2")
			createLevel("test-compact-3")

			delReq := httptest.NewRequest("DELETE", "/api/v1/admin/hierarchy-levels/test-compact-2", nil)
			delReq.Header.Set("Authorization", "Bearer "+adminToken)
			delW := httptest.NewRecorder()
			router.ServeHTTP(delW, delReq)
			Expect(delW.Code).To(Equal(http.StatusOK))

			listReq := httptest.NewRequest("GET", "/api/v1/admin/hierarchy-levels", nil)
			listReq.Header.Set("Authorization", "Bearer "+adminToken)
			listW := httptest.NewRecorder()
			router.ServeHTTP(listW, listReq)
			var list dto.HierarchyLevelsResponse
			Expect(json.Unmarshal(listW.Body.Bytes(), &list)).To(Succeed())

			// Positions must be exactly 1..N with no gaps after the delete.
			for i, lvl := range list.Levels {
				Expect(lvl.Position).To(Equal(i + 1))
			}
		})
	})

	Describe("GET /api/v1/admin/users", func() {
		doListUsersRequest := func(query string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("GET", "/api/v1/admin/users"+query, nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w
		}

		It("should apply default pagination values when no query params are given", func() {
			w := doListUsersRequest("")
			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.UsersResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Users).NotTo(BeEmpty())
			Expect(response.Pagination.Page).To(Equal(1))
			Expect(response.Pagination.PageSize).To(Equal(25))
			Expect(len(response.Users)).To(BeNumerically("<=", 25))
		})

		It("should validate and clamp invalid page/pageSize values", func() {
			// Negative/zero page falls back to page 1
			w := doListUsersRequest("?page=0")
			var response dto.UsersResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Pagination.Page).To(Equal(1))

			// Non-numeric page falls back to default
			w = doListUsersRequest("?page=abc")
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Pagination.Page).To(Equal(1))

			// pageSize above the max is clamped to 100
			w = doListUsersRequest("?pageSize=500")
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Pagination.PageSize).To(Equal(100))

			// Negative pageSize falls back to default
			w = doListUsersRequest("?pageSize=-5")
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Pagination.PageSize).To(Equal(25))
		})

		It("should paginate at the database level with deterministic ordering and correct metadata", func() {
			// Seed 3 known test users (alphabetically ordered usernames) isolated via search
			for i, uname := range []string{"pagealice", "pagebob", "pagecarl"} {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $2, $3, $4, 'level-5', 'x')
				`, "test-page-user-"+uname, uname, uname+"@test.com", "Page User "+string(rune('A'+i)))
				Expect(err).NotTo(HaveOccurred())
			}

			// Page 1 of 2 (pageSize=2) within the isolated search scope
			w := doListUsersRequest("?search=page&page=1&pageSize=2")
			var response dto.UsersResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Users).To(HaveLen(2))
			Expect(response.Users[0].Username).To(Equal("pagealice"))
			Expect(response.Users[1].Username).To(Equal("pagebob"))
			Expect(response.Pagination.TotalItems).To(Equal(3))
			Expect(response.Pagination.TotalPages).To(Equal(2))
			Expect(response.Pagination.HasNextPage).To(BeTrue())
			Expect(response.Pagination.HasPreviousPage).To(BeFalse())

			// Page 2 of 2
			w = doListUsersRequest("?search=page&page=2&pageSize=2")
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Users).To(HaveLen(1))
			Expect(response.Users[0].Username).To(Equal("pagecarl"))
			Expect(response.Pagination.HasNextPage).To(BeFalse())
			Expect(response.Pagination.HasPreviousPage).To(BeTrue())
		})

		It("should filter by search across username, full name, and email", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('test-search-user-1', 'uniqueusername', 'unique@test.com', 'Unique Name', 'level-5', 'x')
			`)
			Expect(err).NotTo(HaveOccurred())

			w := doListUsersRequest("?search=uniqueusername")
			var response dto.UsersResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Users).To(HaveLen(1))
			Expect(response.Users[0].Username).To(Equal("uniqueusername"))
			Expect(response.Pagination.TotalItems).To(Equal(1))
		})

		It("should filter by role (hierarchy level)", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('test-role-user-1', 'roleuser1', 'roleuser1@test.com', 'Role User', 'level-1', 'x')
			`)
			Expect(err).NotTo(HaveOccurred())

			w := doListUsersRequest("?search=roleuser1&role=level-1")
			var response dto.UsersResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Users).To(HaveLen(1))

			w = doListUsersRequest("?search=roleuser1&role=level-2")
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Users).To(BeEmpty())
		})
	})

	Describe("GET /api/v1/admin/users/lite", func() {
		It("should return minimal fields for every user without team data", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('test-lite-user-1', 'liteuser1', 'liteuser1@test.com', 'Lite User', 'level-5', 'x')
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/v1/admin/users/lite", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.UsersLiteResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
			Expect(response.Users).NotTo(BeEmpty())

			found := false
			for _, u := range response.Users {
				if u.Username == "liteuser1" {
					found = true
					Expect(u.FullName).To(Equal("Lite User"))
					Expect(u.HierarchyLevel).To(Equal("level-5"))
				}
			}
			Expect(found).To(BeTrue())
		})
	})

	Describe("POST /api/v1/admin/users", func() {
		It("should create a new user", func() {
			reqBody := dto.CreateUserRequest{
				ID:             "test-user-1",
				Username:       "testuser",
				Email:          "test@example.com",
				FullName:       "Test User",
				Password:       "password123",
				HierarchyLevel: "level-5",
				ReportsTo:      nil,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/users", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))

			var response dto.AdminUserDTO
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Username).To(Equal("testuser"))
		})
	})

	Describe("GET /api/v1/admin/teams", func() {
		It("should return all teams with cadence and total count", func() {
			// Seed a test team so the response is non-empty
			db.Exec("INSERT INTO teams (id, name) VALUES ('test-team-1', 'Test Team') ON CONFLICT DO NOTHING")

			req := httptest.NewRequest("GET", "/api/v1/admin/teams", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.TeamsResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Teams).NotTo(BeEmpty())
			Expect(response.Total).To(Equal(len(response.Teams)))
		})
	})

	Describe("GET /api/v1/admin/settings/dimensions", func() {
		It("should return all 11 health dimensions", func() {
			req := httptest.NewRequest("GET", "/api/v1/admin/settings/dimensions", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.DimensionsResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Dimensions).To(HaveLen(11))

			for _, dim := range response.Dimensions {
				Expect(dim.ID).NotTo(BeEmpty())
				Expect(dim.Name).NotTo(BeEmpty())
				Expect(dim.Weight).To(BeNumerically(">", 0))
			}
		})
	})

	Describe("PUT /api/v1/admin/settings/dimensions/:id", func() {
		It("should update dimension weight and active status", func() {
			isActive := false
			weight := 2.5
			reqBody := dto.UpdateDimensionRequest{
				IsActive: &isActive,
				Weight:   &weight,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("PUT", "/api/v1/admin/settings/dimensions/mission", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.HealthDimensionDTO
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.IsActive).To(BeFalse())
			Expect(response.Weight).To(Equal(2.5))

			// Restore original values
			isActive = true
			weight = 1.0
			reqBody = dto.UpdateDimensionRequest{IsActive: &isActive, Weight: &weight}
			body, _ = json.Marshal(reqBody)
			req = httptest.NewRequest("PUT", "/api/v1/admin/settings/dimensions/mission", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
		})
	})
})
