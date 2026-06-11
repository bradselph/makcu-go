package makcu

import "fmt"

type MakcuError struct {
	Msg string
}

func (e *MakcuError) Error() string { return e.Msg }

type MakcuConnectionError struct{ MakcuError }

type MakcuCommandError struct{ MakcuError }

type MakcuTimeoutError struct{ MakcuError }

type MakcuResponseError struct{ MakcuError }

func (e *MakcuConnectionError) Unwrap() error { return &e.MakcuError }
func (e *MakcuCommandError) Unwrap() error    { return &e.MakcuError }
func (e *MakcuTimeoutError) Unwrap() error    { return &e.MakcuError }
func (e *MakcuResponseError) Unwrap() error   { return &e.MakcuError }

func newMakcuConnectionError(format string, a ...any) *MakcuConnectionError {
	return &MakcuConnectionError{MakcuError{Msg: fmt.Sprintf(format, a...)}}
}

func newMakcuCommandError(format string, a ...any) *MakcuCommandError {
	return &MakcuCommandError{MakcuError{Msg: fmt.Sprintf(format, a...)}}
}

func newMakcuTimeoutError(format string, a ...any) *MakcuTimeoutError {
	return &MakcuTimeoutError{MakcuError{Msg: fmt.Sprintf(format, a...)}}
}

func newMakcuResponseError(format string, a ...any) *MakcuResponseError {
	return &MakcuResponseError{MakcuError{Msg: fmt.Sprintf(format, a...)}}
}
