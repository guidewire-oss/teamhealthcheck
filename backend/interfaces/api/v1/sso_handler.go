// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ssoCallbackRequest is sent by the frontend after the OAuth redirect.
type ssoCallbackRequest struct {
	Code         string `json:"code" binding:"required"`
	CodeVerifier string `json:"code_verifier" binding:"required"`
}

// oauthTokenResponse is the raw response from the provider's token endpoint.
type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
}

// SSOHandler handles SSO-related HTTP requests.
type SSOHandler struct {
	userRepo   user.Repository
	jwtService *services.JWTService
}

// NewSSOHandler creates a new SSOHandler.
func NewSSOHandler(userRepo user.Repository, jwtService *services.JWTService) *SSOHandler {
	return &SSOHandler{userRepo: userRepo, jwtService: jwtService}
}

// Callback exchanges the authorization code + PKCE verifier for provider tokens,
// extracts the email from the id_token, looks the user up in the DB, and issues
// our own JWT token pair — the same structure as a regular password login.
func (h *SSOHandler) Callback(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.Get().WithContext(ctx)

	clientID := os.Getenv("OAUTH_CLIENT_ID")
	tokenURL := os.Getenv("OAUTH_TOKEN_URL")
	redirectURI := os.Getenv("OAUTH_REDIRECT_URI")

	if clientID == "" || tokenURL == "" {
		dto.RespondError(c, http.StatusServiceUnavailable, "SSO is not configured on this server")
		return
	}

	var req ssoCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondError(c, http.StatusBadRequest, "code and code_verifier are required")
		return
	}

	// Exchange code + PKCE verifier for provider tokens
	providerTokens, err := exchangeCodeForTokens(tokenURL, clientID, redirectURI, req.Code, req.CodeVerifier)
	if err != nil {
		log.WithError(err).Error("SSO token exchange failed")
		dto.RespondError(c, http.StatusUnauthorized, "Token exchange with OAuth provider failed")
		return
	}

	// Extract email from id_token; fall back to access_token if needed
	email, _ := extractEmailFromJWT(providerTokens.IDToken)
	if email == "" {
		email, _ = extractEmailFromJWT(providerTokens.AccessToken)
	}
	if email == "" {
		dto.RespondError(c, http.StatusUnauthorized, "Could not determine email from SSO token")
		return
	}

	// Look up the user by email
	usr, err := h.userRepo.FindByEmail(ctx, email)
	if err != nil {
		log.WithField("email", email).Warn("SSO login: no user found for email")
		dto.RespondError(c, http.StatusUnauthorized, "No account found for this email address. Please contact your administrator.")
		return
	}

	// Only SSO users can authenticate via SSO
	if usr.AuthType != user.AuthTypeSSO {
		log.WithField("email", email).Warn("SSO login: local user attempted SSO login")
		dto.RespondError(c, http.StatusUnauthorized, "This account does not support SSO login. Please use username and password.")
		return
	}

	teamIds := collectTeamIDs(ctx, h.userRepo, usr.ID)

	// Issue our own JWT tokens
	tokenPair, err := h.jwtService.GenerateTokenPair(ctx, usr.ID, usr.Username, usr.Email, usr.HierarchyLevelID, teamIds)
	if err != nil {
		dto.RespondError(c, http.StatusInternalServerError, "Failed to generate authentication tokens")
		return
	}

	log.WithField("user_id", usr.ID).Info("SSO login successful")

	dto.RespondSuccess(c, http.StatusOK, dto.LoginResponse{
		User: dto.UserDTO{
			ID:             usr.ID,
			Username:       usr.Username,
			Email:          usr.Email,
			FullName:       usr.Name,
			HierarchyLevel: usr.HierarchyLevelID,
			TeamIds:        teamIds,
		},
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
	})
}

// exchangeCodeForTokens calls the provider's token endpoint using PKCE
// (no client secret — this is a public / SPA client).
func exchangeCodeForTokens(tokenURL, clientID, redirectURI, code, codeVerifier string) (*oauthTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("request to token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	return &tokenResp, nil
}

// extractEmailFromJWT decodes a JWT and returns the "email" claim.
// Signature is intentionally NOT verified — the token was obtained directly
// from the provider's token endpoint (server-side), not supplied by the user.
func extractEmailFromJWT(tokenStr string) (string, error) {
	if tokenStr == "" {
		return "", fmt.Errorf("empty token")
	}
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(tokenStr, claims)
	if err != nil {
		return "", fmt.Errorf("parsing JWT: %w", err)
	}
	email, _ := claims["email"].(string)
	return email, nil
}
