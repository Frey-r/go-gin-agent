package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/service"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService *service.AuthService
	validate    *validator.Validate
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		validate:    validator.New(),
	}
}

// Register handles POST /api/v1/auth/register.
// Registration is invitation-only: the email must have a valid invitation.
// The response is intentionally generic to prevent email enumeration.
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "details": formatValidationErrors(err)})
		return
	}

	user, err := h.authService.Register(c.Request.Context(), req)
	if err != nil {
		switch err {
		case service.ErrNoInvitation:
			// Generic response to prevent enumeration
			c.JSON(http.StatusForbidden, gin.H{"error": "registration not available"})
		case service.ErrWeakPassword:
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "password must be at least 8 characters with 1 uppercase, 1 number, and 1 symbol",
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "account created successfully",
		"user": gin.H{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

// Login handles POST /api/v1/auth/login.
// Error messages are generic to prevent credential enumeration.
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tokenPair, err := h.authService.Login(c.Request.Context(), req)
	if err != nil {
		switch err {
		case service.ErrAccountLocked:
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "account temporarily locked, try again later"})
		case service.ErrInvalidCredentials, service.ErrAccountDisabled:
			// Generic message — never reveal which field is wrong
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication failed"})
		}
		return
	}

	c.JSON(http.StatusOK, tokenPair)
}

// Refresh handles POST /api/v1/auth/refresh.
// Implements token rotation: old refresh token is revoked, new pair issued.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tokenPair, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}

	c.JSON(http.StatusOK, tokenPair)
}

// formatValidationErrors extracts human-readable validation error messages.
func formatValidationErrors(err error) []string {
	var errors []string
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			errors = append(errors, e.Field()+" "+e.Tag())
		}
	}
	return errors
}
