package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// PaginationMeta defines the structure for pagination metadata
type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"totalPages"`
}

// Response defines the standard API response structure
type Response struct {
	Success    bool            `json:"success"`
	Message    string          `json:"message"`
	StatusCode int             `json:"statusCode"`
	Data       interface{}     `json:"data"`
	Meta       *PaginationMeta `json:"meta,omitempty"`
}

type ValidationFieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Success sends a success response with 200 OK
func Success(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success:    true,
		Message:    message,
		StatusCode: http.StatusOK,
		Data:       data,
	})
}

// Created sends a success response with 201 Created
func Created(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Success:    true,
		Message:    message,
		StatusCode: http.StatusCreated,
		Data:       data,
	})
}

// Paginated sends a success response with pagination metadata
func Paginated(c *gin.Context, message string, data interface{}, page, limit int, total int64) {
	if limit <= 0 {
		limit = 50
	}
	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, Response{
		Success:    true,
		Message:    message,
		StatusCode: http.StatusOK,
		Data:       data,
		Meta: &PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// Error sends an error response with the specified status code
func Error(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, Response{
		Success:    false,
		Message:    message,
		StatusCode: statusCode,
		Data:       nil,
	})
}

// ValidationError sends a 400 Bad Request response
func ValidationError(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, message)
}

func ValidationErrorFields(c *gin.Context, fields []ValidationFieldError) {
	c.JSON(http.StatusBadRequest, Response{
		Success:    false,
		Message:    "validation error",
		StatusCode: http.StatusBadRequest,
		Data: gin.H{
			"fields": fields,
		},
	})
}

// InternalServerError sends a 500 Internal Server Error response
func InternalServerError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, message)
}

// GatewayTimeout sends a 504 Gateway Timeout response. Use when a request
// exceeded its deadline (context cancellation or a Postgres statement_timeout
// / query-canceled, see database.IsTimeout) rather than failing outright —
// surfacing 504 instead of 500 lets clients and operators tell "too slow"
// apart from "server broke".
func GatewayTimeout(c *gin.Context, message string) {
	Error(c, http.StatusGatewayTimeout, message)
}

// NotFound sends a 404 Not Found response
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, message)
}

// Unauthorized sends a 401 Unauthorized response
func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, message)
}

// Forbidden sends a 403 Forbidden response
func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, message)
}

// Conflict sends a 409 Conflict response
func Conflict(c *gin.Context, message string) {
	Error(c, http.StatusConflict, message)
}

// UnprocessableEntity sends a 422 Unprocessable Entity response. Used when
// the request is syntactically valid but breaks a business rule (e.g.
// percentages don't sum to 100). Distinct from 400 (malformed payload)
// and 409 (lifecycle conflict) so callers can react accordingly.
func UnprocessableEntity(c *gin.Context, message string) {
	Error(c, http.StatusUnprocessableEntity, message)
}
