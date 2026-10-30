package validation

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
)

func BindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		if ve := Extract(err, obj); ve != nil {
			response.ValidationErrorFields(c, toResponseFields(ve.Fields))
			return false
		}
		response.ValidationError(c, err.Error())
		return false
	}
	return true
}

func BindJSONOptional(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		if ve := Extract(err, obj); ve != nil {
			response.ValidationErrorFields(c, toResponseFields(ve.Fields))
			return false
		}
		response.ValidationError(c, err.Error())
		return false
	}
	return true
}

func RespondIfValidationError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if ve := Extract(err); ve != nil {
		response.ValidationErrorFields(c, toResponseFields(ve.Fields))
		return true
	}
	response.Error(c, http.StatusBadRequest, err.Error())
	return true
}

func toResponseFields(fields []FieldError) []response.ValidationFieldError {
	out := make([]response.ValidationFieldError, 0, len(fields))
	for _, f := range fields {
		out = append(out, response.ValidationFieldError{
			Field:   f.Field,
			Message: f.Message,
			Code:    f.Code,
		})
	}
	return out
}
