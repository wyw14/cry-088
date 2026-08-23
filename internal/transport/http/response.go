package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/wyw14/cry-088/internal/domain/shared"
)

type ErrorResponse struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id"`
}

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	response := ErrorResponse{Code: "INTERNAL", Message: "internal server error", Fields: map[string]string{}, RequestID: requestID(c)}
	var domainErr *shared.Error
	if errors.As(err, &domainErr) {
		response.Code = string(domainErr.Code)
		response.Message = domainErr.Message
		response.Fields = domainErr.Fields
		switch domainErr.Code {
		case shared.CodeInvalid:
			status = http.StatusBadRequest
		case shared.CodeUnauthenticated:
			status = http.StatusUnauthorized
		case shared.CodeForbidden:
			status = http.StatusForbidden
		case shared.CodeNotFound:
			status = http.StatusNotFound
		case shared.CodeConflict, shared.CodeIllegalState, shared.CodeCapacity, shared.CodePeriodClosed:
			status = http.StatusConflict
		}
	}
	c.JSON(status, response)
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		writeError(c, shared.Wrap(shared.CodeInvalid, "request body is invalid", err))
		return false
	}
	if v := validator.New(); v.Struct(target) != nil {
		writeError(c, shared.New(shared.CodeInvalid, "request fields are invalid"))
		return false
	}
	return true
}

func requestID(c *gin.Context) string {
	if value, ok := c.Get("request_id"); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return "unknown"
}
