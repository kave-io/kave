package errors

import "fmt"

// CommandError defines the exit code for different error categories
type CommandError interface {
	error
	ExitCode() int
}

type UserInputError struct {
	Message string
}

func (e *UserInputError) Error() string {
	return e.Message
}

func (e *UserInputError) ExitCode() int {
	return 1
}

func NewUserInputError(msg string) *UserInputError {
	return &UserInputError{Message: msg}
}

func NewUserInputErrorf(format string, args ...interface{}) *UserInputError {
	return &UserInputError{Message: fmt.Sprintf(format, args...)}
}

type DaemonError struct {
	Message string
}

func (e *DaemonError) Error() string {
	return e.Message
}

func (e *DaemonError) ExitCode() int {
	return 2
}

func NewDaemonError(msg string) *DaemonError {
	return &DaemonError{Message: msg}
}

func NewDaemonErrorf(format string, args ...interface{}) *DaemonError {
	return &DaemonError{Message: fmt.Sprintf(format, args...)}
}

type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

func (e *AuthError) ExitCode() int {
	return 3
}

func NewAuthError(msg string) *AuthError {
	return &AuthError{Message: msg}
}

func NewAuthErrorf(format string, args ...interface{}) *AuthError {
	return &AuthError{Message: fmt.Sprintf(format, args...)}
}

type InternalError struct {
	Message string
}

func (e *InternalError) Error() string {
	return e.Message
}

func (e *InternalError) ExitCode() int {
	return 1
}

func NewInternalError(msg string) *InternalError {
	return &InternalError{Message: msg}
}

func NewInternalErrorf(format string, args ...interface{}) *InternalError {
	return &InternalError{Message: fmt.Sprintf(format, args...)}
}
