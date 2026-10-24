package service

import "fmt"

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"status"`
	Details any    `json:"details,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

func Invalid(message string) *AppError {
	return &AppError{Code: "INVALID_REQUEST", Message: message, Status: 400}
}

func NotFound(message string) *AppError {
	return &AppError{Code: "NOT_FOUND", Message: message, Status: 404}
}

func Conflict(message string) *AppError {
	return &AppError{Code: "CONFLICT", Message: message, Status: 409}
}

func Internal(err error) *AppError {
	return &AppError{Code: "INTERNAL_ERROR", Message: fmt.Sprint(err), Status: 500}
}
