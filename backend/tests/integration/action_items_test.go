package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

var _ = Describe("Integration: Action Items API", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		cleanup    func()
		jwtService *services.JWTService
	)

	tokenFor := func(userID, hierarchyLevel string) string {
		pair, err := jwtService.GenerateTokenPair(context.Background(), userID, userID, userID+"@test.com", hierarchyLevel, []string{"int_ai_team1"})
		Expect(err).NotTo(HaveOccurred())
		return pair.AccessToken
	}

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		db, cleanup = testhelpers.SetupTestDatabase()
		jwtService = services.NewJWTService()

		router = gin.New()
		v1.SetupActionItemRoutes(router, db, jwtService)

		// Hierarchy: int_ai_vp -> int_ai_mgr -> int_ai_lead -> int_ai_member
		_, err := db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to)
			VALUES
				('int_ai_vp', 'int_ai_vp', 'int_ai_vp@test.com', 'VP One', 'level-1', NULL),
				('int_ai_mgr', 'int_ai_mgr', 'int_ai_mgr@test.com', 'Manager One', 'level-3', 'int_ai_vp'),
				('int_ai_lead', 'int_ai_lead', 'int_ai_lead@test.com', 'Lead One', 'level-4', 'int_ai_mgr'),
				('int_ai_member', 'int_ai_member', 'int_ai_member@test.com', 'Member One', 'level-5', 'int_ai_lead'),
				('int_ai_outsider', 'int_ai_outsider', 'int_ai_outsider@test.com', 'Outsider', 'level-3', 'int_ai_vp')
		`)
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(`
			INSERT INTO teams (id, name, team_lead_id)
			VALUES ('int_ai_team1', 'Action Squad', 'int_ai_lead')
		`)
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(`
			INSERT INTO team_members (team_id, user_id)
			VALUES ('int_ai_team1', 'int_ai_lead'), ('int_ai_team1', 'int_ai_member')
		`)
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(`
			INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
			VALUES
				('int_ai_team1', 'int_ai_lead', 'level-4', 1),
				('int_ai_team1', 'int_ai_mgr', 'level-3', 2),
				('int_ai_team1', 'int_ai_vp', 'level-1', 3)
		`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	Describe("Direct-manager assignment", func() {
		It("allows assigning to the team lead's direct manager", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			body, _ := json.Marshal(map[string]interface{}{
				"title":      "Sync with manager",
				"assignedTo": "int_ai_mgr",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))
		})

		It("rejects an assignee outside the pod and outside the direct-manager exception", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			body, _ := json.Marshal(map[string]interface{}{
				"title":      "Invalid assignment",
				"assignedTo": "int_ai_outsider",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("resolves the direct manager via GET .../direct-manager", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/int_ai_team1/action-items/direct-manager", nil)
			req.Header.Set("Authorization", "Bearer "+leadToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var resp map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["id"]).To(Equal("int_ai_mgr"))
			Expect(resp["name"]).To(Equal("Manager One"))
		})
	})

	Describe("Status transitions", func() {
		createItem := func(leadToken string) string {
			body, _ := json.Marshal(map[string]interface{}{"title": "Transition target"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusCreated))
			var resp map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			return resp["id"].(string)
		}

		patchStatus := func(leadToken, id, status string) int {
			body, _ := json.Marshal(map[string]interface{}{"status": status})
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/int_ai_team1/action-items/"+id, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w.Code
		}

		It("allows Open -> In Progress -> On Hold -> In Progress -> Done", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			id := createItem(leadToken)

			Expect(patchStatus(leadToken, id, "in_progress")).To(Equal(http.StatusOK))
			Expect(patchStatus(leadToken, id, "on_hold")).To(Equal(http.StatusOK))
			Expect(patchStatus(leadToken, id, "in_progress")).To(Equal(http.StatusOK))
			Expect(patchStatus(leadToken, id, "done")).To(Equal(http.StatusOK))
		})

		It("rejects Open -> On Hold directly", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			id := createItem(leadToken)
			Expect(patchStatus(leadToken, id, "on_hold")).To(Equal(http.StatusBadRequest))
		})

		It("rejects On Hold -> Done directly", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			id := createItem(leadToken)
			Expect(patchStatus(leadToken, id, "in_progress")).To(Equal(http.StatusOK))
			Expect(patchStatus(leadToken, id, "on_hold")).To(Equal(http.StatusOK))
			Expect(patchStatus(leadToken, id, "done")).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("Creator-only edit authorization", func() {
		It("forbids a non-creator from editing content fields", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			body, _ := json.Marshal(map[string]interface{}{"title": "Owned by lead"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			var created map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &created)).To(Succeed())
			id := created["id"].(string)

			// A different team lead-equivalent actor (manager, who also has
			// team-board access) tries to edit the title.
			mgrToken := tokenFor("int_ai_mgr", "level-3")
			editBody, _ := json.Marshal(map[string]interface{}{"title": "Hijacked title"})
			editReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/int_ai_team1/action-items/"+id, bytes.NewReader(editBody))
			editReq.Header.Set("Authorization", "Bearer "+mgrToken)
			editReq.Header.Set("Content-Type", "application/json")
			editW := httptest.NewRecorder()
			router.ServeHTTP(editW, editReq)

			Expect(editW.Code).To(Equal(http.StatusForbidden))
		})

		It("forbids editing a Done action item even for its creator", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			body, _ := json.Marshal(map[string]interface{}{"title": "Will be done"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			var created map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &created)).To(Succeed())
			id := created["id"].(string)

			for _, status := range []string{"in_progress", "done"} {
				body, _ := json.Marshal(map[string]interface{}{"status": status})
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/int_ai_team1/action-items/"+id, bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+leadToken)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(http.StatusOK))
			}

			editBody, _ := json.Marshal(map[string]interface{}{"title": "Too late"})
			editReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/int_ai_team1/action-items/"+id, bytes.NewReader(editBody))
			editReq.Header.Set("Authorization", "Bearer "+leadToken)
			editReq.Header.Set("Content-Type", "application/json")
			editW := httptest.NewRecorder()
			router.ServeHTTP(editW, editReq)

			Expect(editW.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("Manager pod drill-down", func() {
		It("lets an in-hierarchy manager view the full pod board", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			body, _ := json.Marshal(map[string]interface{}{"title": "Visible to manager"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusCreated))

			mgrToken := tokenFor("int_ai_mgr", "level-3")
			podReq := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_ai_mgr/teams/int_ai_team1/action-items", nil)
			podReq.Header.Set("Authorization", "Bearer "+mgrToken)
			podW := httptest.NewRecorder()
			router.ServeHTTP(podW, podReq)

			Expect(podW.Code).To(Equal(http.StatusOK))
			var resp map[string]interface{}
			Expect(json.Unmarshal(podW.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["actionItems"].([]interface{})).To(HaveLen(1))
		})

		It("forbids a manager from viewing a pod outside their hierarchy", func() {
			outsiderToken := tokenFor("int_ai_outsider", "level-3")
			podReq := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_ai_outsider/teams/int_ai_team1/action-items", nil)
			podReq.Header.Set("Authorization", "Bearer "+outsiderToken)
			podW := httptest.NewRecorder()
			router.ServeHTTP(podW, podReq)

			Expect(podW.Code).To(Equal(http.StatusForbidden))
		})

		It("forbids a manager from impersonating another manager's view", func() {
			outsiderToken := tokenFor("int_ai_outsider", "level-3")
			podReq := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_ai_mgr/teams/int_ai_team1/action-items", nil)
			podReq.Header.Set("Authorization", "Bearer "+outsiderToken)
			podW := httptest.NewRecorder()
			router.ServeHTTP(podW, podReq)

			Expect(podW.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("Date handling", func() {
		// 06/12/2026 is entered in the UI's dd/mm/yyyy field, so it means
		// 6 December 2026 and must round-trip through the API's canonical
		// YYYY-MM-DD contract as 2026-12-06. The API itself only ever
		// accepts/returns the ISO form; the dd/mm/yyyy parsing happens at
		// the frontend boundary (see lib/date-format.ts), so this test
		// posts the already-converted ISO value and checks what comes
		// back out.
		It("round-trips 2026-12-06 (6 December 2026, the ISO form of dd/mm/yyyy 06/12/2026) without corruption", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			body, _ := json.Marshal(map[string]interface{}{
				"title":   "Date regression",
				"dueDate": "2026-12-06",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusCreated))

			listReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/int_ai_team1/action-items", nil)
			listReq.Header.Set("Authorization", "Bearer "+leadToken)
			listW := httptest.NewRecorder()
			router.ServeHTTP(listW, listReq)

			var listResp map[string]interface{}
			Expect(json.Unmarshal(listW.Body.Bytes(), &listResp)).To(Succeed())
			items := listResp["actionItems"].([]interface{})
			Expect(items).To(HaveLen(1))
			item := items[0].(map[string]interface{})
			// Exact match (not just a prefix check) guards against a
			// regression back to the RFC3339Nano timestamp bug, where the
			// pq driver's time.Time for a DATE column was implicitly
			// formatted as "2026-12-06T00:00:00Z" instead of "2026-12-06".
			Expect(item["dueDate"]).To(Equal("2026-12-06"))
		})

		It("rejects an unparseable due date instead of saving garbage", func() {
			leadToken := tokenFor("int_ai_lead", "level-4")
			body, _ := json.Marshal(map[string]interface{}{
				"title":   "Bad date",
				"dueDate": "06/12/2026",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/int_ai_team1/action-items", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+leadToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})
	})
})
