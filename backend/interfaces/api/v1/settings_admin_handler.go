package v1

import (
	"net/http"
	"os"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
)

// SettingsAdminHandler handles settings-related admin HTTP requests
type SettingsAdminHandler struct {
	orgRepo organization.Repository
}

// NewSettingsAdminHandler creates a new SettingsAdminHandler
func NewSettingsAdminHandler(orgRepo organization.Repository) *SettingsAdminHandler {
	return &SettingsAdminHandler{orgRepo: orgRepo}
}

// GetDimensions handles GET /api/v1/admin/settings/dimensions
//
// @Summary List health dimensions (admin)
// @Description Returns every configured health dimension, including inactive ones, unlike the public dimensions endpoint which returns only active ones. Each entry includes its ID, name, description, good/bad anchor descriptions, active flag, weight, and timestamps. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Produce json
// @Success 200 {object} dto.DimensionsResponse
// @Failure 500 {object} dto.ErrorResponse "Failed to query dimensions"
// @Security BearerAuth
// @Router /admin/settings/dimensions [get]
func (h *SettingsAdminHandler) GetDimensions(c *gin.Context) {
	dimensions, err := h.orgRepo.FindDimensions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to query dimensions",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTOs
	dimensionDTOs := make([]dto.HealthDimensionDTO, len(dimensions))
	for i, dim := range dimensions {
		dimensionDTOs[i] = dto.HealthDimensionDTO{
			ID:              dim.ID,
			Name:            dim.Name,
			Description:     dim.Description,
			GoodDescription: dim.GoodDescription,
			BadDescription:  dim.BadDescription,
			IsActive:        dim.IsActive,
			Weight:          dim.Weight,
			CreatedAt:       dim.CreatedAt,
			UpdatedAt:       dim.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, dto.DimensionsResponse{Dimensions: dimensionDTOs})
}

// CreateDimension handles POST /api/v1/admin/settings/dimensions
//
// @Summary Create a health dimension
// @Description Creates a new health dimension with an ID, name, and good/bad anchor descriptions. Weight must fall between 0 and 10 and defaults to 1.0 when omitted; isActive defaults to true when omitted. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Accept json
// @Produce json
// @Param body body dto.CreateDimensionRequest true "Dimension to create"
// @Success 201 {object} dto.HealthDimensionDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or weight out of range"
// @Failure 500 {object} dto.ErrorResponse "Failed to create dimension"
// @Security BearerAuth
// @Router /admin/settings/dimensions [post]
func (h *SettingsAdminHandler) CreateDimension(c *gin.Context) {
	var req dto.CreateDimensionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Validate weight
	if req.Weight < 0 || req.Weight > 10 {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Weight must be between 0 and 10"})
		return
	}

	// Default isActive to true if not provided
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	// Default weight to 1.0 if not provided
	weight := req.Weight
	if weight == 0 {
		weight = 1.0
	}

	// Create domain object
	dim := &organization.HealthDimension{
		ID:              req.ID,
		Name:            req.Name,
		Description:     req.Description,
		GoodDescription: req.GoodDescription,
		BadDescription:  req.BadDescription,
		IsActive:        isActive,
		Weight:          weight,
	}

	// Save using repository
	if err := h.orgRepo.SaveDimension(c.Request.Context(), dim); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to create dimension",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTO and return
	responseDTO := dto.HealthDimensionDTO{
		ID:              dim.ID,
		Name:            dim.Name,
		Description:     dim.Description,
		GoodDescription: dim.GoodDescription,
		BadDescription:  dim.BadDescription,
		IsActive:        dim.IsActive,
		Weight:          dim.Weight,
		CreatedAt:       dim.CreatedAt,
		UpdatedAt:       dim.UpdatedAt,
	}

	c.JSON(http.StatusCreated, responseDTO)
}

// UpdateDimension handles PUT /api/v1/admin/settings/dimensions/:id
//
// @Summary Update a health dimension
// @Description Updates the fields of the health dimension identified by the path ID; only the fields present in the request body are changed. Weight, if provided, must fall between 0 and 10. Returns a 404 if no dimension exists with that ID. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Accept json
// @Produce json
// @Param id path string true "Dimension ID"
// @Param body body dto.UpdateDimensionRequest true "Fields to update"
// @Success 200 {object} dto.HealthDimensionDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or weight out of range"
// @Failure 404 {object} dto.ErrorResponse "Dimension not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to update dimension"
// @Security BearerAuth
// @Router /admin/settings/dimensions/{id} [put]
func (h *SettingsAdminHandler) UpdateDimension(c *gin.Context) {
	id := c.Param("id")
	var req dto.UpdateDimensionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Validate weight if provided
	if req.Weight != nil && (*req.Weight < 0 || *req.Weight > 10) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Weight must be between 0 and 10"})
		return
	}

	// Find existing dimension
	dim, err := h.orgRepo.FindDimensionByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Dimension not found"})
		return
	}

	// Update fields if provided
	if req.Name != nil {
		dim.Name = *req.Name
	}
	if req.Description != nil {
		dim.Description = *req.Description
	}
	if req.GoodDescription != nil {
		dim.GoodDescription = *req.GoodDescription
	}
	if req.BadDescription != nil {
		dim.BadDescription = *req.BadDescription
	}
	if req.IsActive != nil {
		dim.IsActive = *req.IsActive
	}
	if req.Weight != nil {
		dim.Weight = *req.Weight
	}

	// Update using repository
	if err := h.orgRepo.UpdateDimension(c.Request.Context(), dim); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to update dimension",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTO and return
	responseDTO := dto.HealthDimensionDTO{
		ID:              dim.ID,
		Name:            dim.Name,
		Description:     dim.Description,
		GoodDescription: dim.GoodDescription,
		BadDescription:  dim.BadDescription,
		IsActive:        dim.IsActive,
		Weight:          dim.Weight,
		CreatedAt:       dim.CreatedAt,
		UpdatedAt:       dim.UpdatedAt,
	}

	c.JSON(http.StatusOK, responseDTO)
}

// DeleteDimension handles DELETE /api/v1/admin/settings/dimensions/:id
//
// @Summary Delete a health dimension
// @Description Permanently deletes the health dimension identified by the path ID. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Produce json
// @Param id path string true "Dimension ID"
// @Success 200 {object} map[string]string "message"
// @Failure 500 {object} dto.ErrorResponse "Failed to delete dimension"
// @Security BearerAuth
// @Router /admin/settings/dimensions/{id} [delete]
func (h *SettingsAdminHandler) DeleteDimension(c *gin.Context) {
	id := c.Param("id")

	// Delete using repository
	if err := h.orgRepo.DeleteDimension(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to delete dimension",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Dimension deleted successfully"})
}

// GetBrandingSettings handles GET /api/v1/admin/settings/branding
//
// @Summary Get branding settings
// @Description Returns the organization's current branding: the company name and the logo URL (a base64 data URL when a custom logo has been uploaded). Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Produce json
// @Success 200 {object} dto.BrandingSettings
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch branding settings"
// @Security BearerAuth
// @Router /admin/settings/branding [get]
func (h *SettingsAdminHandler) GetBrandingSettings(c *gin.Context) {
	appSettings, err := h.orgRepo.GetAppSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to fetch branding settings", Message: err.Error()})
		return
	}

	settings := dto.BrandingSettings{
		CompanyName: appSettings.CompanyName,
		LogoURL:     appSettings.LogoURL,
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateBrandingSettings handles PUT /api/v1/admin/settings/branding
//
// @Summary Update branding settings
// @Description Replaces the organization's company name and logo URL with the given values. The logo is expected as a base64 data URL and is rejected with a 400 if it exceeds roughly 500KB. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Accept json
// @Produce json
// @Param body body dto.BrandingSettings true "Branding settings"
// @Success 200 {object} dto.BrandingSettings
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or logo too large"
// @Failure 500 {object} dto.ErrorResponse "Failed to save branding settings"
// @Security BearerAuth
// @Router /admin/settings/branding [put]
func (h *SettingsAdminHandler) UpdateBrandingSettings(c *gin.Context) {
	var settings dto.BrandingSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Validate logo size (base64 data URLs can be large)
	if len(settings.LogoURL) > 700000 {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Logo is too large. Maximum size is 500KB."})
		return
	}

	if err := h.orgRepo.UpdateBrandingSettings(c.Request.Context(), settings.CompanyName, settings.LogoURL); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to save branding settings", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// GetNotificationSettings handles GET /api/v1/admin/settings/notifications
//
// @Summary Get notification settings
// @Description Returns the organization's notification configuration: whether email and Slack notifications are enabled and whether the weekly digest is on, read from stored settings, alongside whether an SMTP host is configured on the server. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Produce json
// @Success 200 {object} dto.NotificationSettings
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch settings"
// @Security BearerAuth
// @Router /admin/settings/notifications [get]
func (h *SettingsAdminHandler) GetNotificationSettings(c *gin.Context) {
	appSettings, err := h.orgRepo.GetAppSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to fetch settings", Message: err.Error()})
		return
	}

	settings := dto.NotificationSettings{
		EmailEnabled:       appSettings.EmailNotifications,
		SlackEnabled:       appSettings.SlackNotifications,
		NotifyOnSubmission: appSettings.WeeklyDigest,
		NotifyManagers:     false,
		ReminderDaysBefore: 7,
		ReminderRecipients: []string{},
		SmtpConfigured:     os.Getenv("SMTP_HOST") != "",
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateNotificationSettings handles PUT /api/v1/admin/settings/notifications
//
// @Summary Update notification settings
// @Description Updates the organization's stored notification configuration: whether email and Slack notifications are enabled and whether the weekly digest is on. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Accept json
// @Produce json
// @Param body body dto.NotificationSettings true "Notification settings"
// @Success 200 {object} dto.NotificationSettings
// @Failure 400 {object} dto.ErrorResponse "Invalid request body"
// @Failure 500 {object} dto.ErrorResponse "Failed to save settings"
// @Security BearerAuth
// @Router /admin/settings/notifications [put]
func (h *SettingsAdminHandler) UpdateNotificationSettings(c *gin.Context) {
	var settings dto.NotificationSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	if err := h.orgRepo.UpdateNotificationSettings(c.Request.Context(), settings.EmailEnabled, settings.SlackEnabled, settings.NotifyOnSubmission); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to save settings", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// GetRetentionPolicy handles GET /api/v1/admin/settings/retention
//
// @Summary Get data retention policy
// @Description Returns the organization's data retention policy: how many months completed sessions are kept, and the anonymization window in days, which is derived as thirty times the retention months. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Produce json
// @Success 200 {object} dto.RetentionPolicy
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch retention policy"
// @Security BearerAuth
// @Router /admin/settings/retention [get]
func (h *SettingsAdminHandler) GetRetentionPolicy(c *gin.Context) {
	appSettings, err := h.orgRepo.GetAppSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to fetch retention policy", Message: err.Error()})
		return
	}

	policy := dto.RetentionPolicy{
		KeepSessionsMonths: appSettings.RetentionMonths,
		ArchiveEnabled:     false,
		AnonymizeAfterDays: appSettings.RetentionMonths * 30,
	}

	c.JSON(http.StatusOK, policy)
}

// UpdateRetentionPolicy handles PUT /api/v1/admin/settings/retention
//
// @Summary Update data retention policy
// @Description Updates how many months completed sessions are retained; the value must be between 1 and 120. The anonymization window returned in the response is recalculated as thirty times that value. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Settings
// @Accept json
// @Produce json
// @Param body body dto.RetentionPolicy true "Retention policy"
// @Success 200 {object} dto.RetentionPolicy
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or keepSessionsMonths out of range"
// @Failure 500 {object} dto.ErrorResponse "Failed to save retention policy"
// @Security BearerAuth
// @Router /admin/settings/retention [put]
func (h *SettingsAdminHandler) UpdateRetentionPolicy(c *gin.Context) {
	var policy dto.RetentionPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	if policy.KeepSessionsMonths < 1 || policy.KeepSessionsMonths > 120 {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Keep sessions months must be between 1 and 120"})
		return
	}

	if err := h.orgRepo.UpdateRetentionSettings(c.Request.Context(), policy.KeepSessionsMonths); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to save retention policy", Message: err.Error()})
		return
	}

	policy.AnonymizeAfterDays = policy.KeepSessionsMonths * 30

	c.JSON(http.StatusOK, policy)
}
