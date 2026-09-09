package dto

// LoginRequest represents the login credentials
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
} //@name LoginRequest

// LoginResponse represents the successful login response with JWT tokens
type LoginResponse struct {
	User         UserDTO `json:"user"`
	AccessToken  string  `json:"accessToken"`
	RefreshToken string  `json:"refreshToken"`
	ExpiresIn    int64   `json:"expiresIn"` // Access token expiry in seconds
} //@name LoginResponse

// RefreshTokenRequest represents a token refresh request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
} //@name RefreshTokenRequest

// RefreshTokenResponse represents a token refresh response
type RefreshTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int64  `json:"expiresIn"` // Access token expiry in seconds
} //@name RefreshTokenResponse

// UserDTO represents user data transfer object
type UserDTO struct {
	ID             string   `json:"id"`
	Username       string   `json:"username"`
	Email          string   `json:"email"`
	FullName       string   `json:"fullName"`
	HierarchyLevel string   `json:"hierarchyLevel"`
	TeamIds        []string `json:"teamIds"`
	CanTakeSurvey  bool     `json:"canTakeSurvey"`
} //@name UserDTO

// ForgotPasswordRequest represents a forgot password request
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required"`
} //@name ForgotPasswordRequest

// ResetPasswordRequest represents a password reset request
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
} //@name ResetPasswordRequest
