// SPDX-License-Identifier: MIT

// Package errs holds canbusctl's typed errors, their stable codes and the
// process exit codes they map to (docs/exitcodes.md).
package errs

import (
	"context"
	"errors"
	"fmt"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/isotp"
)

// Kind is a failure category; it selects the exit code.
type Kind string

// Kinds.
const (
	KindUsage        Kind = "usage"
	KindInvalidInput Kind = "invalid_input"
	KindTransport    Kind = "transport"
	KindTimeout      Kind = "timeout"
	KindProtocol     Kind = "protocol"
	KindUnsupported  Kind = "unsupported"
	KindSafety       Kind = "safety"
	KindPermission   Kind = "permission"
	KindOutput       Kind = "output"
	KindInternal     Kind = "internal"
)

// Process exit codes.
const (
	ExitOK           = 0
	ExitFindings     = 1 // a result was written and it reports problems (validation errors, undecodable frames)
	ExitUsage        = 2
	ExitInvalidInput = 3
	ExitTransport    = 4
	ExitTimeout      = 5
	ExitProtocol     = 6
	ExitPermission   = 7
	ExitUnsupported  = 8
	ExitSafety       = 9
	ExitInternal     = 10
)

// Stable codes for scripts (the JSON error's "code").
const (
	CodeInvalidInput       = "invalid_input"
	CodeInvalidFrame       = "invalid_frame"
	CodeInvalidDatabase    = "invalid_database"
	CodeInvalidCapture     = "invalid_capture"
	CodeFileOpen           = "file_open_failed"
	CodeInterface          = "interface_unavailable"
	CodeIO                 = "io_failed"
	CodeTimeout            = "timeout"
	CodeISOTP              = "isotp_failed"
	CodeUnsupported        = "unsupported"
	CodeNeedsYes           = "confirmation_required"
	CodePermission         = "permission_denied"
	CodeOutputFailed       = "output_failed"
	CodeCanceled           = "canceled"
	CodeInternalUnexpected = "internal_unexpected"
)

// Error is a typed failure.
type Error struct {
	Kind    Kind   `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func (e *Error) Error() string { return e.Message }

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Cause }

// New returns a typed error.
func New(kind Kind, code, format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	return &Error{Kind: kind, Code: code, Message: err.Error(), Cause: errors.Unwrap(err)}
}

// Invalid is an invalid input error.
func Invalid(format string, args ...any) *Error {
	return New(KindInvalidInput, CodeInvalidInput, format, args...)
}

// ExitCode maps a kind to its exit code.
func ExitCode(k Kind) int {
	switch k {
	case KindUsage:
		return ExitUsage
	case KindInvalidInput:
		return ExitInvalidInput
	case KindTransport:
		return ExitTransport
	case KindTimeout:
		return ExitTimeout
	case KindProtocol:
		return ExitProtocol
	case KindPermission:
		return ExitPermission
	case KindUnsupported:
		return ExitUnsupported
	case KindSafety:
		return ExitSafety
	default:
		return ExitInternal
	}
}

// Classify turns any error into a typed one, recognising go-canbus
// sentinels.
func Classify(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	mk := func(k Kind, code string) *Error { return &Error{Kind: k, Code: code, Message: err.Error(), Cause: err} }
	switch {
	case errors.Is(err, context.Canceled):
		return mk(KindInternal, CodeCanceled)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, canbus.ErrTimeout):
		return mk(KindTimeout, CodeTimeout)
	case errors.Is(err, isotp.ErrTimeout):
		return mk(KindTimeout, CodeISOTP)
	case errors.Is(err, isotp.ErrOverflow), errors.Is(err, isotp.ErrSequence), errors.Is(err, isotp.ErrWaitLimit),
		errors.Is(err, isotp.ErrProtocol), errors.Is(err, isotp.ErrInterrupted), errors.Is(err, isotp.ErrTooLarge):
		return mk(KindProtocol, CodeISOTP)
	case errors.Is(err, canbus.ErrUnsupported):
		return mk(KindUnsupported, CodeUnsupported)
	case errors.Is(err, canbus.ErrInterfaceDown):
		return mk(KindTransport, CodeInterface)
	case errors.Is(err, canbus.ErrInvalidFrame):
		return mk(KindInvalidInput, CodeInvalidFrame)
	case errors.Is(err, canbus.ErrInvalidOption):
		return mk(KindInvalidInput, CodeInvalidInput)
	case errors.Is(err, candb.ErrRange), errors.Is(err, candb.ErrUnknownSignal), errors.Is(err, candb.ErrMissingSignal),
		errors.Is(err, candb.ErrInactiveSignal), errors.Is(err, candb.ErrUnknownMessage), errors.Is(err, candb.ErrShortPayload):
		return mk(KindInvalidInput, CodeInvalidInput)
	case isPermission(err):
		return mk(KindPermission, CodePermission)
	case errors.Is(err, canbus.ErrClosed), errors.Is(err, canbus.ErrBufferOverflow):
		return mk(KindTransport, CodeIO)
	}
	return mk(KindInternal, CodeInternalUnexpected)
}

// ErrFindings ends a command whose result was written and reports
// problems: exit code 1, no error document.
var ErrFindings = errors.New("findings reported")
