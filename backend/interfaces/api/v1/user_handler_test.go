// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetUserSurveyHistory_Unit(t *testing.T) {
	// This is a unit test to verify the DTO structure and JSON serialization
	// For full integration testing with database, see tests/acceptance/

	gin.SetMode(gin.TestMode)

	t.Run("response structure matches DTO", func(t *testing.T) {
		// Verify DTO can be marshaled/unmarshaled correctly
		entry := dto.SurveyHistoryEntry{
			SessionID:        "session-1",
			TeamID:           "team-1",
			TeamName:         "Team Alpha",
			Date:             "2024-01-15",
			AssessmentPeriod: "2023 - 2nd Half",
			AvgScore:         2.5,
			ResponseCount:    11,
			Completed:        true,
		}

		response := dto.SurveyHistoryResponse{
			UserID:        "user1",
			SurveyHistory: []dto.SurveyHistoryEntry{entry},
			TotalSessions: 1,
		}

		jsonData, err := json.Marshal(response)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "sessionId")
		assert.Contains(t, string(jsonData), "teamName")
		assert.Contains(t, string(jsonData), "avgScore")
		assert.Contains(t, string(jsonData), "userId")
		assert.Contains(t, string(jsonData), "totalSessions")

		// Verify unmarshal works
		var decoded dto.SurveyHistoryResponse
		err = json.Unmarshal(jsonData, &decoded)
		assert.NoError(t, err)
		assert.Equal(t, "user1", decoded.UserID)
		assert.Equal(t, 1, decoded.TotalSessions)
		assert.Equal(t, 1, len(decoded.SurveyHistory))
		assert.Equal(t, "Team Alpha", decoded.SurveyHistory[0].TeamName)
		assert.Equal(t, 2.5, decoded.SurveyHistory[0].AvgScore)
		assert.Equal(t, 11, decoded.SurveyHistory[0].ResponseCount)
		assert.Equal(t, true, decoded.SurveyHistory[0].Completed)
	})

	t.Run("handler constructor creates valid handler", func(t *testing.T) {
		// Verify handler can be created (db can be nil for this test)
		handler := NewUserHandler(nil)
		assert.NotNil(t, handler)
	})
}
