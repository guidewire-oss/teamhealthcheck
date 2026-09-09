package v1

import (
	"net/http"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
)

// PasswordResetHandler handles password reset HTTP requests
type PasswordResetHandler struct {
	resetService *services.PasswordResetService
	userRepo     user.Repository
}

// NewPasswordResetHandler creates a new handler
func NewPasswordResetHandler(resetService *services.PasswordResetService, userRepo user.Repository) *PasswordResetHandler {
	return &PasswordResetHandler{
		resetService: resetService,
		userRepo:     userRepo,
	}
}

// ForgotPassword handles forgot password requests
//
// @Summary Request a password reset email
// @Description Validates the submitted email format and, if a matching account exists, creates a password reset token and emails a reset link to it. The response message is identical whether or not the email is registered, so the endpoint cannot be used to discover which addresses have accounts. This is a public endpoint that requires no authentication.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body dto.ForgotPasswordRequest true "Email address"
// @Success 200 {object} map[string]string "message"
// @Failure 400 {object} dto.ErrorResponse "Missing or invalid email"
// @Failure 500 {object} map[string]string "error"
// @Router /auth/forgot-password [post]
func (h *PasswordResetHandler) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondError(c, http.StatusBadRequest, "Email is required")
		return
	}

	// Validate email format
	if err := services.ValidateEmail(req.Email); err != nil {
		dto.RespondError(c, http.StatusBadRequest, "Invalid email format")
		return
	}

	// Create reset token - always returns success for security (prevents email enumeration)
	_, err := h.resetService.CreateResetToken(req.Email)
	if err != nil {
		// Log error but don't expose it
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
		return
	}

	// Always return success to prevent email enumeration
	dto.RespondSuccess(c, http.StatusOK, gin.H{
		"message": "If your email is registered, you will receive a password reset link shortly",
	})
}

// ResetPassword handles password reset with token
//
// @Summary Reset password using a reset token
// @Description Sets a new password for the account tied to the given reset token, after checking the token is present, unexpired, and unused, and that the new password is at least 8 characters. Returns a 401 if the token is invalid or expired. This is a public endpoint that relies on the reset token itself for authorization rather than a bearer session.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body dto.ResetPasswordRequest true "Reset token and new password"
// @Success 200 {object} map[string]string "message"
// @Failure 400 {object} dto.ErrorResponse "Missing token/password or password too short"
// @Failure 401 {object} dto.ErrorResponse "Invalid or expired reset token"
// @Failure 500 {object} dto.ErrorResponse "Failed to reset password"
// @Router /auth/reset-password [post]
func (h *PasswordResetHandler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondError(c, http.StatusBadRequest, "Token and new password are required")
		return
	}

	// Validate request
	if req.Token == "" {
		dto.RespondError(c, http.StatusBadRequest, "Token is required")
		return
	}
	if req.NewPassword == "" {
		dto.RespondError(c, http.StatusBadRequest, "New password is required")
		return
	}

	// Attempt password reset
	err := h.resetService.ResetPassword(req.Token, req.NewPassword)
	if err != nil {
		switch err {
		case services.ErrInvalidResetToken:
			dto.RespondError(c, http.StatusUnauthorized, "Invalid or expired reset token")
		case services.ErrPasswordTooShort:
			dto.RespondError(c, http.StatusBadRequest, "Password must be at least 8 characters")
		default:
			dto.RespondError(c, http.StatusInternalServerError, "Failed to reset password")
		}
		return
	}

	dto.RespondSuccess(c, http.StatusOK, gin.H{
		"message": "Password has been reset successfully",
	})
}

// SetupPasswordResetRoutes configures password reset routes
func SetupPasswordResetRoutes(router *gin.Engine, resetService *services.PasswordResetService, userRepo user.Repository) {
	handler := NewPasswordResetHandler(resetService, userRepo)

	// Password reset routes (public - no JWT required)
	auth := router.Group("/api/v1/auth")
	{
		auth.POST("/forgot-password", handler.ForgotPassword)
		auth.POST("/reset-password", handler.ResetPassword)
	}
}
