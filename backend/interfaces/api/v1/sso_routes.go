// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"os"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/gin-gonic/gin"
)

// SetupSSORoutes registers the SSO/OAuth endpoints.
func SetupSSORoutes(router *gin.Engine, userRepo user.Repository, jwtService *services.JWTService, orgRepo organization.Repository) {
	h := NewSSOHandler(userRepo, jwtService)

	sso := router.Group("/api/v1/auth/sso")
	{
		sso.POST("/callback", h.Callback)
	}

	// Public config endpoint — returns SSO settings and branding
	// so the frontend can display the "Sign in with SSO" button and company branding
	// without baking values at build time.
	router.GET("/api/v1/config", func(c *gin.Context) {
		appEnv := getEnvOrDefault("APP_ENV", "production")

		// Fetch branding from database
		companyName := "My Company"
		var logoURL *string
		if appSettings, err := orgRepo.GetAppSettings(c.Request.Context()); err == nil {
			companyName = appSettings.CompanyName
			if appSettings.LogoURL != "" {
				logoURL = &appSettings.LogoURL
			}
		}

		clientID := os.Getenv("OAUTH_CLIENT_ID")
		if clientID == "" {
			c.JSON(200, gin.H{"sso": nil, "appEnv": appEnv, "companyName": companyName, "logoURL": logoURL})
			return
		}
		c.JSON(200, gin.H{
			"appEnv":      appEnv,
			"companyName": companyName,
			"logoURL":     logoURL,
			"sso": gin.H{
				"clientId":     clientID,
				"authorizeUrl": os.Getenv("OAUTH_AUTHORIZE_URL"),
				"redirectUri":  os.Getenv("OAUTH_REDIRECT_URI"),
				"scopes":       getEnvOrDefault("OAUTH_SCOPES", "openid email profile"),
			},
		})
	})
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
