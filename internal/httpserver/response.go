package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type response struct {
	Data  any        `json:"data,omitempty"`
	Error *apiError  `json:"error,omitempty"`
	Meta  gin.H      `json:"meta,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, response{Data: data})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, response{Data: data})
}

func Fail(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, response{
		Error: &apiError{
			Code:    code,
			Message: message,
		},
	})
}
