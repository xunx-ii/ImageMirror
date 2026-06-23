package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrNotFound         = errors.New("not found")
	ErrInsufficientFund = errors.New("insufficient credits")
	ErrBadRequest       = errors.New("bad request")
)

type APIError struct {
	Code    int
	Message string
	Err     error
}

func (e APIError) Error() string {
	return e.Message
}

func NewError(code int, message string, err error) APIError {
	return APIError{Code: code, Message: message, Err: err}
}

func Abort(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"

	var apiErr APIError
	if errors.As(err, &apiErr) {
		status = apiErr.Code
		message = apiErr.Message
	} else {
		switch {
		case errors.Is(err, ErrUnauthorized):
			status = http.StatusUnauthorized
			message = "unauthorized"
		case errors.Is(err, ErrForbidden):
			status = http.StatusForbidden
			message = "forbidden"
		case errors.Is(err, ErrNotFound):
			status = http.StatusNotFound
			message = "not found"
		case errors.Is(err, ErrInsufficientFund):
			status = http.StatusPaymentRequired
			message = "insufficient credits"
		case errors.Is(err, ErrBadRequest):
			status = http.StatusBadRequest
			message = "bad request"
		}
	}

	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"message": message}})
}

func OK(c *gin.Context, payload any) {
	c.JSON(http.StatusOK, payload)
}

func Created(c *gin.Context, payload any) {
	c.JSON(http.StatusCreated, payload)
}
