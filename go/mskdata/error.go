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

func GetError(err error) (result *Error) {
	if !errors.As(err, result) {
		result = &Error{
			Code: -32767,
			Err:  err,
		}
	}
	return
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
