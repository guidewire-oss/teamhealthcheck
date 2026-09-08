package v1

import (
	"errors"
	"net/http"
	"os"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

// OrganizationProviderHandler serves the external organization-data provider
// configuration status and the manual synchronization trigger. It holds no
// credential and performs no persistence or mapping of its own -- both live
// in OrganizationSyncService and OrganizationProviderRepository respectively.
type OrganizationProviderHandler struct {
	syncService  *services.OrganizationSyncService
	providerName string
}

// NewOrganizationProviderHandler creates the handler.
func NewOrganizationProviderHandler(syncService *services.OrganizationSyncService) *OrganizationProviderHandler {
	return &OrganizationProviderHandler{
		syncService:  syncService,
		providerName: "data-provider",
	}
}

// GetSettings handles GET /api/v1/admin/settings/organization-provider.
//
// This reports environment-configuration readiness only. It never queries a
// credential table (there is none) and never returns a token value.
func (h *OrganizationProviderHandler) GetSettings(c *gin.Context) {
	settings := dto.OrganizationProviderSettingsDTO{
		Provider:          h.providerName,
		BaseURLConfigured: os.Getenv(dataprovider.EnvBaseURL) != "",
		TokenConfigured:   os.Getenv(dataprovider.EnvAPIToken) != "",
		ReadyToSync:       h.syncService != nil && h.syncService.Configured(),
	}

	c.JSON(http.StatusOK, settings)
}

// Sync handles POST /api/v1/admin/organization-provider/sync.
func (h *OrganizationProviderHandler) Sync(c *gin.Context) {
	if h.syncService == nil || !h.syncService.Configured() {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Organization provider is not configured",
			Message: "Set " + dataprovider.EnvBaseURL + " and " + dataprovider.EnvAPIToken + " on the API service",
		})
		return
	}

	result, err := h.syncService.Sync(c.Request.Context())
	if err != nil {
		status, response := mapSyncError(err)
		c.JSON(status, response)
		return
	}

	c.JSON(http.StatusOK, result)
}

// mapSyncError converts a sync failure into a status code and a safe body.
// None of these branches echo a token, header, or the raw provider payload.
func mapSyncError(err error) (int, dto.ErrorResponse) {
	switch {
	case errors.Is(err, services.ErrSyncInProgress):
		return http.StatusConflict, dto.ErrorResponse{
			Error: "A synchronization is already running",
		}

	case errors.Is(err, services.ErrProviderNotConfigured):
		return http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Organization provider is not configured",
			Message: "Set " + dataprovider.EnvBaseURL + " and " + dataprovider.EnvAPIToken + " on the API service",
		}

	case errors.Is(err, services.ErrInvalidSnapshot):
		return http.StatusBadGateway, dto.ErrorResponse{
			Error:   "Provider returned data that failed contract validation",
			Message: err.Error(),
		}

	case errors.Is(err, orgprovider.ErrMassDeletionBlocked):
		return http.StatusConflict, dto.ErrorResponse{
			Error:   "Synchronization held for review",
			Message: "This sync would remove an unusually large share of users or teams. No changes were made. Review the provider data, or adjust " + services.EnvMaxDeletePercent + ", before retrying.",
		}

	default:
		logger.Get().WithError(err).Error("organization provider sync failed")
		return http.StatusBadGateway, dto.ErrorResponse{
			Error:   "Synchronization failed",
			Message: err.Error(),
		}
	}
}
