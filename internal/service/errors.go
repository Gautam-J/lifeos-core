package service

import "errors"

// ErrNotFound means that no live record has the requested ID.
var ErrNotFound = errors.New("not found")

// ValidationError means that the service refused the input. Field is the
// name of the input field, as the adapters spell it in JSON.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func invalid(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
