package commands

import "fmt"

// ValidationError represents a client input validation failure raised by a command handler.
// Handlers may surface its message directly to the client (as a 400 Bad Request) since, unlike
// a wrapped repository/infrastructure error, it never contains internal implementation details.
type ValidationError struct {
	msg string
}

// NewValidationError creates a ValidationError with a formatted message.
func NewValidationError(format string, args ...any) *ValidationError {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

func (e *ValidationError) Error() string { return e.msg }
