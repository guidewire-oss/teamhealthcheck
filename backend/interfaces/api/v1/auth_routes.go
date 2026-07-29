// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/gin-gonic/gin"
)

// SetupAuthRoutes configures authentication routes with repository dependency injection
func SetupAuthRoutes(router *gin.Engine, userRepo user.Repository, orgRepo organization.Repository, jwtService *services.JWTService) {
	authHandler := NewAuthHandler(userRepo, orgRepo, jwtService)

	// Authentication routes (public - no JWT required)
	auth := router.Group("/api/v1/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)
	}
}
