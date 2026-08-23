package shared

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeInvalid         Code = "INVALID_ARGUMENT"
	CodeNotFound        Code = "NOT_FOUND"
	CodeForbidden       Code = "FORBIDDEN"
	CodeConflict        Code = "CONFLICT"
	CodeIllegalState    Code = "ILLEGAL_STATE"
	CodePeriodClosed    Code = "PERIOD_CLOSED"
	CodeCapacity        Code = "CAPACITY_EXCEEDED"
	CodeUnauthenticated Code = "UNAUTHENTICATED"
)

type Error struct {
	Code    Code
	Message string
	Fields  map[string]string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message, Fields: map[string]string{}}
}

func Field(code Code, message, field, detail string) *Error {
	return &Error{Code: code, Message: message, Fields: map[string]string{field: detail}}
}

func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Fields: map[string]string{}, Cause: cause}
}

func IsCode(err error, code Code) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}
