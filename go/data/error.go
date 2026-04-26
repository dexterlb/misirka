package data

import (
	"fmt"

	"github.com/goccy/go-json"
)

type Error struct {
	Code int32
	Err  error
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
