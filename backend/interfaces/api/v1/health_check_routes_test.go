package v1

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
)

// The eligibility route is a static path under /health-checks, registered next to the
// /health-checks/:id wildcard. Gin 1.11 resolves a static segment ahead of a wildcard, so
// registering both must not panic and both must be served. This pins that behavior, so a
// future router change that breaks the pairing fails here rather than at application startup.
func TestSetupHealthCheckRoutes_StaticEligibilityRouteCoexistsWithTheIDWildcard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("registering the health check routes panicked: %v", recovered)
		}
	}()
	SetupHealthCheckRoutes(router, nil, nil, services.NewJWTService(), nil)

	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /api/v1/health-checks/eligibility",
		"GET /api/v1/health-checks/:id",
		"GET /api/v1/health-checks/team/:id",
	} {
		if !registered[want] {
			t.Errorf("route %q is not registered", want)
		}
	}
}
