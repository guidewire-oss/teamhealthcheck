
# Team Lead Dashboard API Documentation

## Overview
The Team Lead Dashboard API provides endpoints for team leads to view aggregated health check data, individual responses, distributions, and trends for their teams.

## Base Path
All endpoints are prefixed with `/api/v1/teams/:teamId/dashboard`

## Authentication
All endpoints require a valid JWT access token (`JWTAuthMiddleware`) **and** team membership or supervision (`TeamMembershipMiddleware`, keyed on the `:teamId` path param) — see `SetupTeamDashboardRoutes` in `backend/interfaces/api/v1/team_dashboard_routes.go`. A request without a valid token, or from a user who isn't on the team's roster or supervisor chain, is rejected before reaching the handler.

## Endpoints

### 1. Health Summary (Radar Chart Data)

**Endpoint:** `GET /api/v1/teams/:teamId/dashboard/health-summary`

**Description:** Returns aggregated health data showing average scores per dimension. Used for radar chart visualization.

**Query Parameters:**
- `assessmentPeriod` (optional): Filter by specific assessment period (e.g., "2024 - 1st Half")

**Response:**
```json
{
  "teamId": "team-alpha",
  "teamName": "Alpha Squad",
  "assessmentPeriod": "2024 - 1st Half",
  "dimensions": [
    {
      "dimensionId": "mission",
      "avgScore": 2.5,
      "responseCount": 4
    },
    {
      "dimensionId": "value",
      "avgScore": 2.0,
      "responseCount": 4
    }
  ],
  "overallHealth": 2.3,
  "submissionCount": 4
}
```

**Example:**
```bash
curl http://localhost:8080/api/v1/teams/team-alpha/dashboard/health-summary?assessmentPeriod=2024%20-%201st%20Half
```

---

### 2. Response Distribution (Bar Chart Data)

**Endpoint:** `GET /api/v1/teams/:teamId/dashboard/response-distribution`

**Description:** Returns count of red/yellow/green scores per dimension. Used for bar chart visualization showing response distribution.

**Query Parameters:**
- `assessmentPeriod` (optional): Filter by specific assessment period

**Response:**
```json
{
  "teamId": "team-alpha",
  "distribution": [
    {
      "dimensionId": "mission",
      "red": 1,
      "yellow": 2,
      "green": 3
    },
    {
      "dimensionId": "value",
      "red": 2,
      "yellow": 1,
      "green": 1
    }
  ]
}
```

**Score Mapping:**
- Red: score = 1 (Poor)
- Yellow: score = 2 (Medium)
- Green: score = 3 (Good)

**Example:**
```bash
curl http://localhost:8080/api/v1/teams/team-alpha/dashboard/response-distribution
```

---

### 3. Individual Responses

**Endpoint:** `GET /api/v1/teams/:teamId/dashboard/individual-responses`

**Description:** Returns individual team member responses with their scores, trends, and comments for each dimension.

**Query Parameters:**
- `assessmentPeriod` (optional): Filter by specific assessment period

**Response:**
```json
{
  "teamId": "team-alpha",
  "responses": [
    {
      "sessionId": "session-123",
      "userId": "alice",
      "userName": "Alice Johnson",
      "date": "2024-06-15",
      "dimensions": [
        {
          "dimensionId": "mission",
          "score": 3,
          "trend": "improving",
          "comment": "We have great clarity on our mission"
        },
        {
          "dimensionId": "value",
          "score": 2,
          "trend": "stable",
          "comment": ""
        }
      ]
    }
  ]
}
```

**Example:**
```bash
curl http://localhost:8080/api/v1/teams/team-alpha/dashboard/individual-responses?assessmentPeriod=2024%20-%201st%20Half
```

---

### 4. Trends (Multi-Period Line Chart Data)

**Endpoint:** `GET /api/v1/teams/:teamId/dashboard/trends`

**Description:** Returns trend data showing average scores per dimension across multiple assessment periods. Used for line chart visualization.

**Query Parameters:**
None (automatically includes all periods)

**Response:**
```json
{
  "teamId": "team-alpha",
  "periods": ["2023 - 2nd Half", "2024 - 1st Half"],
  "dimensions": [
    {
      "dimensionId": "mission",
      "scores": [1.5, 2.5]
    },
    {
      "dimensionId": "value",
      "scores": [1.8, 2.0]
    }
  ]
}
```

**Notes:**
- The `scores` array matches the order of the `periods` array
- If a dimension has no data for a period, the score is `0`
- Only includes periods where the team has completed health check sessions

**Example:**
```bash
curl http://localhost:8080/api/v1/teams/team-alpha/dashboard/trends
```

---

## Error Responses

All endpoints return standard error responses:

```json
{
  "error": "Error type",
  "message": "Detailed error message"
}
```

**Common HTTP Status Codes:**
- `200 OK`: Successful request
- `400 Bad Request`: Missing or invalid team ID
- `404 Not Found`: Team not found
- `500 Internal Server Error`: Database or server error

---

## Implementation Details

### Database Optimization
- Uses Common Table Expressions (CTEs) to avoid N+1 query problems
- Aggregates data at the database level for performance
- Single query per endpoint (except trends which uses 2 queries)

### Query Patterns
- All queries filter by `completed = true` to only include finished sessions
- Optional `assessmentPeriod` filtering supported on all endpoints except trends
- Uses PostgreSQL JSON aggregation (`json_agg`, `json_build_object`) for efficient data structuring

### Architecture
- **Handler:** `/backend/interfaces/api/v1/team_dashboard_handler.go`
- **Routes:** `/backend/interfaces/api/v1/team_dashboard_routes.go`
- **DTOs:** `/backend/interfaces/dto/team_dashboard_dto.go`
- **Main:** Routes registered in `/backend/cmd/api/main.go`

---

## Frontend Integration

These endpoints are designed to support the Team Lead Dashboard at `/dashboard` in the Next.js frontend:

1. **Health Summary** → Radar chart showing team health across dimensions
2. **Response Distribution** → Bar chart showing red/yellow/green distribution
3. **Individual Responses** → Table showing individual team member feedback
4. **Trends** → Line chart showing health trends over time

The frontend should call these endpoints when a team lead views their dashboard, passing the appropriate `teamId` from the logged-in user's team assignment.
