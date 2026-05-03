package mskdata

import (
	"errors"
	"fmt"

	"github.com/goccy/go-json"
)

type Error struct {
	Code int32
	Err  error
}

func GetError(err error) *Error {
	if merr, ok := errors.AsType[*Error](err); ok {
		return merr
	} else {
		return &Error{
			Code: -32767,
			Err:  err,
		}
	}
}

func Errorf(code int32, format string, a ...any) *Error {
	return &Error{
		Code: code,
		Err:  fmt.Errorf(format, a...),
	}
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) MarshalJSON() ([]byte, error) {
	var errStr *string
	if e.Err != nil {
		s := e.Err.Error()
		errStr = &s
	}

	return json.Marshal(struct {
		Code int32   `json:"code"`
		Err  *string `json:"message"`
	}{
		Code: e.Code,
		Err:  errStr,
	})
}

func (e *Error) Error() string {
	return fmt.Sprintf("[code %d] %s", e.Code, e.Err)
}
