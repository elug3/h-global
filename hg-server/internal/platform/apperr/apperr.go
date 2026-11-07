// Package apperr defines the errors services return. The HTTP layer maps each
// Kind to a status code, so services never deal with HTTP.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error for callers.
type Kind int

const (
	KindInvalid      Kind = iota + 1 // bad input: 400
	KindUnauthorized                 // no or bad session: 401
	KindForbidden                    // signed in but not allowed: 403
	KindNotFound                     // 404
	KindConflict                     // state does not allow it: 409
	KindUnavailable                  // a dependency failed: 502
)

// Error is a classified error with a stable machine code.
type Error struct {
	Kind    Kind
	Code    string // stable, snake_case, e.g. "sold_out"
	Message string // human readable, safe to show
	Details any    // optional structured details, returned as "details"
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// As returns the *Error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Is reports whether err carries the given code.
func Is(err error, code string) bool {
	e, ok := As(err)
	return ok && e.Code == code
}

func Invalid(code, format string, args ...any) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: fmt.Sprintf(format, args...)}
}

func Unauthorized(code, format string, args ...any) *Error {
	return &Error{Kind: KindUnauthorized, Code: code, Message: fmt.Sprintf(format, args...)}
}

func Forbidden(code, format string, args ...any) *Error {
	return &Error{Kind: KindForbidden, Code: code, Message: fmt.Sprintf(format, args...)}
}

// NotFound returns "<what>_not_found".
func NotFound(what string) *Error {
	return &Error{Kind: KindNotFound, Code: what + "_not_found", Message: what + " not found"}
}

func Conflict(code, format string, args ...any) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: fmt.Sprintf(format, args...)}
}

func Unavailable(code, format string, args ...any) *Error {
	return &Error{Kind: KindUnavailable, Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithDetails returns a copy of e carrying details.
func (e *Error) WithDetails(d any) *Error {
	c := *e
	c.Details = d
	return &c
}
